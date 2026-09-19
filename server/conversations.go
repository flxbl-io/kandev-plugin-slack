package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

type conversationBridge struct {
	host func() pluginsdk.Host
	wake chan struct{}
}
type conversationInbox struct {
	Team, App, Event, Channel, Thread, User, Text, TS, Receipt string
	Created                                                    time.Time
}
type conversationBinding struct {
	Team, User, Channel, Thread, Key, Fingerprint, SourceTask, SourceSession, SourceInvocation string
	Target                                                                                     pluginsdk.ConversationTarget
}

func (b *conversationBridge) settings(ctx context.Context) (conversationSettings, error) {
	h := b.host()
	if h == nil {
		return conversationSettings{}, errors.New("host unavailable")
	}
	raw, err := h.GetConfig(ctx)
	if err != nil {
		return conversationSettings{}, err
	}
	return loadConversations(raw)
}
func (b *conversationBridge) persist(ctx context.Context, raw json.RawMessage) error {
	if len(raw) > 65536 {
		return nil
	}
	var p eventsAPIPayload
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	e := p.Event
	if e.Type != "message" || e.ChannelType != "im" || e.Subtype != "" || e.BotID != "" || e.User == "" || e.TS == "" || e.Text == "" || len(e.Text) > 32000 || p.ExternalShared || !strings.HasPrefix(e.Channel, "D") {
		return nil
	}
	c, err := b.settings(ctx)
	if err != nil {
		return nil
	}
	if p.TeamID != c.Team || p.AppID != c.App || p.EventID == "" {
		return nil
	}
	key := notificationDigest(p.TeamID, p.EventID)
	var old conversationInbox
	if err = readConversation("done", key, &old); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	dir, err := conversationDir("inbox")
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) >= 128 {
		if readConversation("inbox", key, &old) == nil {
			return nil
		}
		return errors.New("conversation inbox full")
	}
	record := conversationInbox{Team: p.TeamID, App: p.AppID, Event: p.EventID, Channel: e.Channel, Thread: e.ThreadTS, User: e.User, Text: e.Text, TS: e.TS, Created: time.Now().UTC()}
	if err = writeConversation("inbox", key, record, true); err != nil && !os.IsExist(err) {
		return err
	}
	if b.wake != nil {
		select {
		case b.wake <- struct{}{}:
		default:
		}
	}
	return nil
}
func (b *conversationBridge) run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-b.wake:
		}
		b.reconcile(ctx)
	}
}
func (b *conversationBridge) reconcile(ctx context.Context) {
	c, err := b.settings(ctx)
	if err != nil {
		return
	}
	dir, err := conversationDir("inbox")
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type pending struct {
		key  string
		item conversationInbox
	}
	var pendingItems []pending
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		var item conversationInbox
		if readConversation("inbox", key, &item) == nil {
			pendingItems = append(pendingItems, pending{key, item})
		}
	}
	sort.Slice(pendingItems, func(i, j int) bool { return pendingItems[i].item.Created.Before(pendingItems[j].item.Created) })
	visited := map[string]bool{}
	for _, pending := range pendingItems {
		if ctx.Err() != nil {
			return
		}
		item := pending.item
		threadKey := notificationDigest(item.Team, item.Channel, item.Thread)
		if visited[threadKey] {
			continue
		}
		visited[threadKey] = true
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = b.process(callCtx, c, pending.key, &item)
		cancel()
		if err != nil {
			log.Printf("slack: conversation %s awaits reconciliation", pending.key)
		}
	}
}
