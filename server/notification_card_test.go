package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cardArguments() map[string]any {
	return map[string]any{"title": "fix(engine): source package and in-project dependency crash the build", "workspace_name": "Codev", "repository": "flxbl-io/sfp-pro", "issue_number": 2419, "attention": "input", "summary": "Needs your decision on the next step.", "workfloor_url": "https://workfloor.example/?workspaceId=workspace&taskId=task&sessionId=session", "issue_url": "https://github.com/flxbl-io/sfp-pro/issues/2419"}
}
func cardPlugin() *slackPlugin {
	p := newSlackPlugin(context.Background())
	p.UnimplementedPlugin.SetHost(newFakeHost(map[string]any{"bot_token": "xoxb-test", "workfloor_url": "https://workfloor.example"}))
	return p
}
func captureCardPosts(t *testing.T) *[]url.Values {
	t.Helper()
	posts := []url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		posts = append(posts, r.Form)
		fmt.Fprint(w, `{"ok":true,"channel":"D12345678","ts":"1.000001"}`)
	}))
	t.Cleanup(srv.Close)
	old := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = old })
	return &posts
}

// @covers AC-SLACK-CARD-001.1, AC-SLACK-CARD-001.2, AC-SLACK-CARD-001.3
func TestNotificationCardDeliveryAndFallback(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	posts := captureCardPosts(t)
	req := notificationRequest()
	card := cardArguments()
	card["title"] = strings.Repeat("Long title *literal* & ", 12)
	req.Arguments["card"] = card
	got, _ := cardPlugin().InvokeAgentTool(context.Background(), req)
	if got.IsError {
		t.Fatalf("card rejected: %+v", got)
	}
	if len(*posts) != 1 {
		t.Fatalf("posts=%d", len(*posts))
	}
	f := (*posts)[0]
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(f.Get("blocks")), &blocks); err != nil {
		t.Fatalf("missing Slack blocks: %v", err)
	}
	if blocks[0]["type"] != "divider" {
		t.Fatal("card boundary must precede the title")
	}
	if len(blocks) != 5 {
		t.Fatalf("blocks=%d", len(blocks))
	}
	raw := f.Get("blocks")
	for _, v := range []string{"#2419", "Open in Workfloor", "View issue", "Needs your decision", "Codev"} {
		if !strings.Contains(raw, v) {
			t.Errorf("card missing %s", v)
		}
	}
	if strings.Contains(raw, "View PR") || strings.Contains(raw, "Reply in this thread") {
		t.Fatal("unavailable action advertised")
	}
	for _, v := range []string{card["title"].(string), card["workfloor_url"].(string), card["issue_url"].(string), card["summary"].(string)} {
		if !strings.Contains(f.Get("text"), v) {
			t.Errorf("fallback missing %s", v)
		}
	}
	if f.Get("mrkdwn") != "false" || f.Get("link_names") != "false" || f.Get("unfurl_links") != "false" || f.Get("unfurl_media") != "false" || f.Get("parse") == "none" {
		t.Fatal("unsafe delivery flags")
	}
}

// @covers AC-SLACK-CARD-002.1, AC-SLACK-CARD-002.2
func TestNotificationCardRestartAndMetadataConflict(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	posts := captureCardPosts(t)
	req := notificationRequest()
	req.Arguments["card"] = cardArguments()
	for i := 0; i < 2; i++ {
		got, _ := cardPlugin().InvokeAgentTool(context.Background(), req)
		if got.IsError || got.StructuredContent["duplicate"] != (i == 1) {
			t.Fatalf("delivery %+v", got)
		}
	}
	card := req.Arguments["card"].(map[string]any)
	card["pr_url"] = "https://github.com/flxbl-io/sfp-pro/pull/2424"
	got, _ := cardPlugin().InvokeAgentTool(context.Background(), req)
	if got.StructuredContent["status"] != "idempotency_conflict" || len(*posts) != 1 {
		t.Fatalf("changed destination not fenced: %+v", got)
	}
}

