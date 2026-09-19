package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func digestEvent(id, ts, user, thread, text string) json.RawMessage {
	var p map[string]any
	_ = json.Unmarshal(dmEvent(id, thread), &p)
	e := p["event"].(map[string]any)
	e["text"] = text
	e["ts"] = ts
	e["user"] = user
	raw, _ := json.Marshal(p)
	return raw
}
func digestTestBridge(t *testing.T) (*conversationBridge, *fakeHost, *[]string) {
	t.Helper()
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	var posts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		posts = append(posts, r.Form.Get("text"))
		fmt.Fprintf(w, `{"ok":true,"channel":%q,"ts":"3.000001"}`, r.Form.Get("channel"))
	}))
	t.Cleanup(srv.Close)
	old := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = old })
	cfg := conversationConfig()
	cfg["digests_enabled"] = true
	cfg["conversations_enabled"] = false
	h := newFakeHost(cfg)
	return &conversationBridge{host: func() pluginsdk.Host { return h }}, h, &posts
}
func TestDigestCommandsPersistOnlyAuthenticatedPreference(t *testing.T) {
	b, _, posts := digestTestBridge(t)
	ctx := context.Background()
	if err := b.persist(ctx, digestEvent("EvDigest", "3.000001", "U12345678", "", "digest at 09:00 Australia/Melbourne weekdays")); err != nil {
		t.Fatal(err)
	}
	b.reconcile(ctx)
	var pref map[string]any
	if err := readConversation("digest-preferences", notificationDigest("T12345678", "U12345678"), &pref); err != nil {
		t.Fatalf("preference was not persisted: %v", err)
	}
	if pref["enabled"] != true || pref["actor"] != "human" || pref["timezone"] != "Australia/Melbourne" {
		t.Fatalf("wrong preference: %+v", pref)
	}
	if len(*posts) != 1 || !strings.Contains((*posts)[0], "09:00") {
		t.Fatalf("missing personal confirmation: %v", *posts)
	}
	// Replay after restart cannot change or reconfirm the setting.
	b.persist(ctx, digestEvent("EvDigest", "3.000001", "U12345678", "", "digest at 09:00 Australia/Melbourne weekdays"))
	b.reconcile(ctx)
	if len(*posts) != 1 {
		t.Fatal("replayed command sent duplicate confirmation")
	}
	// An older event arriving late cannot re-enable a newer opt-out.
	b.persist(ctx, digestEvent("EvOff", "5.000001", "U12345678", "", "digest off"))
	b.reconcile(ctx)
	b.persist(ctx, digestEvent("EvOld", "4.000001", "U12345678", "", "digest at 10:00 UTC daily"))
	b.reconcile(ctx)
	readConversation("digest-preferences", notificationDigest("T12345678", "U12345678"), &pref)
	if pref["enabled"] != false {
		t.Fatal("late event overwrote opt-out")
	}
}
func TestDigestCommandsRejectInvalidSettingsAndUnknownUsers(t *testing.T) {
	for _, text := range []string{"digest at 25:00 UTC daily", "digest at 09:00 Made/Up daily", "digest at 09:00 UTC monthly", "digest at 09:00 UTC daily U99999999"} {
		t.Run(text, func(t *testing.T) {
			b, _, _ := digestTestBridge(t)
			b.persist(context.Background(), digestEvent("Ev", "3.000001", "U12345678", "", text))
			b.reconcile(context.Background())
			var pref map[string]any
			if readConversation("digest-preferences", notificationDigest("T12345678", "U12345678"), &pref) == nil {
				t.Fatal("invalid preference saved")
			}
		})
	}
	b, _, _ := digestTestBridge(t)
	b.persist(context.Background(), digestEvent("EvUnknown", "3.000001", "U99999999", "", "digest at 09:00 UTC daily"))
	b.reconcile(context.Background())
	var pref map[string]any
	if readConversation("digest-preferences", notificationDigest("T12345678", "U99999999"), &pref) == nil {
		t.Fatal("unmapped user configured a digest")
	}
}

func TestDigestPreferencesAreIndependentAndThreadRepliesRemainConversation(t *testing.T) {
	b, h, _ := digestTestBridge(t)
	h.config["conversation_users"] = `{"T12345678:U12345678":"human","T12345678:U87654321":"other"}`
	for _, v := range []struct{ user, at string }{{"U12345678", "09:00"}, {"U87654321", "17:00"}} {
		b.persist(context.Background(), digestEvent("Ev"+v.user, "5.000001", v.user, "", "digest at "+v.at+" UTC daily"))
		b.reconcile(context.Background())
		var p digestPreference
		if err := readConversation("digest-preferences", notificationDigest("T12345678", v.user), &p); err != nil || p.At != v.at {
			t.Fatalf("wrong personal time %+v %v", p, err)
		}
	}
	if isDigestCommand(&conversationInbox{Thread: "1.000001", Text: "digest off"}) {
		t.Fatal("card thread captured as a settings command")
	}
}

func TestDigestPreferenceExplainsMissingHostSupport(t *testing.T) {
	b, _, posts := digestTestBridge(t)
	b.persist(context.Background(), digestEvent("EvNeedsHost", "3.000001", "U12345678", "", "digest at 09:00 UTC daily"))
	b.reconcile(context.Background())
	if len(*posts) != 1 || !strings.Contains((*posts)[0], "host") {
		t.Fatalf("unsupported delivery looked ready: %v", *posts)
	}
}
