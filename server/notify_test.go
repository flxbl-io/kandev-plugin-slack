package main

import (
	"context"
	"encoding/json"
	"fmt"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotificationRejectsInvalidRecipient(t *testing.T) {
	p := newSlackPlugin(context.Background())
	result, err := p.InvokeAgentTool(context.Background(), &pluginsdk.AgentToolRequest{
		Name: "notify_user", Context: pluginsdk.AgentToolContext{TaskID: "task", SessionID: "session", WorkspaceID: "workspace", Surface: "kanban-task"},
		Arguments: map[string]any{"user_id": "C12345678", "text": "Review needed", "idempotency_key": "episode-1"},
	})
	if err != nil || result == nil || !result.IsError || result.StructuredContent["status"] != "invalid_request" {
		t.Fatalf("got %+v, %v; want invalid_request", result, err)
	}
}

func notificationRequest() *pluginsdk.AgentToolRequest {
	return &pluginsdk.AgentToolRequest{Name: "notify_user", Context: pluginsdk.AgentToolContext{TaskID: "task", SessionID: "session", WorkspaceID: "workspace", Surface: "kanban-task"}, Arguments: map[string]any{"user_id": "U12345678", "text": "Review needed: https://workfloor.example/task/1", "idempotency_key": "episode-1"}}
}

func notificationPlugin() *slackPlugin {
	p := newSlackPlugin(context.Background())
	p.UnimplementedPlugin.SetHost(newFakeHost(map[string]any{"bot_token": "xoxb-test-secret"}))
	return p
}

func TestNotificationDeliveryAndRestartDedup(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/chat.postMessage" || r.Header.Get("Authorization") != "Bearer xoxb-test-secret" {
			t.Errorf("unexpected Slack request")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("channel") != "U12345678" || r.Form.Get("mrkdwn") != "false" || r.Form.Get("parse") != "none" {
			t.Errorf("unsafe message parameters: %v", r.Form)
		}
		fmt.Fprint(w, `{"ok":true,"channel":"D12345678","ts":"12345.000001"}`)
	}))
	defer srv.Close()
	original := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = original })
	for i := 0; i < 2; i++ {
		got, err := notificationPlugin().InvokeAgentTool(context.Background(), notificationRequest())
		if err != nil || got.IsError || got.StructuredContent["status"] != "sent" || got.StructuredContent["duplicate"] != (i == 1) || got.StructuredContent["channel"] != "D12345678" {
			t.Fatalf("result %+v, %v", got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("sent %d messages", calls)
	}
	req := notificationRequest()
	req.Arguments["text"] = "Different message"
	got, _ := notificationPlugin().InvokeAgentTool(context.Background(), req)
	if got.StructuredContent["status"] != "idempotency_conflict" || calls != 1 {
		t.Fatalf("changed payload: %+v calls %d", got, calls)
	}
}

func TestNotificationFailuresAreExplicitAndRedacted(t *testing.T) {
	cases := []struct {
		name, body, want string
		httpStatus       int
	}{
		{"scope", `{"ok":false,"error":"missing_scope"}`, "missing_scope", 200},
		{"revoked", `{"ok":false,"error":"token_revoked"}`, "token_revoked", 200},
		{"inactive", `{"ok":false,"error":"account_inactive"}`, "account_inactive", 200},
		{"unreachable", `{"ok":false,"error":"channel_not_found"}`, "channel_not_found", 200},
		{"rate", `{"ok":false,"error":"ratelimited"}`, "rate_limited", 429},
		{"internal", `{"ok":false,"error":"internal_error"}`, "unknown", 200},
		{"invalidjson", `xoxb-test-secret`, "unknown", 200},
		{"upstream", `xoxb-test-secret`, "unknown", 503},
		{"unexpected", `{"ok":false,"error":"xoxb-test-secret"}`, "unknown", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", "27")
				w.WriteHeader(tc.httpStatus)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			original := slackAPIBase
			slackAPIBase = srv.URL
			t.Cleanup(func() { slackAPIBase = original })
			for i := 0; i < 2; i++ {
				got, _ := notificationPlugin().InvokeAgentTool(context.Background(), notificationRequest())
				if !got.IsError || got.StructuredContent["status"] != tc.want {
					t.Fatalf("got %+v want %s", got, tc.want)
				}
				if strings.Contains(fmt.Sprint(got), "xoxb-test-secret") {
					t.Fatal("credential reflected")
				}
				if tc.want == "rate_limited" && fmt.Sprint(got.StructuredContent["retry_after_seconds"]) != "27" {
					t.Fatalf("missing retry information: %+v", got)
				}
			}
			if calls != 1 {
				t.Fatalf("retried terminal/ambiguous result %d", calls)
			}
		})
	}
}

