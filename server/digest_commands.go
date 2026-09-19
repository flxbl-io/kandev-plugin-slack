package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type digestPreference struct {
	Team    string `json:"team"`
	User    string `json:"user"`
	Actor   string `json:"actor"`
	Channel string `json:"channel"`
	Enabled bool   `json:"enabled"`
	Zone    string `json:"timezone"`
	At      string `json:"at"`
	Days    string `json:"days"`
	EventTS string `json:"event_ts"`
}

const digestUsage = "DM me: digest at 09:00 Australia/Melbourne weekdays (or daily). Use digest status to check your schedule or digest off to disable it. Times are personal; nothing is enabled automatically."

func isDigestCommand(item *conversationInbox) bool {
	words := strings.Fields(strings.ToLower(item.Text))
	return item.Thread == "" && len(words) > 0 && words[0] == "digest"
}
func (b *conversationBridge) digestCommand(ctx context.Context, c conversationSettings, key string, item *conversationInbox) error {
	if !c.Digests {
		return b.guide(ctx, c, key, item, "Personal digests are disabled. Ask your Workfloor administrator to enable them.")
	}
	actor := c.actor(item.Team, item.User)
	if actor == "" {
		return b.guide(ctx, c, key, item, "Ask your Workfloor administrator to link your Slack account before configuring a digest.")
	}
	prefKey := notificationDigest(item.Team, item.User)
	var p digestPreference
	if err := readConversation("digest-preferences", prefKey, &p); err != nil && !os.IsNotExist(err) {
		return err
	}
	words := strings.Fields(item.Text)
	reply := digestUsage
	mutate := false
	switch {
	case len(words) == 2 && strings.EqualFold(words[1], "status"):
		reply = digestStatus(p)
	case len(words) == 2 && strings.EqualFold(words[1], "off"):
		p.Enabled = false
		mutate = true
		reply = "Your daily attention digest is switched off."
	case len(words) == 5 && strings.EqualFold(words[1], "at"):
		if validDigestTime(words[2], words[3], words[4]) == nil {
			p.At = words[2]
			p.Zone = words[3]
			p.Days = strings.ToLower(words[4])
			p.Enabled = true
			mutate = true
			reply = digestStatus(p)
			if !b.digestHostReady(ctx) {
				reply += "\nDelivery is waiting for the Workfloor host attention API and its read permission. Your preference is saved."
			}
		}
	}
	if mutate {
		ts, err := strconv.ParseFloat(item.TS, 64)
		old, _ := strconv.ParseFloat(p.EventTS, 64)
		if err != nil || ts <= 0 || math.IsNaN(ts) || math.IsInf(ts, 0) {
			return errors.New("invalid Slack timestamp")
		}
		if old > ts {
			return b.guide(ctx, c, key, item, "Ignored an older settings message; your newer preference is preserved.")
		}
		p.Team = item.Team
		p.User = item.User
		p.Actor = actor
		p.Channel = item.Channel
		p.EventTS = item.TS
		if err = writeConversation("digest-preferences", prefKey, p, false); err != nil {
			return err
		}
	}
	return b.guide(ctx, c, key, item, reply)
}
func validDigestTime(at, zone, days string) error {
	if len(at) != 5 || at[2] != ':' {
		return errors.New("use HH:MM")
	}
	t, err := time.Parse("15:04", at)
	if err != nil || t.Format("15:04") != at {
		return errors.New("invalid time")
	}
	if zone == "Local" || zone == "" {
		return errors.New("explicit timezone required")
	}
	if _, err = time.LoadLocation(zone); err != nil {
		return err
	}
	if days != "daily" && days != "weekdays" {
		return errors.New("choose daily or weekdays")
	}
	return nil
}
func digestStatus(p digestPreference) string {
	if !p.Enabled {
		return "Your daily attention digest is switched off. " + digestUsage
	}
	return fmt.Sprintf("Your attention digest is set for %s %s, %s, across your accessible workspaces. No message is sent when nothing needs your attention. Use digest off to disable it.", p.At, p.Zone, p.Days)
}

func (b *conversationBridge) digestHostReady(ctx context.Context) bool {
	api, ok := pluginsdk.Attention(b.host())
	if !ok {
		return false
	}
	return grpcstatus.Code(api.Resolve(ctx, pluginsdk.ConversationTarget{})) == codes.InvalidArgument
}