// @covers AC-SLACK-CARD-002.2
func TestNotificationCardRejectsUnsafeMetadataBeforeJournal(t *testing.T) {
	cases := map[string]func(map[string]any){
		"zero issue": func(c map[string]any) { c["issue_number"] = 0 },
		"encoded control": func(c map[string]any) {
			c["workfloor_url"] = "https://workfloor.example/?workspaceId=workspace&taskId=task%0A&sessionId=session"
		},
		"explicit empty link": func(c map[string]any) { c["pr_url"] = "" },
		"foreign origin": func(c map[string]any) {
			c["workfloor_url"] = "https://evil.example/?workspaceId=workspace&taskId=task&sessionId=session"
		},
		"foreign workspace": func(c map[string]any) {
			c["workfloor_url"] = "https://workfloor.example/?workspaceId=another&taskId=task&sessionId=session"
		},
		"duplicate query":   func(c map[string]any) { c["workfloor_url"] = c["workfloor_url"].(string) + "&taskId=other" },
		"non HTTPS":         func(c map[string]any) { c["issue_url"] = "http://github.com/flxbl-io/sfp-pro/issues/2419" },
		"userinfo":          func(c map[string]any) { c["issue_url"] = "https://user@github.com/flxbl-io/sfp-pro/issues/2419" },
		"wrong github path": func(c map[string]any) { c["pr_url"] = "https://github.com/flxbl-io/sfp-pro/settings" },
		"wrong issue":       func(c map[string]any) { c["issue_url"] = "https://github.com/flxbl-io/sfp-pro/issues/2424" },
		"fractional issue":  func(c map[string]any) { c["issue_number"] = 1.5 },
		"negative issue":    func(c map[string]any) { c["issue_number"] = -1 },
		"mass mention":      func(c map[string]any) { c["summary"] = "Please @channel review" },
		"markup mention":    func(c map[string]any) { c["title"] = "<!here>" },
		"control":           func(c map[string]any) { c["title"] = "bad\x00title" },
		"oversize":          func(c map[string]any) { c["summary"] = strings.Repeat("x", 1201) },
		"unknown field":     func(c map[string]any) { c["blocks"] = []any{} },
		"missing field":     func(c map[string]any) { delete(c, "summary") },
		"bad attention":     func(c map[string]any) { c["attention"] = "ci-passed" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("KANDEV_PLUGIN_DATA_DIR", root)
			posts := captureCardPosts(t)
			req := notificationRequest()
			c := cardArguments()
			mutate(c)
			req.Arguments["card"] = c
			got, _ := cardPlugin().InvokeAgentTool(context.Background(), req)
			if got.StructuredContent["status"] != "invalid_request" {
				t.Fatalf("accepted bad metadata %+v", got)
			}
			if len(*posts) != 0 {
				t.Fatal("invalid card sent")
			}
			if _, err := os.Stat(filepath.Join(root, "notifications-v1")); !os.IsNotExist(err) {
				t.Fatal("invalid input consumed key")
			}
		})
	}
}

// @covers AC-SLACK-CARD-002.1
func TestBoundNotificationCardPreservesBindingAndFooter(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	posts := captureCardPosts(t)
	h := &conversationTestHost{fakeHost: newFakeHost(conversationConfig())}
	h.config["notification_brand_name"] = "Flux"
	h.config["notification_brand_icon_url"] = "https://cdn.example/flux.png"
	p := newSlackPlugin(context.Background())
	p.UnimplementedPlugin.SetHost(h)
	req := notificationRequest()
	req.Name = "notify_task_user"
	req.Arguments["task_id"] = "task"
	req.Arguments["session_id"] = "session"
	req.Arguments["card"] = cardArguments()
	got, _ := p.InvokeAgentTool(context.Background(), req)
	if got.IsError {
		t.Fatalf("bound card failed %+v", got)
	}
	if len(*posts) != 1 || !strings.Contains((*posts)[0].Get("blocks"), "Reply in this thread") {
		t.Fatal("missing bound-thread footer")
	}
	if !strings.Contains((*posts)[0].Get("text"), "Questions and permissions") {
		t.Fatal("fallback omitted limitation")
	}
	if err := p.bridge.persist(context.Background(), dmEvent("EvCard", "1.000001")); err != nil {
		t.Fatal(err)
	}
	p.bridge.reconcile(context.Background())
	if len(h.submitted) != 1 || h.submitted[0].Target.TaskID != "task" {
		t.Fatal("card reply binding lost")
	}
	h.config["notification_brand_name"] = "Flux renamed"
	replay, _ := p.InvokeAgentTool(context.Background(), req)
	if replay.IsError || replay.StructuredContent["duplicate"] != true || len(*posts) != 1 {
		t.Fatalf("bound brand replay failed: %+v", replay)
	}
	req.Arguments["card"].(map[string]any)["summary"] = "Changed summary"
	again, _ := p.InvokeAgentTool(context.Background(), req)
	if again.StructuredContent["status"] != "idempotency_conflict" || len(*posts) != 1 {
		t.Fatalf("bound card content conflict not detected %+v", again)
	}
}

