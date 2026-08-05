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

// Slack's `oldest` is inclusive, so the watermark message comes back on every
// poll. Returning it would re-triage the same request forever.
func TestChannelHistoryExcludesTheWatermarkMessage(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":true,"messages":[
		{"ts":"3.0","text":"!kandev newer","user":"U1"},
		{"ts":"2.0","text":"!kandev watermark","user":"U1"}]}`)
	msgs, err := c.ChannelHistory(context.Background(), "C1", "2.0")
	if err != nil {
		t.Fatalf("ChannelHistory: %v", err)
	}
	if len(msgs) != 1 || msgs[0].TS != "3.0" {
		t.Fatalf("got %+v, want only ts 3.0", msgs)
	}
}

// Joins, topic changes and bot posts are not user requests; replying to our
// own reply would loop.
func TestChannelHistorySkipsSubtypesAndBotPosts(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":true,"messages":[
		{"ts":"5.0","text":"joined","subtype":"channel_join","user":"U1"},
		{"ts":"6.0","text":"from a bot","bot_id":"B1"},
		{"ts":"7.0","text":"!kandev real","user":"U1"}]}`)
	msgs, err := c.ChannelHistory(context.Background(), "C1", "")
	if err != nil {
		t.Fatalf("ChannelHistory: %v", err)
	}
	if len(msgs) != 1 || msgs[0].TS != "7.0" {
		t.Fatalf("got %+v, want only the real user message", msgs)
	}
}

// A non-threaded trigger must read exactly its own message, not whatever
// landed in the channel since it was detected.
func TestThreadContextPinsToTheTriggerWhenNotThreaded(t *testing.T) {
	c, rec := newTestClient(t, `{"ok":true,"messages":[{"ts":"4.0","text":"!kandev x","user":"U1"}]}`)
	msgs, err := c.ThreadContext(context.Background(), "C1", "", "4.0")
	if err != nil {
		t.Fatalf("ThreadContext: %v", err)
	}
	if len(msgs) != 1 || msgs[0].TS != "4.0" {
		t.Fatalf("got %+v, want the trigger message", msgs)
	}
	if rec.Form.Get("oldest") != "4.0" || rec.Form.Get("latest") != "4.0" || rec.Form.Get("inclusive") != "true" {
		t.Fatalf("history window = %v, want both ends pinned to the trigger", rec.Form)
	}
}

func TestThreadContextEmptyTriggerReadsNothing(t *testing.T) {
	c, _ := newTestClient(t, `{"ok":true,"messages":[{"ts":"9.0","text":"unrelated"}]}`)
	msgs, err := c.ThreadContext(context.Background(), "C1", "", "")
	if err != nil {
		t.Fatalf("ThreadContext: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("got %+v, want nothing rather than the channel's latest", msgs)
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
// the usual cause for xoxc- and impossible for xoxb-/xoxp-, so mode-blind
// advice sends two of the three modes chasing a field they must leave empty.
func TestExplainInvalidAuthIsModeSpecific(t *testing.T) {
	cases := []struct {
		mode     authMode
		want     string
		unwanted string
	}{
		{mode: authModeCookie, want: "`d` cookie is stale"},
		{mode: authModeBot, want: "Bot User OAuth token", unwanted: "cookie"},
		{mode: authModeUser, want: "User OAuth token", unwanted: "cookie"},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			got := explainSlackError("invalid_auth", tc.mode)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("explainSlackError(invalid_auth, %s) = %q, want it to mention %q", tc.mode, got, tc.want)
			}
			if tc.unwanted != "" && strings.Contains(got, tc.unwanted) {
				t.Fatalf("explainSlackError(invalid_auth, %s) = %q, must not mention %q", tc.mode, got, tc.unwanted)
			}
		})
	}
}

// Bot tokens are the one mode that can never search, so the scope advice has
// to say so rather than listing search:read as a fix.
func TestExplainScopeErrorTellsBotsSearchIsImpossible(t *testing.T) {
	got := explainSlackError("not_allowed_token_type", authModeBot)
	if !strings.Contains(got, "never call search.messages") {
		t.Fatalf("explainSlackError = %q, want the bot search limitation", got)
	}
	if strings.Contains(explainSlackError("missing_scope", authModeUser), "never call search.messages") {
		t.Fatal("user tokens can search; the bot limitation must not be shown for them")
	}
}

// The client derives its mode from the token so callers never have to pass it.
func TestClientCarriesTheDetectedMode(t *testing.T) {
	if newClient("xoxb-abc", "").mode != authModeBot {
		t.Fatal("client did not detect a bot token")
	}
	if newClient("garbage", "").mode != "" {
		t.Fatal("an unrecognized token must leave the mode empty rather than guessing")
	}
}

// An unmapped Slack code must reach the operator verbatim rather than being
// swallowed into a generic message.
func TestExplainUnknownCodePassesThrough(t *testing.T) {
	if got := explainSlackError("channel_not_found", authModeBot); got != "channel_not_found" {
		t.Fatalf("explainSlackError = %q, want the raw code", got)
	}
}
