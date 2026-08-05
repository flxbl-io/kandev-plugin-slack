package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestStripMention(t *testing.T) {
	cases := []struct {
		name string
		text string
		bot  string
		want string
	}{
		{name: "plain mention", text: "<@U0BOT> fix the login bug", bot: "U0BOT", want: "fix the login bug"},
		{name: "colon separator", text: "<@U0BOT>: fix it", bot: "U0BOT", want: "fix it"},
		{name: "labelled mention", text: "<@U0BOT|kandev> fix it", bot: "U0BOT", want: "fix it"},
		{name: "extra whitespace", text: "  <@U0BOT>   fix it  ", bot: "U0BOT", want: "fix it"},
		{name: "unknown bot id leaves text intact", text: "<@U0BOT> fix it", bot: "", want: "<@U0BOT> fix it"},
		// A mention partway through is part of what the user wrote — stripping
		// it would silently edit their instruction.
		{name: "mid-sentence mention is preserved", text: "ask <@U0BOT> about it", bot: "U0BOT", want: "ask <@U0BOT> about it"},
		{name: "mention only", text: "<@U0BOT>", bot: "U0BOT", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripMention(tc.text, tc.bot); got != tc.want {
				t.Fatalf("stripMention(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestDecodeAppMention(t *testing.T) {
	l := &socketListener{botUserID: "U0BOT"}
	payload := json.RawMessage(`{"event":{"type":"app_mention","user":"U1","text":"<@U0BOT> fix login",
		"ts":"2.0","thread_ts":"1.0","channel":"C1"}}`)
	req, ok := l.decodeEvent(payload)
	if !ok {
		t.Fatal("app_mention was not decoded")
	}
	if req.Instruction != "fix login" || req.ChannelID != "C1" || req.TS != "2.0" || req.ThreadTS != "1.0" {
		t.Fatalf("request = %+v", req)
	}
	if !req.Acknowledge {
		t.Fatal("a mention has a real message to react to")
	}
}

// The app posts its own replies into the same channels it listens to; acting
// on them would loop.
func TestDecodeIgnoresTheAppsOwnPosts(t *testing.T) {
	l := &socketListener{botUserID: "U0BOT"}
	byBotID := json.RawMessage(`{"event":{"type":"app_mention","bot_id":"B1","text":"<@U0BOT> x","ts":"2.0","channel":"C1"}}`)
	if _, ok := l.decodeEvent(byBotID); ok {
		t.Fatal("a bot_id post must be ignored")
	}
	bySelf := json.RawMessage(`{"event":{"type":"app_mention","user":"U0BOT","text":"<@U0BOT> x","ts":"2.0","channel":"C1"}}`)
	if _, ok := l.decodeEvent(bySelf); ok {
		t.Fatal("the app's own user id must be ignored")
	}
}

func TestDecodeIgnoresOtherEventTypesAndEmptyInstructions(t *testing.T) {
	l := &socketListener{botUserID: "U0BOT"}
	other := json.RawMessage(`{"event":{"type":"message","user":"U1","text":"hello","ts":"2.0","channel":"C1"}}`)
	if _, ok := l.decodeEvent(other); ok {
		t.Fatal("only app_mention should be triaged")
	}
	bare := json.RawMessage(`{"event":{"type":"app_mention","user":"U1","text":"<@U0BOT>","ts":"2.0","channel":"C1"}}`)
	if _, ok := l.decodeEvent(bare); ok {
		t.Fatal("a mention with no instruction has nothing to triage")
	}
}

// A slash command is only visible to the person who ran it, so there is no
// message to react to and nothing to thread under.
func TestDecodeSlashCommand(t *testing.T) {
	payload := json.RawMessage(`{"channel_id":"C1","user_id":"U1","user_name":"alice",
		"text":"fix the login bug","command":"/kandev"}`)
	req, ok := decodeSlashCommand(payload)
	if !ok {
		t.Fatal("slash command was not decoded")
	}
	if req.Instruction != "fix the login bug" || req.UserName != "alice" || req.ChannelID != "C1" {
		t.Fatalf("request = %+v", req)
	}
	if req.Acknowledge {
		t.Fatal("a slash command has no message to react to")
	}
	if req.replyThread() != "" {
		t.Fatalf("replyThread() = %q, want empty so the reply starts a thread", req.replyThread())
	}
}

func TestDecodeSlashCommandRejectsEmptyText(t *testing.T) {
	if _, ok := decodeSlashCommand(json.RawMessage(`{"channel_id":"C1","text":"   "}`)); ok {
		t.Fatal("a bare /kandev has nothing to triage")
	}
}

func TestDecodeUndecodablePayloads(t *testing.T) {
	l := &socketListener{}
	if _, ok := l.decodeEvent(json.RawMessage(`not json`)); ok {
		t.Fatal("undecodable event payload must be dropped, not acted on")
	}
	if _, ok := decodeSlashCommand(json.RawMessage(`not json`)); ok {
		t.Fatal("undecodable slash payload must be dropped, not acted on")
	}
}

func TestReplyThreadPrefersTheExistingThread(t *testing.T) {
	threaded := inboundRequest{TS: "2.0", ThreadTS: "1.0"}
	if got := threaded.replyThread(); got != "1.0" {
		t.Fatalf("replyThread() = %q, want the parent thread", got)
	}
	standalone := inboundRequest{TS: "2.0"}
	if got := standalone.replyThread(); got != "2.0" {
		t.Fatalf("replyThread() = %q, want a thread under the message itself", got)
	}
}

// --- live socket behaviour ---

// socketServer stubs both halves Slack provides: apps.connections.open over
// HTTP, and the WebSocket it points at.
type socketServer struct {
	http   *httptest.Server
	frames chan []byte

	mu   sync.Mutex
	acks []string
}

func newSocketServer(t *testing.T, send func(conn *websocket.Conn)) *socketServer {
	t.Helper()
	s := &socketServer{frames: make(chan []byte, 8)}
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/socket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		go send(conn)
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var ack struct {
				EnvelopeID string `json:"envelope_id"`
			}
			if json.Unmarshal(raw, &ack) == nil && ack.EnvelopeID != "" {
				s.mu.Lock()
				s.acks = append(s.acks, ack.EnvelopeID)
				s.mu.Unlock()
			}
		}
	})
	s.http = httptest.NewServer(mux)
	t.Cleanup(s.http.Close)
	return s
}

func (s *socketServer) wsURL() string {
	return "ws" + strings.TrimPrefix(s.http.URL, "http") + "/socket"
}

func (s *socketServer) ackedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.acks)
}