func TestNotificationCardOptionalLinksAndTrustedOrigin(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	posts := captureCardPosts(t)
	req := notificationRequest()
	card := cardArguments()
	req.Arguments["card"] = card
	got, _ := notificationPlugin().InvokeAgentTool(context.Background(), req)
	if got.StructuredContent["status"] != "not_configured" {
		t.Fatalf("missing trusted origin: %+v", got)
	}
	card["pr_url"] = "https://github.com/flxbl-io/sfp-pro/pull/2424"
	got, _ = cardPlugin().InvokeAgentTool(context.Background(), req)
	if got.IsError || len(*posts) != 1 {
		t.Fatalf("PR card %+v", got)
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte((*posts)[0].Get("blocks")), &blocks); err != nil {
		t.Fatal(err)
	}
	actions := blocks[len(blocks)-1]["elements"].([]any)
	if len(actions) != 3 {
		t.Fatalf("actions %d", len(actions))
	}
	for i, want := range []string{card["workfloor_url"].(string), card["issue_url"].(string), card["pr_url"].(string)} {
		if actions[i].(map[string]any)["url"] != want {
			t.Fatalf("button %d changed destination", i)
		}
	}
}

func TestNotificationCardRejectsMismatchedBoundTarget(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	posts := captureCardPosts(t)
	h := &conversationTestHost{fakeHost: newFakeHost(conversationConfig())}
	p := newSlackPlugin(context.Background())
	p.UnimplementedPlugin.SetHost(h)
	req := notificationRequest()
	req.Name = "notify_task_user"
	req.Arguments["task_id"] = "different"
	req.Arguments["session_id"] = "session"
	req.Context.Surface = "automation"
	req.Arguments["card"] = cardArguments()
	got, _ := p.InvokeAgentTool(context.Background(), req)
	if got.StructuredContent["status"] != "invalid_request" || len(*posts) != 0 {
		t.Fatalf("mismatched link accepted: %+v", got)
	}
}

func TestNotificationLegacyJournalSurvivesCardUpgrade(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", root)
	posts := captureCardPosts(t)
	req := notificationRequest()
	text := req.Arguments["text"].(string)
	dir := filepath.Join(root, "notifications-v1")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// A v0.4.1 committed receipt, before cards existed.
	record := notificationRecord{Fingerprint: notificationDigest(text), Result: map[string]any{"status": "sent", "recipient": "U12345678", "channel": "D12345678", "timestamp": "0.000001", "duplicate": false}}
	raw, _ := json.Marshal(record)
	path := filepath.Join(dir, notificationDigest("workspace", "U12345678", "episode-1")+".jsonl")
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	got, _ := cardPlugin().InvokeAgentTool(context.Background(), req)
	if got.IsError || got.StructuredContent["duplicate"] != true || len(*posts) != 0 {
		t.Fatalf("legacy receipt re-sent %+v", got)
	}
	req.Arguments["card"] = cardArguments()
	got, _ = cardPlugin().InvokeAgentTool(context.Background(), req)
	if got.StructuredContent["status"] != "idempotency_conflict" || len(*posts) != 0 {
		t.Fatalf("format upgrade resent old notification %+v", got)
	}
}

