package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// recorder captures what the stub Slack server received, so tests can assert
// on the request the client built rather than only on what it returned.
type recorder struct {
	Method        string
	Form          url.Values
	Authorization string
	Cookie        string
}

// newTestClient points a client at a stub Slack that returns the supplied
// body for every call and records the last request it received.
func newTestClient(t *testing.T, body string) (*client, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		rec.Method = strings.TrimPrefix(r.URL.Path, "/")
		rec.Form = r.PostForm
		rec.Authorization = r.Header.Get("Authorization")
		rec.Cookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := newClient("xoxc-token", "cookie-value")
	c.endpoint = srv.URL
	return c, rec
}

func TestAuthTestSuccess(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true,"url":"https://acme.slack.com/","team":"Acme","user":"alice","team_id":"T1","user_id":"U1"}`)
	res, err := c.AuthTest(context.Background())
	if err != nil {
		t.Fatalf("AuthTest: %v", err)
	}
	if !res.OK || res.UserID != "U1" || res.TeamName != "Acme" {
		t.Fatalf("AuthTest = %+v, want ok with U1/Acme", res)
	}
	if rec.Authorization != "Bearer xoxc-token" {
		t.Fatalf("Authorization = %q", rec.Authorization)
	}
	if rec.Cookie != "d=cookie-value" {
		t.Fatalf("Cookie = %q, want d=cookie-value", rec.Cookie)
	}
}

// Slack reports auth failures inside a 200 envelope; the probe must surface
// them as a result the caller can persist, not as a transport error.
func TestAuthTestSurfacesSlackErrorAsResult(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":false,"error":"invalid_auth"}`)
	res, err := c.AuthTest(context.Background())
	if err != nil {
		t.Fatalf("AuthTest returned a transport error for an envelope failure: %v", err)
	}
	if res.OK {
		t.Fatal("AuthTest reported ok for invalid_auth")
	}
	if !strings.Contains(res.Error, "invalid_auth") || !strings.Contains(res.Error, "cookie") {
		t.Fatalf("Error = %q, want the expanded invalid_auth explanation", res.Error)
	}
}

func TestOmitsCookieHeaderWhenUnset(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true}`)
	c.cookie = ""
	if _, err := c.AuthTest(context.Background()); err != nil {
		t.Fatalf("AuthTest: %v", err)
	}
	if rec.Cookie != "" {
		t.Fatalf("Cookie = %q, want no cookie header for a bot/user token", rec.Cookie)
	}
}

func TestSearchMessagesMapsMatches(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true,"messages":{"matches":[
		{"ts":"2.0","text":"!kandev fix login","user":"U1","username":"alice",
		 "permalink":"https://acme.slack.com/p2","channel":{"id":"C1"},"thread_ts":"1.0"}]}}`)
	msgs, err := c.SearchMessages(context.Background(), `from:<@U1> "!kandev"`)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.TS != "2.0" || got.ChannelID != "C1" || got.ThreadTS != "1.0" || got.Permalink == "" {
		t.Fatalf("message = %+v", got)
	}
	if rec.Form.Get("sort_dir") != "desc" {
		t.Fatalf("sort_dir = %q, want desc", rec.Form.Get("sort_dir"))
	}
}

// A mention that is not in a thread used to fetch only its own message, so
// "@Kandev file what Bob just said" reached the agent with no idea what Bob
// said. It now gathers the conversation leading up to the mention.
func TestConversationContextGathersRecentHistoryForANonThreadedMention(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true,"messages":[
		{"ts":"5.0","text":"@Kandev file that","user":"U2"},
		{"ts":"4.0","text":"only on safari","user":"U1"},
		{"ts":"3.0","text":"login redirect loops","user":"U1"}]}`)
	msgs, err := c.ConversationContext(context.Background(), "C1", "", "5.0")
	if err != nil {
		t.Fatalf("ConversationContext: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want the mention plus its context", len(msgs))
	}
	// Slack returns history newest-first; the prompt needs reading order.
	if msgs[0].TS != "3.0" || msgs[2].TS != "5.0" {
		t.Fatalf("order = %s..%s, want oldest first", msgs[0].TS, msgs[2].TS)
	}
	if rec.Form.Get("latest") != "5.0" || rec.Form.Get("inclusive") != "true" {
		t.Fatalf("window = %v, want it anchored at the trigger", rec.Form)
	}
	// Anchoring at the trigger is what keeps messages that landed between
	// detection and processing out of the agent's context.
	if rec.Form.Get("oldest") != "" {
		t.Fatalf("oldest = %q, want an open lower bound", rec.Form.Get("oldest"))
	}
	if rec.Form.Get("limit") != "20" {
		t.Fatalf("limit = %q, want the context window", rec.Form.Get("limit"))
	}
}

// A slash command has no message of its own, so "now" is the right anchor:
// the conversation the user was looking at when they ran it.
func TestConversationContextForASlashCommandReadsTheLatestMessages(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true,"messages":[{"ts":"9.0","text":"latest","user":"U1"}]}`)
	msgs, err := c.ConversationContext(context.Background(), "C1", "", "")
	if err != nil {
		t.Fatalf("ConversationContext: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want the recent channel history", len(msgs))
	}
	if rec.Form.Get("latest") != "" {
		t.Fatalf("latest = %q, want an open upper bound for a command", rec.Form.Get("latest"))
	}
}