func TestNotificationConcurrentClaimsAndWorkspaceScope(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		fmt.Fprint(w, `{"ok":true,"channel":"D12345678","ts":"12345.000001"}`)
	}))
	defer srv.Close()
	original := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = original })
	done := make(chan *pluginsdk.AgentToolResult)
	go func() {
		got, _ := notificationPlugin().InvokeAgentTool(context.Background(), notificationRequest())
		done <- got
	}()
	<-entered
	got, _ := notificationPlugin().InvokeAgentTool(context.Background(), notificationRequest())
	close(release)
	first := <-done
	if got.StructuredContent["status"] != "unknown" || !got.IsError || first.IsError || calls.Load() != 1 {
		t.Fatalf("first %+v second %+v calls %d", first, got, calls.Load())
	}
	req := notificationRequest()
	req.Context.WorkspaceID = "other-workspace"
	got, _ = notificationPlugin().InvokeAgentTool(context.Background(), req)
	if got.IsError || calls.Load() != 2 {
		t.Fatalf("workspace scope %+v calls %d", got, calls.Load())
	}
}

func TestNotificationValidationAndMissingConfiguration(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	for _, text := range []string{"", "<@U12345678>", "<!here>", "@everyone", strings.Repeat("x", 4001), "\x00", "\xff"} {
		req := notificationRequest()
		req.Arguments["text"] = text
		got, _ := notificationPlugin().InvokeAgentTool(context.Background(), req)
		if got.StructuredContent["status"] != "invalid_request" {
			t.Fatalf("invalid text accepted: %+v", got)
		}
	}
	req := notificationRequest()
	req.Context.WorkspaceID = ""
	got, _ := notificationPlugin().InvokeAgentTool(context.Background(), req)
	if got.StructuredContent["status"] != "invalid_request" {
		t.Fatalf("missing context accepted: %+v", got)
	}
	p := newSlackPlugin(context.Background())
	p.UnimplementedPlugin.SetHost(newFakeHost(map[string]any{"session_token": "xoxc-private"}))
	got, _ = p.InvokeAgentTool(context.Background(), notificationRequest())
	if got.StructuredContent["status"] != "not_configured" {
		t.Fatalf("browser token must not send: %+v", got)
	}
}

func TestNotificationTransportCancellationStaysAmbiguous(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release }))
	defer srv.Close()
	original := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = original })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan *pluginsdk.AgentToolResult)
	go func() { got, _ := notificationPlugin().InvokeAgentTool(ctx, notificationRequest()); done <- got }()
	<-entered
	cancel()
	got := <-done
	close(release)
	if got.StructuredContent["status"] != "unknown" {
		t.Fatalf("cancelled delivery %+v", got)
	}
	again, _ := notificationPlugin().InvokeAgentTool(context.Background(), notificationRequest())
	if again.StructuredContent["status"] != "unknown" || again.StructuredContent["duplicate"] != true {
		t.Fatalf("ambiguous retry %+v", again)
	}
}

func TestNotificationJournalCrashAndCorruptionFailClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", root)
	p := notificationPlugin()
	// Missing configuration produces a journal without making a network request.
	p.UnimplementedPlugin.SetHost(newFakeHost(nil))
	if _, err := p.InvokeAgentTool(context.Background(), notificationRequest()); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "notifications-v1", "*.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("journal paths %v %v", paths, err)
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	pending := strings.Split(string(raw), "\n")[0] + "\n"
	for _, body := range []string{"", pending, pending + `{"result":`, "invalid\n"} {
		if err := os.WriteFile(paths[0], []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		got, _ := notificationPlugin().InvokeAgentTool(context.Background(), notificationRequest())
		if got.StructuredContent["status"] != "unknown" {
			t.Fatalf("crash/corrupt journal %+v", got)
		}
	}
}

func TestNotificationNeverFollowsRedirectOrRetries(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "/redirect")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	original := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = original })
	got, _ := notificationPlugin().InvokeAgentTool(context.Background(), notificationRequest())
	if calls != 1 || got.StructuredContent["status"] != "unknown" {
		t.Fatalf("calls %d result %+v", calls, got)
	}
}

// Exercise the public SDK wire boundary used by a supervised host process.
func TestNotificationAgentToolGRPCRoundTrip(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gp := &pluginsdk.GRPCPlugin{Impl: newSlackPlugin(ctx), Host: newFakeHost(nil), HostDialTimeout: 5 * time.Second}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{pluginsdk.PluginMapKey: gp})
	defer client.Close()
	defer server.Stop()
	raw, err := client.Dispense(pluginsdk.PluginMapKey)
	if err != nil {
		t.Fatal(err)
	}
	remote := raw.(*pluginsdk.RemotePlugin)
	got, err := remote.InvokeAgentTool(context.Background(), notificationRequest())
	if err != nil || got.StructuredContent["status"] != "not_configured" {
		t.Fatalf("RPC result %+v %v", got, err)
	}
}

// Kandev 0.95 serializes error results through MCP's text-only error helper.
// Preserve the full safe payload in fallback text for these host versions.
func TestNotificationErrorFallbackRetainsStructuredStatus(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	p := newSlackPlugin(context.Background())
	for i := 0; i < 2; i++ {
		got, _ := p.InvokeAgentTool(context.Background(), notificationRequest())
		var data map[string]any
		if err := json.Unmarshal([]byte(got.Text), &data); err != nil || data["status"] != "not_configured" || data["duplicate"] != (i == 1) {
			t.Fatalf("fallback loses result: %+v", got)
		}
	}
}
