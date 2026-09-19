package main

import (
	"context"
	"errors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"log"
	"os"
	"strings"
	"time"
)

type digestRecord struct {
	Status, Text, Date string
	Items              []digestItem
}

func digestDue(p digestPreference, now time.Time) bool {
	if !p.Enabled || validDigestTime(p.At, p.Zone, p.Days) != nil {
		return false
	}
	zone, _ := time.LoadLocation(p.Zone)
	local := now.In(zone)
	if p.Days == "weekdays" && (local.Weekday() == time.Saturday || local.Weekday() == time.Sunday) {
		return false
	}
	t, _ := time.Parse("15:04", p.At)
	minutes := local.Hour()*60 + local.Minute() - (t.Hour()*60 + t.Minute())
	return minutes >= 0 && minutes < 120
}
func (b *conversationBridge) digestSweep(ctx context.Context, now time.Time) {
	c, err := b.settings(ctx)
	if err != nil || !c.Digests {
		return
	}
	dir, err := conversationDir("digest-preferences")
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var p digestPreference
		prefKey := strings.TrimSuffix(entry.Name(), ".json")
		if readConversation("digest-preferences", prefKey, &p) != nil || !digestDue(p, now) || p.Team != c.Team || c.actor(p.Team, p.User) != p.Actor || !strings.HasPrefix(p.Channel, "D") {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		err = b.deliverDigest(callCtx, c, prefKey, p, now)
		cancel()
		if err != nil {
			log.Printf("slack: personal digest %s held for retry or reconciliation", prefKey)
		}
	}
}
func (b *conversationBridge) deliverDigest(ctx context.Context, c conversationSettings, prefKey string, p digestPreference, now time.Time) error {
	zone, _ := time.LoadLocation(p.Zone)
	date := now.In(zone).Format("2006-01-02")
	key := notificationDigest("digest-v1", p.Team, p.Actor, date)
	var record digestRecord
	err := readConversation("digest-days", key, &record)
	if os.IsNotExist(err) {
		items, scanErr := b.collectDigest(ctx, p)
		if scanErr != nil {
			return scanErr
		}
		record = digestRecord{Status: "prepared", Text: formatDigest(c, p, date, items), Date: date, Items: items}
		if err = writeConversation("digest-days", key, record, true); os.IsExist(err) {
			err = readConversation("digest-days", key, &record)
		}
	}
	if err != nil {
		return err
	}
	if record.Status == "sent" || record.Status == "empty" {
		return nil
	}
	if record.Status != "prepared" {
		return errors.New("digest needs reconciliation")
	}
	// Re-read identity, settings and every included target before any Slack post.
	latest, err := b.settings(ctx)
	if err != nil {
		return err
	}
	var current digestPreference
	if err = readConversation("digest-preferences", prefKey, &current); err != nil {
		return err
	}
	if !latest.Digests || !current.Enabled || current.Actor != p.Actor || current.EventTS != p.EventTS || current.Channel != p.Channel || latest.actor(p.Team, p.User) != p.Actor {
		return errors.New("digest preferences changed")
	}
	api, ok := pluginsdk.Attention(b.host())
	if !ok {
		return errors.New("attention Host API required")
	}
	for _, item := range record.Items {
		if err = api.Resolve(ctx, item.Target); err != nil {
			return err
		}
		task, readErr := b.host().Tasks().Get(ctx, item.Target.TaskID)
		if readErr != nil {
			return readErr
		}
		if task == nil || terminalDigestTask(*task) || task.UpdatedAt != item.UpdatedAt || task.State != item.TaskState {
			return errors.New("digest task changed")
		}
		sessions, readErr := digestPages(func(page pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
			return b.host().Sessions().List(ctx, pluginsdk.SessionFilter{TaskIDs: []string{task.ID}}, page)
		})
		if readErr != nil {
			return readErr
		}
		matched := false
		for _, s := range sessions {
			if s.ID == item.Target.SessionID {
				category, categoryErr := b.digestCategory(ctx, *task, s)
				if categoryErr != nil {
					return categoryErr
				}
				matched = s.State == item.SessionState && category == item.Category
				break
			}
		}
		if !matched {
			return errors.New("digest attention changed")
		}
	}
	if record.Text == "" {
		record.Status = "empty"
	} else {
		if err = b.postOnce(ctx, latest, "daily-"+key, p.Channel, "", record.Text); err != nil {
			return err
		}
		record.Status = "sent"
	}
	return writeConversation("digest-days", key, record, false)
}
