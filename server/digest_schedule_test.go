package main

import (
	"context"
	"errors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"strings"
	"testing"
	"time"
)

func TestDigestDuePersonalTimezoneDSTAndCatchup(t *testing.T) {
	p := digestPreference{Enabled: true, At: "09:00", Zone: "Australia/Melbourne", Days: "weekdays"}
	for _, tc := range []struct {
		at   string
		want bool
	}{{"2026-09-20T22:59:00Z", false}, {"2026-09-20T23:00:00Z", true}, {"2026-09-21T01:00:00Z", false}, {"2026-09-18T23:00:00Z", false}} {
		n, _ := time.Parse(time.RFC3339, tc.at)
		if got := digestDue(p, n); got != tc.want {
			t.Errorf("%s due=%v want %v", tc.at, got, tc.want)
		}
	}
	p.At = "02:30"
	p.Days = "daily" // nonexistent spring-forward time: catch up at 03:00
	n, _ := time.Parse(time.RFC3339, "2026-10-03T16:00:00Z")
	if !digestDue(p, n) {
		t.Error("missed spring-forward catchup")
	}
	p.Enabled = false
	if digestDue(p, n) {
		t.Error("disabled digest due")
	}
}

type digestHost struct {
	*fakeHost
	tasks       []pluginsdk.Task
	sessions    map[string][]pluginsdk.Session
	denied      map[string]bool
	fail        bool
	resolutions int
	revokeAt    int
}

func (h *digestHost) Tasks() pluginsdk.TaskReader                 { return digestTasks{h: h} }
func (h *digestHost) Sessions() pluginsdk.SessionReader           { return digestSessions{h: h} }
func (h *digestHost) Attention() pluginsdk.AttentionAccessor      { return h }
func (h *digestHost) Interactions() pluginsdk.InteractionAccessor { return digestInteractions{} }
func (h *digestHost) Resolve(_ context.Context, t pluginsdk.ConversationTarget) error {
	h.resolutions++
	if h.revokeAt > 0 && h.resolutions >= h.revokeAt || h.denied[t.TaskID] || t.ActorID != "human" || strings.HasPrefix(t.SessionID, "old") {
		return grpcstatus.Error(codes.PermissionDenied, "denied")
	}
	return nil
}

type digestTasks struct {
	pluginsdk.TaskReader
	h *digestHost
}

func (r digestTasks) List(_ context.Context, _ pluginsdk.TaskFilter, p pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error) {
	if r.h.fail {
		return nil, nil, errors.New("scan unavailable")
	}
	if p.Cursor == "second" {
		return r.h.tasks[1:], nil, nil
	}
	return r.h.tasks[:1], &pluginsdk.PageInfo{HasMore: true, NextCursor: "second"}, nil
}

type digestSessions struct {
	pluginsdk.SessionReader
	h *digestHost
}

func (r digestSessions) List(_ context.Context, f pluginsdk.SessionFilter, _ pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
	return r.h.sessions[f.TaskIDs[0]], nil, nil
}

type digestInteractions struct{ pluginsdk.InteractionAccessor }

func (digestInteractions) ListPending(context.Context, pluginsdk.InteractionFilter, pluginsdk.Page) ([]pluginsdk.Interaction, *pluginsdk.PageInfo, error) {
	return nil, nil, nil
}

