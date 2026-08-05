package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestHasCommandPrefix(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{name: "exact", text: "!kandev", want: true},
		{name: "with instruction", text: "!kandev fix the login bug", want: true},
		{name: "colon separator", text: "!kandev: fix it", want: true},
		{name: "comma separator", text: "!kandev, fix it", want: true},
		{name: "leading whitespace", text: "   !kandev fix it", want: true},
		{name: "slack quote rendering", text: "> !kandev fix it", want: true},
		{name: "case insensitive", text: "!KanDev fix it", want: true},
		{name: "longer word does not match", text: "!kandevish fix it", want: false},
		{name: "prefix mid-message does not match", text: "please !kandev fix it", want: false},
		{name: "unrelated", text: "just chatting", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasCommandPrefix(tc.text, "!kandev"); got != tc.want {
				t.Fatalf("hasCommandPrefix(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestStripCommandPrefix(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{text: "!kandev fix the login bug", want: "fix the login bug"},
		{text: "!kandev: fix it", want: "fix it"},
		{text: "!kandev,   fix it", want: "fix it"},
		{text: "> !kandev fix it", want: "fix it"},
		{text: "!KANDEV fix it", want: "fix it"},
		{text: "!kandev", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := stripCommandPrefix(tc.text, "!kandev"); got != tc.want {
				t.Fatalf("stripCommandPrefix(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// Slack's `in:` modifiers combine with AND, so two channels in one query
// match nothing — each channel needs its own search.
func TestSearchQueriesOnePerChannel(t *testing.T) {
	cfg := &config{CommandPrefix: "!kandev", Channels: []string{"C1", "C2"}}
	got := searchQueries(cfg, "U1")
	if len(got) != 2 {
		t.Fatalf("got %d queries, want one per channel: %v", len(got), got)
	}
	if !strings.Contains(got[0], "in:<#C1>") || !strings.Contains(got[1], "in:<#C2>") {
		t.Fatalf("queries = %v", got)
	}
	for _, q := range got {
		if !strings.Contains(q, "from:<@U1>") || !strings.Contains(q, `"!kandev"`) {
			t.Fatalf("query %q lost the author or prefix filter", q)
		}
	}
}

func TestSearchQueriesWorkspaceWideWhenNoChannels(t *testing.T) {
	got := searchQueries(&config{CommandPrefix: "!kandev"}, "U1")
	if len(got) != 1 || strings.Contains(got[0], "in:") {
		t.Fatalf("queries = %v, want a single unscoped query", got)
	}
}

// Search results span channels, so the search modes keep one watermark; bot
// mode reads each channel's history independently and needs one per channel.
func TestAdvanceUsesTheRightWatermarkKey(t *testing.T) {
	marks := map[string]string{}
	advance(marks, &config{Mode: authModeUser}, message{ChannelID: "C1", TS: "2.0"})
	if marks[searchWatermarkKey] != "2.0" {
		t.Fatalf("search watermark = %q, want 2.0", marks[searchWatermarkKey])
	}
	if _, ok := marks["C1"]; ok {
		t.Fatal("search mode must not write a per-channel watermark")
	}

	marks = map[string]string{}
	advance(marks, &config{Mode: authModeBot}, message{ChannelID: "C1", TS: "2.0"})
	if marks["C1"] != "2.0" {
		t.Fatalf("channel watermark = %q, want 2.0", marks["C1"])
	}
}

// Messages can arrive out of order across channels; the watermark must only
// ever move forward.
func TestAdvanceNeverMovesBackwards(t *testing.T) {
	marks := map[string]string{searchWatermarkKey: "5.0"}
	advance(marks, &config{Mode: authModeUser}, message{TS: "3.0"})
	if marks[searchWatermarkKey] != "5.0" {
		t.Fatalf("watermark = %q, want it to stay at 5.0", marks[searchWatermarkKey])
	}
}

// collect must hand the caller only prefixed messages, oldest first, so a
// batch is triaged in the order the user sent it.
func TestCollectFiltersAndOrders(t *testing.T) {
	body := `{"ok":true,"messages":{"matches":[
		{"ts":"3.0","text":"!kandev third","channel":{"id":"C1"}},
		{"ts":"1.0","text":"unrelated chatter","channel":{"id":"C1"}},
		{"ts":"2.0","text":"!kandev second","channel":{"id":"C1"}}]}}`
	c, _ := newTestClient(t, body)
	cfg := &config{Mode: authModeUser, CommandPrefix: "!kandev"}
	got, err := collect(context.Background(), c, cfg, "U1", map[string]string{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d matches, want the 2 prefixed ones: %+v", len(got), got)
	}
	if got[0].TS != "2.0" || got[1].TS != "3.0" {
		t.Fatalf("order = %s, %s; want oldest first", got[0].TS, got[1].TS)
	}
}

func TestCollectHonoursTheWatermark(t *testing.T) {
	body := `{"ok":true,"messages":{"matches":[
		{"ts":"3.0","text":"!kandev newer","channel":{"id":"C1"}},
		{"ts":"1.0","text":"!kandev already done","channel":{"id":"C1"}}]}}`
	c, _ := newTestClient(t, body)
	cfg := &config{Mode: authModeUser, CommandPrefix: "!kandev"}
	got, err := collect(context.Background(), c, cfg, "U1", map[string]string{searchWatermarkKey: "2.0"})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 1 || got[0].TS != "3.0" {
		t.Fatalf("got %+v, want only the message after the watermark", got)
	}
}

// Without a probed user id the search query would be unscoped and match every
// `!kandev` in the workspace, including other people's.
func TestCollectBySearchRequiresAnAuthenticatedUser(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":true,"messages":{"matches":[]}}`)
	cfg := &config{Mode: authModeUser, CommandPrefix: "!kandev"}
	_, err := collect(context.Background(), c, cfg, "", map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "authenticated Slack user id") {
		t.Fatalf("collect = %v, want the missing-user-id error", err)
	}
}

func TestCollectByHistoryReadsEveryConfiguredChannel(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true,"messages":[{"ts":"2.0","text":"!kandev do it","user":"U1"}]}`)
	cfg := &config{Mode: authModeBot, CommandPrefix: "!kandev", Channels: []string{"C1", "C2"}}
	got, err := collect(context.Background(), c, cfg, "", map[string]string{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d matches, want one per channel", len(got))
	}
	if rec.Method != "conversations.history" {
		t.Fatalf("called %q, want conversations.history for a bot token", rec.Method)
	}
}

func TestCredentialFingerprintChangesWithTheToken(t *testing.T) {
	a := credentialFingerprint(&config{Token: "xoxc-aaaa", Cookie: "c"})
	b := credentialFingerprint(&config{Token: "xoxc-bbbb", Cookie: "c"})
	if a == b {
		t.Fatal("rotating the token must invalidate the probe fingerprint")
	}
}

// The fingerprint is compared on every scan and can reach a log; it must not
// be reversible into the credential.
func TestCredentialFingerprintDoesNotEmbedTheSecret(t *testing.T) {
	token := "xoxc-super-secret-value"
	got := credentialFingerprint(&config{Token: token})
	if strings.Contains(got, token) || strings.Contains(got, "secret") {
		t.Fatalf("fingerprint %q leaks the token", got)
	}
}

func TestFirstLineTruncates(t *testing.T) {
	if got := firstLine("one\ntwo"); got != "one" {
		t.Fatalf("firstLine = %q, want the first line only", got)
	}
	long := strings.Repeat("x", 200)
	if got := firstLine(long); len(got) > 130 || !strings.HasSuffix(got, "…") {
		t.Fatalf("firstLine did not truncate: len=%d", len(got))
	}
}

// A queued request already covers the caller's intent, so repeated presses of
// "Scan now" must not block the webhook handler on a busy loop.
func TestScanNowIsNonBlocking(t *testing.T) {
	tr := newTrigger(func() pluginsdk.Host { return nil })
	done := make(chan struct{})
	go func() {
		for range 5 {
			tr.ScanNow()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ScanNow blocked")
	}
}

// A nil Host is the normal state between subprocess start and broker
// injection; a scan in that window must be a no-op, not a panic.
func TestRunScanToleratesAnUninjectedHost(t *testing.T) {
	newTrigger(func() pluginsdk.Host { return nil }).runScan(context.Background(), true)
}