// listenerFor wires a listener straight at the stub's WebSocket, bypassing
// apps.connections.open (covered separately).
func listenerFor(t *testing.T, srv *socketServer, handle func(context.Context, inboundRequest)) *socketListener {
	t.Helper()
	return &socketListener{botUserID: "U0BOT", handle: handle}
}

func TestReadLoopAcksAndDispatches(t *testing.T) {
	srv := newSocketServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello"}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"events_api","envelope_id":"env-1",
			"payload":{"event":{"type":"app_mention","user":"U1","text":"<@U0BOT> do it","ts":"2.0","channel":"C1"}}}`))
	})

	got := make(chan inboundRequest, 1)
	l := listenerFor(t, srv, func(_ context.Context, req inboundRequest) { got <- req })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, srv.wsURL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	go func() { _ = l.readLoop(ctx, conn) }()

	select {
	case req := <-got:
		if req.Instruction != "do it" {
			t.Fatalf("instruction = %q", req.Instruction)
		}
	case <-ctx.Done():
		t.Fatal("no request dispatched")
	}

	// Slack redelivers anything unacknowledged within three seconds, so the
	// ack must go out regardless of how long triage takes.
	deadline := time.After(3 * time.Second)
	for srv.ackedCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("envelope was never acknowledged")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Slack sends `disconnect` before cycling a connection. That is routine, so
// the read loop must return cleanly rather than reporting an error that would
// trigger the error backoff.
func TestReadLoopTreatsDisconnectAsClean(t *testing.T) {
	srv := newSocketServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"disconnect","reason":"refresh_requested"}`))
	})
	l := listenerFor(t, srv, func(context.Context, inboundRequest) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, srv.wsURL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	done := make(chan error, 1)
	go func() { done <- l.readLoop(ctx, conn) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("readLoop = %v, want a clean return for a deliberate cycle", err)
		}
	case <-ctx.Done():
		t.Fatal("readLoop did not return on disconnect")
	}
}

// An unrecognized frame type still gets acknowledged, or Slack retries it
// forever; the frame set grows over time.
func TestReadLoopAcksUnknownFrames(t *testing.T) {
	srv := newSocketServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"some_future_type","envelope_id":"env-9"}`))
	})
	l := listenerFor(t, srv, func(context.Context, inboundRequest) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, srv.wsURL(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	go func() { _ = l.readLoop(ctx, conn) }()

	deadline := time.After(3 * time.Second)
	for srv.ackedCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("unknown frame was never acknowledged")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestOpenSocketConnectionReportsAMissingURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newClient("xapp-token", "")
	c.endpoint = srv.URL
	var resp openConnectionResponse
	if err := c.post(context.Background(), "apps.connections.open", nil, &resp); err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.URL != "" {
		t.Fatal("expected no URL in this response")
	}
}

// The app-level token and the bot token sit in adjacent fields and come from
// different pages of the same Slack app, so a rejection here has to name which
// one Slack refused rather than reusing the bot-token advice.
func TestExplainAppTokenErrorNamesTheAppLevelToken(t *testing.T) {
	got := explainAppTokenError("invalid_auth")
	if !strings.Contains(got, "App-Level Tokens") || !strings.Contains(got, "not OAuth & Permissions") {
		t.Fatalf("explainAppTokenError = %q, want it to point at the app-level token page", got)
	}
	if !strings.Contains(explainAppTokenError("missing_scope"), "connections:write") {
		t.Fatal("missing_scope must name the required scope")
	}
	if !strings.Contains(explainAppTokenError("socket_mode_not_enabled"), "slack-app-manifest.yaml") {
		t.Fatal("socket_mode_not_enabled should point back at the shipped manifest")
	}
	if got := explainAppTokenError("weird_code"); got != "weird_code" {
		t.Fatalf("explainAppTokenError = %q, want unknown codes verbatim", got)
	}
}