// A threaded mention is answered from the thread, not the channel.
func TestConversationContextUsesTheThreadWhenThereIsOne(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true,"messages":[{"ts":"2.0","text":"in thread","user":"U1"}]}`)
	if _, err := c.ConversationContext(context.Background(), "C1", "1.0", "2.0"); err != nil {
		t.Fatalf("ConversationContext: %v", err)
	}
	if rec.Method != "conversations.replies" || rec.Form.Get("ts") != "1.0" {
		t.Fatalf("called %q with %v, want conversations.replies on the parent", rec.Method, rec.Form)
	}
}

// Joins and topic changes are noise; a monitoring bot's alert usually *is* the
// thing being triaged, so app messages are kept.
func TestConversationContextSkipsSubtypesButKeepsAppMessages(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":true,"messages":[
		{"ts":"7.0","text":"@Kandev file this","user":"U1"},
		{"ts":"6.0","text":"CRITICAL: checkout 500s","bot_id":"B_SENTRY"},
		{"ts":"5.0","text":"joined the channel","subtype":"channel_join","user":"U2"}]}`)
	msgs, err := c.ConversationContext(context.Background(), "C1", "", "7.0")
	if err != nil {
		t.Fatalf("ConversationContext: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want the alert and the mention", len(msgs))
	}
	if msgs[0].Text != "CRITICAL: checkout 500s" {
		t.Fatalf("first context message = %q, want the alert retained", msgs[0].Text)
	}
}

// The trigger re-acknowledges after a restart or a stalled watermark, so a
// repeat reaction must not read as a failure.
func TestAddReactionSwallowsAlreadyReacted(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":false,"error":"already_reacted"}`)
	if err := c.AddReaction(context.Background(), "C1", "1.0", "eyes"); err != nil {
		t.Fatalf("AddReaction = %v, want nil for already_reacted", err)
	}
}

func TestAddReactionPropagatesOtherErrors(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":false,"error":"missing_scope"}`)
	err := c.AddReaction(context.Background(), "C1", "1.0", "eyes")
	if err == nil || !strings.Contains(err.Error(), "missing_scope") {
		t.Fatalf("AddReaction = %v, want the missing_scope error", err)
	}
}

func TestPostMessageThreadsTheReply(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true}`)
	if err := c.PostMessage(context.Background(), "C1", "1.0", "done"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if rec.Form.Get("thread_ts") != "1.0" || rec.Form.Get("text") != "done" {
		t.Fatalf("form = %v", rec.Form)
	}
}

func TestPostFailsFastWithoutAToken(t *testing.T) {
	c := newClient("", "")
	if err := c.PostMessage(context.Background(), "C1", "", "x"); err == nil {
		t.Fatal("PostMessage without a token should fail")
	}
}

func TestCompareTSOrdersSlackTimestamps(t *testing.T) {
	if compareTS("1714659000.000200", "1714659000.000100") <= 0 {
		t.Fatal("later microsecond suffix must sort higher")
	}
	if compareTS("", "1.0") != -1 || compareTS("1.0", "") != 1 || compareTS("1.0", "1.0") != 0 {
		t.Fatal("empty watermark must sort below any timestamp")
	}
}

// The remedy for a rejected credential differs per mode: a stale `d` cookie is
// the usual cause for the browser fallback and impossible for a Slack app, so
// mode-blind advice sends one of the two paths chasing a field it must leave
// empty.
func TestExplainInvalidAuthIsModeSpecific(t *testing.T) {
	session := explainSlackError("invalid_auth", authModeSession)
	if !strings.Contains(session, "`d` cookie is stale") {
		t.Fatalf("session remedy = %q, want the cookie advice", session)
	}
	app := explainSlackError("invalid_auth", authModeApp)
	if !strings.Contains(app, "Bot User OAuth Token") {
		t.Fatalf("app remedy = %q, want the reinstall advice", app)
	}
	if strings.Contains(app, "cookie") {
		t.Fatalf("app remedy = %q, must not mention a cookie", app)
	}
}

// Slack only applies scope changes on reinstall, which is the single most
// common reason a freshly-edited manifest still fails.
func TestExplainScopeErrorTellsAppsToReinstall(t *testing.T) {
	got := explainSlackError("missing_scope", authModeApp)
	if !strings.Contains(got, "reinstall") {
		t.Fatalf("explainSlackError = %q, want the reinstall requirement", got)
	}
}

// The client derives its mode from the token so callers never pass it.
func TestClientCarriesTheDetectedMode(t *testing.T) {
	if newClient("xoxc-abc", "d").mode != authModeSession {
		t.Fatal("a session token must select the fallback remedies")
	}
	if newClient("xoxb-abc", "").mode != authModeApp {
		t.Fatal("a bot token must select the app remedies")
	}
}

// An unmapped Slack code must reach the operator verbatim rather than being
// swallowed into a generic message.
func TestExplainUnknownCodePassesThrough(t *testing.T) {
	if got := explainSlackError("channel_not_found", authModeApp); got != "channel_not_found" {
		t.Fatalf("explainSlackError = %q, want the raw code", got)
	}
}
