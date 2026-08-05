package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// appConfig is a minimal valid Socket Mode config the validation tests mutate.
func appConfig() map[string]any {
	return map[string]any{
		"app_token":     "xapp-1-abc",
		"bot_token":     "xoxb-abc",
		"utility_agent": "agent-1",
	}
}

// sessionConfig is the fallback equivalent.
func sessionConfig() map[string]any {
	return map[string]any{
		"session_token":  "xoxc-abc",
		"session_cookie": "d-value",
		"utility_agent":  "agent-1",
	}
}

func TestLoadConfigDetectsSocketMode(t *testing.T) {
	cfg, err := loadConfig(appConfig())
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Mode != authModeApp {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, authModeApp)
	}
	if !cfg.Mode.realtime() {
		t.Fatal("a Slack app must be event-driven, not polled")
	}
	token, cookie := cfg.WebCredentials()
	if token != "xoxb-abc" || cookie != "" {
		t.Fatalf("WebCredentials() = (%q, %q), want the bot token and no cookie", token, cookie)
	}
}

func TestLoadConfigDetectsSessionFallback(t *testing.T) {
	cfg, err := loadConfig(sessionConfig())
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Mode != authModeSession {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, authModeSession)
	}
	if cfg.Mode.realtime() {
		t.Fatal("the browser-session fallback cannot receive events")
	}
	token, cookie := cfg.WebCredentials()
	if token != "xoxc-abc" || cookie != "d-value" {
		t.Fatalf("WebCredentials() = (%q, %q), want the session pair", token, cookie)
	}
}

// Preferring the fallback because a stale cookie is still saved would be a
// confusing way to silently lose real-time events.
func TestSocketModeWinsWhenBothPathsAreFilled(t *testing.T) {
	raw := appConfig()
	raw["session_token"] = "xoxc-old"
	raw["session_cookie"] = "d-old"
	cfg, err := loadConfig(raw)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Mode != authModeApp {
		t.Fatalf("Mode = %q, want the Slack app to win", cfg.Mode)
	}
}

func TestLoadConfigUnconfiguredIsSentinel(t *testing.T) {
	if _, err := loadConfig(map[string]any{"utility_agent": "agent-1"}); !errors.Is(err, errNotConfigured) {
		t.Fatalf("loadConfig(no credentials) = %v, want errNotConfigured", err)
	}
}

func TestLoadConfigRejectsIncoherentCredentials(t *testing.T) {
	cases := []struct {
		name    string
		base    func() map[string]any
		mutate  func(map[string]any)
		wantErr string
	}{
		{
			name:    "app token without bot token",
			base:    appConfig,
			mutate:  func(m map[string]any) { delete(m, "bot_token") },
			wantErr: "needs its bot token too",
		},
		{
			name:    "bot token without app token",
			base:    appConfig,
			mutate:  func(m map[string]any) { delete(m, "app_token") },
			wantErr: "needs its app-level token too",
		},
		{
			name:    "tokens swapped",
			base:    appConfig,
			mutate:  func(m map[string]any) { m["app_token"], m["bot_token"] = m["bot_token"], m["app_token"] },
			wantErr: "this is a bot token",
		},
		{
			name:    "user token pasted as bot token",
			base:    appConfig,
			mutate:  func(m map[string]any) { m["bot_token"] = "xoxp-user" },
			wantErr: "this is a user token",
		},
		{
			name:    "session token without cookie",
			base:    sessionConfig,
			mutate:  func(m map[string]any) { delete(m, "session_cookie") },
			wantErr: "needs the `d` cookie",
		},
		{
			name:    "cookie without session token",
			base:    sessionConfig,
			mutate:  func(m map[string]any) { delete(m, "session_token") },
			wantErr: "needs the xoxc- token",
		},
		{
			name:    "slash prefix in fallback",
			base:    sessionConfig,
			mutate:  func(m map[string]any) { m["command_prefix"] = "/kandev" },
			wantErr: "Slack intercepts slash commands",
		},
		{
			name:    "no triage agent",
			base:    appConfig,
			mutate:  func(m map[string]any) { delete(m, "utility_agent") },
			wantErr: "pick a triage agent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := tc.base()
			tc.mutate(raw)
			_, err := loadConfig(raw)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("loadConfig error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// Every one of these is reachable from the same Slack app page as the token
// that was wanted, so naming the wrong one saves a round of guessing.
func TestDescribeTokenNamesWhatWasPasted(t *testing.T) {
	cases := map[string]string{
		"xapp-1": "app-level token",
		"xoxb-1": "bot token",
		"xoxp-1": "user token",
		"xoxc-1": "browser-session token",
		"xoxe-1": "refresh token",
		"nope":   "does not look like a Slack token",
	}
	for token, want := range cases {
		if got := describeToken(token); !strings.Contains(got, want) {
			t.Fatalf("describeToken(%q) = %q, want it to mention %q", token, got, want)
		}
	}
}

func TestFallbackDefaultsAndChannelParsing(t *testing.T) {
	raw := sessionConfig()
	raw["channels"] = " #C0123ABCD , C0456EFGH ,, C0123ABCD "
	cfg, err := loadConfig(raw)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.CommandPrefix != defaultCommandPrefix {
		t.Fatalf("CommandPrefix = %q, want %q", cfg.CommandPrefix, defaultCommandPrefix)
	}
	if cfg.PollInterval != defaultPollIntervalSeconds*time.Second {
		t.Fatalf("PollInterval = %v, want %ds", cfg.PollInterval, defaultPollIntervalSeconds)
	}
	want := []string{"C0123ABCD", "C0456EFGH"}
	if len(cfg.Channels) != len(want) || cfg.Channels[0] != want[0] || cfg.Channels[1] != want[1] {
		t.Fatalf("Channels = %v, want %v", cfg.Channels, want)
	}
}

func TestPollIntervalClamping(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  time.Duration
	}{
		{name: "in range", value: float64(120), want: 120 * time.Second},
		{name: "below floor falls back", value: float64(1), want: defaultPollIntervalSeconds * time.Second},
		{name: "above ceiling falls back", value: float64(9999), want: defaultPollIntervalSeconds * time.Second},
		{name: "absent falls back", value: nil, want: defaultPollIntervalSeconds * time.Second},
		{name: "int survives direct construction", value: 45, want: 45 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{}
			if tc.value != nil {
				raw["poll_interval_seconds"] = tc.value
			}
			if got := pollInterval(raw); got != tc.want {
				t.Fatalf("pollInterval = %v, want %v", got, tc.want)
			}
		})
	}
}
