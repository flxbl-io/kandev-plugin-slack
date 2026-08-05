package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDetectAuthMode(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		want    authMode
		wantErr string
	}{
		{name: "bot", token: "xoxb-123", want: authModeBot},
		{name: "user", token: "xoxp-123", want: authModeUser},
		{name: "legacy user", token: "xoxs-123", want: authModeUser},
		{name: "browser session", token: "xoxc-123", want: authModeCookie},
		{name: "refresh token rejected", token: "xoxe-123", wantErr: "refresh token"},
		{name: "app token rejected", token: "xapp-123", wantErr: "Socket Mode"},
		{name: "unknown rejected", token: "ghp_123", wantErr: "unrecognized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectAuthMode(tc.token)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("detectAuthMode(%q) error = %v, want containing %q", tc.token, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectAuthMode(%q) unexpected error: %v", tc.token, err)
			}
			if got != tc.want {
				t.Fatalf("detectAuthMode(%q) = %q, want %q", tc.token, got, tc.want)
			}
		})
	}
}

func TestSearchCapable(t *testing.T) {
	if !authModeUser.searchCapable() || !authModeCookie.searchCapable() {
		t.Fatal("user and cookie tokens must be able to call search.messages")
	}
	if authModeBot.searchCapable() {
		t.Fatal("bot tokens cannot call search.messages")
	}
}

// baseConfig is a minimal valid cookie-mode config the validation tests mutate.
func baseConfig() map[string]any {
	return map[string]any{
		"slack_token":   "xoxc-abc",
		"slack_cookie":  "xoxd-cookie",
		"utility_agent": "agent-1",
	}
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	cfg, err := loadConfig(baseConfig())
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.CommandPrefix != defaultCommandPrefix {
		t.Fatalf("CommandPrefix = %q, want %q", cfg.CommandPrefix, defaultCommandPrefix)
	}
	if cfg.PollInterval != defaultPollIntervalSeconds*time.Second {
		t.Fatalf("PollInterval = %v, want %ds", cfg.PollInterval, defaultPollIntervalSeconds)
	}
	if cfg.Mode != authModeCookie {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, authModeCookie)
	}
}

func TestLoadConfigUnconfiguredIsSentinel(t *testing.T) {
	_, err := loadConfig(map[string]any{})
	if !errors.Is(err, errNotConfigured) {
		t.Fatalf("loadConfig(empty) = %v, want errNotConfigured", err)
	}
}

func TestLoadConfigRejectsIncoherentCredentials(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]any)
		wantErr string
	}{
		{
			name:    "cookie token without cookie",
			mutate:  func(m map[string]any) { delete(m, "slack_cookie") },
			wantErr: "needs the matching `d` cookie",
		},
		{
			name: "bot token with a cookie",
			mutate: func(m map[string]any) {
				m["slack_token"] = "xoxb-abc"
				m["channels"] = "C1"
			},
			wantErr: "cookie is only used with an xoxc- token",
		},
		{
			name: "bot token without channels",
			mutate: func(m map[string]any) {
				m["slack_token"] = "xoxb-abc"
				delete(m, "slack_cookie")
			},
			wantErr: "cannot call search.messages",
		},
		{
			name:    "slash command prefix",
			mutate:  func(m map[string]any) { m["command_prefix"] = "/kandev" },
			wantErr: "Slack intercepts slash commands",
		},
		{
			name:    "non-bot reply token",
			mutate:  func(m map[string]any) { m["reply_token"] = "xoxp-nope" },
			wantErr: "must be an xoxb- bot token",
		},
		{
			name:    "no triage agent",
			mutate:  func(m map[string]any) { delete(m, "utility_agent") },
			wantErr: "pick a triage agent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := baseConfig()
			tc.mutate(raw)
			_, err := loadConfig(raw)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("loadConfig error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadConfigAcceptsBotTokenWithChannels(t *testing.T) {
	raw := baseConfig()
	raw["slack_token"] = "xoxb-abc"
	delete(raw, "slack_cookie")
	raw["channels"] = " #C0123ABCD , C0456EFGH ,, C0123ABCD "
	cfg, err := loadConfig(raw)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Mode != authModeBot {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, authModeBot)
	}
	want := []string{"C0123ABCD", "C0456EFGH"}
	if len(cfg.Channels) != len(want) {
		t.Fatalf("Channels = %v, want %v", cfg.Channels, want)
	}
	for i := range want {
		if cfg.Channels[i] != want[i] {
			t.Fatalf("Channels = %v, want %v", cfg.Channels, want)
		}
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

// A bot reply token authenticates on its own; sending the browser cookie
// alongside it makes Slack reject the write.
func TestReplyCredentialsDropCookieForBotToken(t *testing.T) {
	cfg := &config{Token: "xoxc-a", Cookie: "d-value", ReplyToken: "xoxb-b"}
	token, cookie := cfg.ReplyCredentials()
	if token != "xoxb-b" || cookie != "" {
		t.Fatalf("ReplyCredentials() = (%q, %q), want (\"xoxb-b\", \"\")", token, cookie)
	}
}

func TestReplyCredentialsFallBackToReadCredentials(t *testing.T) {
	cfg := &config{Token: "xoxc-a", Cookie: "d-value"}
	token, cookie := cfg.ReplyCredentials()
	if token != "xoxc-a" || cookie != "d-value" {
		t.Fatalf("ReplyCredentials() = (%q, %q), want the read pair", token, cookie)
	}
}
