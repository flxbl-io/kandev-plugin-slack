package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func conversationConfig() map[string]any {
	return map[string]any{"conversations_enabled": true, "conversation_team_id": "T12345678", "conversation_app_id": "A12345678", "conversation_users": `{"T12345678:U12345678":"human"}`, "workfloor_url": "https://workfloor.example", "bot_token": "xoxb-test"}
}
func dmEvent(id, thread string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"team_id": "T12345678", "api_app_id": "A12345678", "event_id": id, "event": map[string]any{"type": "message", "channel_type": "im", "channel": "D12345678", "user": "U12345678", "text": "Please continue", "ts": "2.000001", "thread_ts": thread}})
	return raw
}
func TestConversationInboxSurvivesReplay(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", root)
	h := newFakeHost(conversationConfig())
	b := &conversationBridge{host: func() pluginsdk.Host { return h }}
	for range 2 {
		if err := b.persist(context.Background(), dmEvent("Ev1", "1.000001")); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := os.ReadDir(filepath.Join(root, "conversations-v1", "inbox"))
	if len(files) != 1 {
		t.Fatalf("durable inbox contains %d events, want 1", len(files))
	}
}

type conversationTestHost struct {
	*fakeHost
	submitted []pluginsdk.ExternalMessageRequest
	denied    bool
	done      bool
}

func (h *conversationTestHost) Conversations() pluginsdk.ConversationAccessor { return h }
func (h *conversationTestHost) Resolve(_ context.Context, t pluginsdk.ConversationTarget) error {
	if h.denied || t.ActorID != "human" || t.WorkspaceID != "workspace" || t.TaskID != "task" || t.SessionID != "session" {
		return errors.New("denied")
	}
	return nil
}
func (h *conversationTestHost) Submit(_ context.Context, r pluginsdk.ExternalMessageRequest) (*pluginsdk.ExternalMessageReceipt, error) {
	h.submitted = append(h.submitted, r)
	return &pluginsdk.ExternalMessageReceipt{ID: "receipt", Status: "queued"}, nil
}
func (h *conversationTestHost) Get(_ context.Context, actor, id string) (*pluginsdk.ExternalMessageReceipt, error) {
	r := &pluginsdk.ExternalMessageReceipt{ID: id, Status: "running"}
	if h.done {
		r.Status = "completed"
		r.TurnID = "turn-1"
		r.Output = "Finished safely."
	}
	return r, nil
}
func TestConversationBoundReplyAndRestart(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	var posts []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		posts = append(posts, map[string]string{"channel": r.Form.Get("channel"), "thread": r.Form.Get("thread_ts"), "text": r.Form.Get("text")})
		fmt.Fprint(w, `{"ok":true,"channel":"D12345678","ts":"1.000001"}`)
	}))
	defer srv.Close()
	old := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = old })
	h := &conversationTestHost{fakeHost: newFakeHost(conversationConfig())}
	p := newSlackPlugin(context.Background())
	p.UnimplementedPlugin.SetHost(h)
	req := &pluginsdk.AgentToolRequest{Name: "notify_task_user", Context: pluginsdk.AgentToolContext{WorkspaceID: "workspace", TaskID: "observer", SessionID: "observer-session", Surface: "automation"}, Arguments: map[string]any{"user_id": "U12345678", "task_id": "task", "session_id": "session", "text": "Please review", "idempotency_key": "card-episode-1"}}
	for range 2 {
		r, err := p.InvokeAgentTool(context.Background(), req)
		if err != nil || r.IsError {
			t.Fatalf("notify %+v %v", r, err)
		}
	}
	if len(posts) != 1 {
		t.Fatal("notification duplicated")
	}
	if err := p.bridge.persist(context.Background(), dmEvent("Ev1", "1.000001")); err != nil {
		t.Fatal(err)
	}
	p.bridge.reconcile(context.Background())
	if len(h.submitted) != 1 || h.submitted[0].Target.TaskID != "task" {
		t.Fatalf("wrong target: %+v", h.submitted)
	}
	restarted := newSlackPlugin(context.Background())
	restarted.UnimplementedPlugin.SetHost(h)
	h.done = true
	restarted.bridge.reconcile(context.Background())
	if len(posts) != 2 || posts[1]["channel"] != "D12345678" || posts[1]["thread"] != "1.000001" {
		t.Fatalf("reply escaped thread: %+v", posts)
	}
	restarted.bridge.persist(context.Background(), dmEvent("Ev1", "1.000001"))
	restarted.bridge.reconcile(context.Background())
	if len(posts) != 2 || len(h.submitted) != 1 {
		t.Fatal("replay duplicated work or output")
	}
}
func TestConversationSerializesBoundThread(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	h := &conversationTestHost{fakeHost: newFakeHost(conversationConfig())}
	b := &conversationBridge{host: func() pluginsdk.Host { return h }}
	binding := conversationBinding{Team: "T12345678", User: "U12345678", Channel: "D12345678", Thread: "1.000001", Target: pluginsdk.ConversationTarget{ActorID: "human", WorkspaceID: "workspace", TaskID: "task", SessionID: "session"}}
	if err := writeConversation("bindings", notificationDigest(binding.Team, binding.Channel, binding.Thread), binding, true); err != nil {
		t.Fatal(err)
	}
	b.persist(context.Background(), dmEvent("Ev1", binding.Thread))
	b.persist(context.Background(), dmEvent("Ev2", binding.Thread))
	b.reconcile(context.Background())
	if len(h.submitted) != 1 {
		t.Fatalf("concurrent replies submitted: %d", len(h.submitted))
	}
}