// @covers AC-SLACK-CARD-003.1, AC-SLACK-CARD-003.2, AC-SLACK-CARD-003.3
func TestNotificationCardBranding(t *testing.T) {
	for _, attention := range []string{"input", "review", "error"} {
		t.Run(attention, func(t *testing.T) {
			t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
			posts := captureCardPosts(t)
			p := cardPlugin()
			h := p.Host().(*fakeHost)
			h.config["notification_brand_name"] = "Flux"
			h.config["notification_brand_icon_url"] = "https://cdn.example/flux.png"
			req := notificationRequest()
			card := cardArguments()
			card["attention"] = attention
			req.Arguments["card"] = card
			for i := 0; i < 2; i++ {
				req.Arguments["idempotency_key"] = fmt.Sprintf("brand-%d", i)
				got, _ := p.InvokeAgentTool(context.Background(), req)
				if got.IsError {
					t.Fatalf("notification rejected: %+v", got)
				}
			}
			if len(*posts) != 2 {
				t.Fatalf("posts=%d", len(*posts))
			}
			for _, post := range *posts {
				var blocks []map[string]any
				if err := json.Unmarshal([]byte(post.Get("blocks")), &blocks); err != nil {
					t.Fatal(err)
				}
				if blocks[0]["type"] != "divider" {
					t.Fatal("each consecutive card needs a boundary before its identity")
				}
				if strings.Count(post.Get("blocks"), `"type":"divider"`) != 1 {
					t.Fatal("card should have one boundary, not an internal divider")
				}
				if blocks[1]["type"] != "context" {
					t.Fatalf("missing identity strip: %s", post.Get("blocks"))
				}
				elements := blocks[1]["elements"].([]any)
				if len(elements) != 2 {
					t.Fatalf("identity elements=%v", elements)
				}
				icon := elements[0].(map[string]any)
				label := elements[1].(map[string]any)
				if icon["type"] != "image" || icon["image_url"] != "https://cdn.example/flux.png" || icon["alt_text"] != "Flux" || label["type"] != "plain_text" || label["text"] != "Flux" {
					t.Fatalf("identity=%v", elements)
				}
				if blocks[2]["type"] != "rich_text" || !strings.HasPrefix(post.Get("text"), "Flux\n#2419") {
					t.Fatal("title or accessible identity missing")
				}
			}
			h.config["notification_brand_name"] = "Renamed"
			h.config["notification_brand_icon_url"] = "https://cdn.example/changed.png"
			again, _ := p.InvokeAgentTool(context.Background(), req)
			if again.IsError || again.StructuredContent["duplicate"] != true || len(*posts) != 2 {
				t.Fatalf("brand change replay=%+v", again)
			}
		})
	}
}

// @covers AC-SLACK-CARD-003.2
func TestNotificationCardBrandingDegradesSafely(t *testing.T) {
	cases := []struct {
		name     string
		brand    any
		icon     any
		wantName bool
		wantIcon bool
	}{
		{"name only", "Flux", "", true, false},
		{"icon only", "", "https://cdn.example/flux.png", false, false},
		{"unsafe name", "<!here>", "https://cdn.example/flux.png", false, false},
		{"long name", strings.Repeat("x", 81), "", false, false},
		{"wrong type", 42, "", false, false},
		{"insecure image", "Flux", "http://cdn.example/flux.png", true, false},
		{"image credentials", "Flux", "https://user:password@cdn.example/flux.png", true, false},
		{"image fragment", "Flux", "https://cdn.example/flux.png#x", true, false},
		{"image encoded control", "Flux", "https://cdn.example/flux.png?x=%0A", true, false},
		{"wrong image type", "Flux", 42, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := cardPlugin()
			h := p.Host().(*fakeHost)
			h.config["notification_brand_name"] = tc.brand
			h.config["notification_brand_icon_url"] = tc.icon
			req := notificationRequest()
			req.Arguments["card"] = cardArguments()
			message, status := p.notificationMessage(context.Background(), req, "Input needed", "")
			if status != "" {
				t.Fatalf("branding blocked delivery: %s", status)
			}
			if strings.HasPrefix(message.text, "Flux\n") != tc.wantName {
				t.Fatalf("fallback=%q", message.text)
			}
			if strings.Contains(message.blocks, `"type":"image"`) != tc.wantIcon {
				t.Fatalf("unsafe image rendered: %s", message.blocks)
			}
		})
	}
}

// @covers AC-SLACK-CARD-003.3
func TestNotificationCardBrandingPreservesUnbrandedReceiptAndPlainText(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	posts := captureCardPosts(t)
	p := cardPlugin()
	req := notificationRequest()
	req.Arguments["card"] = cardArguments()
	first, _ := p.InvokeAgentTool(context.Background(), req)
	if first.IsError {
		t.Fatalf("first delivery=%+v", first)
	}
	p = cardPlugin() // Same durable journal, newly configured plugin process.
	h := p.Host().(*fakeHost)
	h.config["notification_brand_name"] = "Flux"
	h.config["notification_brand_icon_url"] = "https://cdn.example/flux.png"
	replay, _ := p.InvokeAgentTool(context.Background(), req)
	if replay.IsError || replay.StructuredContent["duplicate"] != true || len(*posts) != 1 {
		t.Fatalf("unbranded receipt replay=%+v", replay)
	}
	delete(req.Arguments, "card")
	req.Arguments["idempotency_key"] = "text-only"
	textOnly, _ := p.InvokeAgentTool(context.Background(), req)
	if textOnly.IsError || len(*posts) != 2 {
		t.Fatalf("plain text=%+v", textOnly)
	}
	if (*posts)[1].Get("blocks") != "" || (*posts)[1].Get("text") != req.Arguments["text"] {
		t.Fatal("branding changed text-only delivery")
	}
}