func digestFixture(t *testing.T) (*conversationBridge, *digestHost, *[]string, time.Time) {
	b, f, posts := digestTestBridge(t)
	h := &digestHost{fakeHost: f, denied: map[string]bool{"foreign": true}, tasks: []pluginsdk.Task{
		{ID: "one", WorkspaceID: "ws-1", Title: "Fix callbacks", State: "WAITING_FOR_INPUT", Metadata: map[string]any{"issue_url": "https://github.com/flxbl-io/sfp-pro/issues/2375"}},
		{ID: "two", WorkspaceID: "ws-2", Title: "Recover validation", State: "FAILED"},
		{ID: "foreign", WorkspaceID: "ws-2", Title: "PRIVATE OTHER USER", State: "WAITING_FOR_INPUT"},
	}, sessions: map[string][]pluginsdk.Session{
		"one":     {{ID: "old-one", TaskID: "one", State: "FAILED"}, {ID: "one-session", TaskID: "one", State: "WAITING_FOR_INPUT"}},
		"two":     {{ID: "two-session", TaskID: "two", State: "FAILED"}},
		"foreign": {{ID: "foreign-session", TaskID: "foreign", State: "WAITING_FOR_INPUT"}},
	}}
	b.host = func() pluginsdk.Host { return h }
	p := digestPreference{Team: "T12345678", User: "U12345678", Actor: "human", Channel: "D12345678", Enabled: true, At: "09:00", Zone: "Australia/Melbourne", Days: "daily"}
	if err := writeConversation("digest-preferences", notificationDigest(p.Team, p.User), p, false); err != nil {
		t.Fatal(err)
	}
	n, _ := time.Parse(time.RFC3339, "2026-09-20T23:00:00Z")
	return b, h, posts, n
}
func TestDigestCombinesWorkspacesAndSurvivesRestart(t *testing.T) {
	b, h, posts, n := digestFixture(t)
	b.digestSweep(context.Background(), n)
	if len(*posts) != 1 || !strings.Contains((*posts)[0], "#2375 · Fix callbacks") || !strings.Contains((*posts)[0], "Recover validation") || strings.Contains((*posts)[0], "PRIVATE OTHER USER") {
		t.Fatalf("wrong digest: %v", *posts)
	}
	restarted := &conversationBridge{host: func() pluginsdk.Host { return h }}
	restarted.digestSweep(context.Background(), n.Add(time.Minute))
	if len(*posts) != 1 {
		t.Fatal("restart repeated today's digest")
	}
}
func TestDigestDoesNotDiscloseAfterRevocationOrIncompleteScan(t *testing.T) {
	for _, reason := range []string{"revoked", "scan"} {
		t.Run(reason, func(t *testing.T) {
			b, h, posts, n := digestFixture(t)
			if reason == "scan" {
				h.fail = true
			} else {
				h.revokeAt = 5
			}
			b.digestSweep(context.Background(), n)
			if len(*posts) != 0 {
				t.Fatalf("unsafe delivery: %v", *posts)
			}
		})
	}
}

func (r digestTasks) Get(_ context.Context, id string) (*pluginsdk.Task, error) {
	for _, t := range r.h.tasks {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, errors.New("missing")
}

func TestDigestEmptyCorruptAndAmbiguousStatesNeverResend(t *testing.T) {
	for _, scenario := range []string{"empty", "corrupt", "ambiguous"} {
		t.Run(scenario, func(t *testing.T) {
			b, h, posts, n := digestFixture(t)
			if scenario == "empty" {
				h.denied["one"] = true
				h.denied["two"] = true
			}
			dayKey := notificationDigest("digest-v1", "T12345678", "human", "2026-09-21")
			if scenario == "corrupt" {
				writeConversation("digest-days", dayKey, "not a digest object", false)
			}
			if scenario == "ambiguous" {
				writeConversation("outbox", notificationDigest("daily-"+dayKey), conversationDelivery{Status: "sending"}, false)
			}
			b.digestSweep(context.Background(), n)
			b.digestSweep(context.Background(), n.Add(time.Minute))
			if len(*posts) != 0 {
				t.Fatal("unexpected delivery")
			}
		})
	}
}
func TestDigestFormatBoundedIssueFirstAndNoMentions(t *testing.T) {
	var items []digestItem
	for i := 0; i < 100; i++ {
		items = append(items, digestItem{Title: digestPlain("@channel <secret> "+strings.Repeat("x", 400), 170), Workspace: "Workfloor", Category: "Input needed", Target: pluginsdk.ConversationTarget{WorkspaceID: "ws", TaskID: "task", SessionID: "session"}})
	}
	c, _ := loadConversations(conversationConfig())
	text := formatDigest(c, digestPreference{Zone: "UTC"}, "2026-09-21", items)
	if len(text) > 3500 || !strings.Contains(text, "additional tasks") || strings.Contains(text, "@channel") || strings.Contains(text, "<secret>") {
		t.Fatalf("unsafe or unbounded digest: %d chars", len(text))
	}
}

func TestDigestCurrentRunningSessionSuppressesHistoricalFailure(t *testing.T) {
	b, _, _ := digestTestBridge(t)
	h := &digestHost{fakeHost: newFakeHost(conversationConfig())}
	b.host = func() pluginsdk.Host { return h }
	category, err := b.digestCategory(context.Background(), pluginsdk.Task{ID: "task", State: "FAILED"}, pluginsdk.Session{ID: "current", State: "RUNNING"})
	if err != nil || category != "" {
		t.Fatalf("stale task failure surfaced while current session runs: %q %v", category, err)
	}
}