func TestConversationRejectsForeignAndBotEvents(t *testing.T) {
	for _, kind := range []string{"foreign team", "foreign app", "bot", "edited", "channel", "shared", "missing ts"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("KANDEV_PLUGIN_DATA_DIR", root)
			h := newFakeHost(conversationConfig())
			b := &conversationBridge{host: func() pluginsdk.Host { return h }}
			var data map[string]any
			json.Unmarshal(dmEvent("Ev1", "1.0"), &data)
			e := data["event"].(map[string]any)
			switch kind {
			case "foreign team":
				data["team_id"] = "TOTHER"
			case "foreign app":
				data["api_app_id"] = "AOTHER"
			case "bot":
				e["bot_id"] = "bot"
			case "edited":
				e["subtype"] = "message_changed"
			case "channel":
				e["channel_type"] = "channel"
			case "shared":
				data["is_ext_shared_channel"] = true
			case "missing ts":
				delete(e, "ts")
			}
			raw, _ := json.Marshal(data)
			if err := b.persist(context.Background(), raw); err != nil {
				t.Fatal(err)
			}
			files, _ := os.ReadDir(filepath.Join(root, "conversations-v1", "inbox"))
			if len(files) != 0 {
				t.Fatal("unsupported event persisted")
			}
		})
	}
}
func TestConversationSocketDoesNotAckFailedPersistence(t *testing.T) {
	srv := newSocketServer(t, func(conn *websocket.Conn) {
		conn.WriteJSON(socketEnvelope{Type: "events_api", EnvelopeID: "envelope", Payload: dmEvent("Ev1", "1.0")})
	})
	l := listenerFor(t, srv, func(context.Context, inboundRequest) { t.Error("DM entered triage") })
	l.persistDM = func(context.Context, json.RawMessage) error { return errors.New("disk full") }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, srv.wsURL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = l.readLoop(ctx, conn); err == nil {
		t.Fatal("persistence failure ignored")
	}
	if srv.ackedCount() != 0 {
		t.Fatal("unpersisted event acknowledged")
	}
}
func TestConversationAmbiguousOutboundIsNeverReposted(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer srv.Close()
	old := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = old })
	c, _ := loadConversations(conversationConfig())
	for range 2 {
		b := &conversationBridge{}
		if err := b.postOnce(context.Background(), c, "event-answer", "D12345678", "1.0", "answer"); !errors.Is(err, errConversationDeliveryUncertain) {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("ambiguous message posted %d times", calls)
	}
}

func TestConversationRateLimitHonorsDelayThenRetriesOnce(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"ok":true,"channel":"D12345678","ts":"3.0"}`)
	}))
	defer srv.Close()
	old := slackAPIBase
	slackAPIBase = srv.URL
	t.Cleanup(func() { slackAPIBase = old })
	c, _ := loadConversations(conversationConfig())
	b := &conversationBridge{}
	for range 2 {
		if err := b.postOnce(context.Background(), c, "rate-event", "D12345678", "1.0", "answer"); err == nil {
			t.Fatal("rate limit ignored")
		}
	}
	if calls != 1 {
		t.Fatal("Retry-After ignored")
	}
	key := notificationDigest("rate-event")
	var record conversationDelivery
	if err := readConversation("outbox", key, &record); err != nil {
		t.Fatal(err)
	}
	record.RetryAt = time.Now().Add(-time.Second)
	if err := writeConversation("outbox", key, record, false); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := b.postOnce(context.Background(), c, "rate-event", "D12345678", "1.0", "answer"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("posts=%d", calls)
	}
}
