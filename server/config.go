package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// authMode is how the plugin authenticates against Slack's Web API. It is
// derived from the token's prefix rather than configured separately: Slack
// prefixes are unambiguous, and an explicit picker only creates a second
// source of truth that can disagree with the pasted credential.
type authMode string

const (
	// authModeUser is an xoxp- OAuth user token. The officially supported way
	// to run the search-based trigger: search.messages requires a user token
	// carrying the search:read scope, and is not available to bot tokens.
	authModeUser authMode = "user_token"

	// authModeBot is an xoxb- bot token from an installed Slack app. Bot
	// tokens cannot search, so the trigger falls back to polling
	// conversations.history for each configured channel.
	authModeBot authMode = "bot_token"

	// authModeCookie is an xoxc- browser session token paired with the `d`
	// cookie. Unofficial and unsupported by Slack, but it needs no app
	// install and has the full privileges of the signed-in user, including
	// search.
	authModeCookie authMode = "cookie"
)

// searchCapable reports whether the mode can drive the search.messages
// trigger. Bot tokens cannot, which is why they need an explicit channel list.
func (m authMode) searchCapable() bool {
	return m == authModeUser || m == authModeCookie
}

// String renders the mode for status output and error messages.
func (m authMode) String() string { return string(m) }

// Label is the human-facing name shown on the plugin page.
func (m authMode) Label() string {
	switch m {
	case authModeUser:
		return "User token (xoxp)"
	case authModeBot:
		return "Bot token (xoxb)"
	case authModeCookie:
		return "Browser session (xoxc + d cookie)"
	default:
		return string(m)
	}
}

// Defaults mirror the values declared in manifest.yaml's config_schema. They
// are repeated here because GetConfig returns the stored config, and a field
// the operator never touched is simply absent rather than defaulted.
const (
	defaultCommandPrefix       = "!kandev"
	defaultPollIntervalSeconds = 30
	minPollIntervalSeconds     = 5
	maxPollIntervalSeconds     = 600
)

// config is the validated view of the operator's plugin configuration.
type config struct {
	Mode authMode

	// Token is the credential used for reads (search / history) and, unless
	// ReplyToken is set, for writes as well.
	Token string
	// Cookie is the `d` cookie value; only populated in cookie mode.
	Cookie string
	// ReplyToken optionally overrides Token for chat.postMessage and
	// reactions.add so replies carry an app identity instead of the
	// operator's own Slack account.
	ReplyToken string

	CommandPrefix string
	Channels      []string
	PollInterval  time.Duration
	StartAgent    bool
	UtilityAgent  string
}

// ReplyCredentials returns the (token, cookie) pair to use for writes. A bot
// reply token authenticates on its own, so the browser cookie is dropped with
// it — sending a `d` cookie alongside an xoxb- token makes Slack reject the
// call rather than ignore the extra header.
func (c *config) ReplyCredentials() (string, string) {
	if c.ReplyToken != "" {
		return c.ReplyToken, ""
	}
	return c.Token, c.Cookie
}

// errNotConfigured is the sentinel for "the operator has not finished
// filling in Settings > Plugins yet". Callers treat it as an idle state, not
// a failure worth logging on every poll.
var errNotConfigured = errors.New("slack plugin is not configured")

// loadConfig validates the raw config_schema values Host.GetConfig returns.
// kandev already enforces required-ness and types; what is left here is the
// cross-field logic a JSON-Schema subset cannot express — which credential
// pairs are coherent, and which modes need a channel list.
func loadConfig(raw map[string]any) (*config, error) {
	token := strings.TrimSpace(configString(raw, "slack_token"))
	if token == "" {
		return nil, errNotConfigured
	}
	mode, err := detectAuthMode(token)
	if err != nil {
		return nil, err
	}
	cfg := &config{
		Mode:          mode,
		Token:         token,
		Cookie:        strings.TrimSpace(configString(raw, "slack_cookie")),
		ReplyToken:    strings.TrimSpace(configString(raw, "reply_token")),
		CommandPrefix: strings.TrimSpace(configString(raw, "command_prefix")),
		Channels:      parseChannels(configString(raw, "channels")),
		PollInterval:  pollInterval(raw),
		StartAgent:    configBool(raw, "start_agent"),
		UtilityAgent:  strings.TrimSpace(configString(raw, "utility_agent")),
	}
	if cfg.CommandPrefix == "" {
		cfg.CommandPrefix = defaultCommandPrefix
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *config) validate() error {
	if strings.HasPrefix(c.CommandPrefix, "/") {
		return errors.New("command prefix cannot start with \"/\" — Slack intercepts slash commands before they become messages")
	}
	if c.Mode == authModeCookie && c.Cookie == "" {
		return errors.New("an xoxc- token needs the matching `d` cookie — paste it into the \"Browser d cookie\" field")
	}
	if c.Mode != authModeCookie && c.Cookie != "" {
		// Not fatal on its own, but silently ignoring a pasted credential
		// hides a real misunderstanding about which mode is active.
		return fmt.Errorf("a `d` cookie is only used with an xoxc- token; this token is a %s — clear the cookie field", c.Mode.Label())
	}
	if c.ReplyToken != "" && !strings.HasPrefix(c.ReplyToken, botTokenPrefix) {
		return errors.New("the reply token must be an xoxb- bot token")
	}
	if !c.Mode.searchCapable() && len(c.Channels) == 0 {
		return errors.New("a bot token cannot call search.messages — list the channel IDs to watch in \"Channels\" and invite the bot to each one")
	}
	if c.UtilityAgent == "" {
		return errors.New("pick a triage agent")
	}
	return nil
}

// Slack's documented token prefixes. Kept as constants because the same
// strings appear in validation, detection, and operator-facing errors.
const (
	botTokenPrefix     = "xoxb-"
	userTokenPrefix    = "xoxp-"
	legacyTokenPrefix  = "xoxs-"
	cookieTokenPrefix  = "xoxc-"
	refreshTokenPrefix = "xoxe-"
	appTokenPrefix     = "xapp-"
)

// detectAuthMode maps a Slack token onto the trigger strategy it can support.
// The two rejected prefixes are called out by name because both are easy to
// copy by mistake from the same Slack app page as the token you wanted.
func detectAuthMode(token string) (authMode, error) {
	switch {
	case strings.HasPrefix(token, botTokenPrefix):
		return authModeBot, nil
	case strings.HasPrefix(token, userTokenPrefix), strings.HasPrefix(token, legacyTokenPrefix):
		return authModeUser, nil
	case strings.HasPrefix(token, cookieTokenPrefix):
		return authModeCookie, nil
	case strings.HasPrefix(token, refreshTokenPrefix):
		return "", errors.New("xoxe- is a refresh token, not an access token — use the xoxb-/xoxp- token it refreshes")
	case strings.HasPrefix(token, appTokenPrefix):
		return "", errors.New("xapp- is an app-level token for Socket Mode — use the xoxb- bot token from the same app instead")
	default:
		return "", errors.New("unrecognized Slack token: expected it to start with xoxp- (user), xoxb- (bot), or xoxc- (browser session)")
	}
}

// parseChannels splits the comma-separated channel list, tolerating the
// spaces and #-prefixes operators paste in. Channel *names* cannot be
// resolved without an extra API round-trip per poll, so only IDs are
// accepted; a stray "#" is stripped rather than silently producing a channel
// that never matches.
func parseChannels(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		id := strings.TrimPrefix(strings.TrimSpace(f), "#")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// pollInterval clamps the configured cadence into the supported band. Out-of
// -range values fall back to the default rather than erroring: a too-eager
// interval is a tuning mistake, not a reason to stop triaging.
func pollInterval(raw map[string]any) time.Duration {
	seconds := configInt(raw, "poll_interval_seconds")
	if seconds < minPollIntervalSeconds || seconds > maxPollIntervalSeconds {
		seconds = defaultPollIntervalSeconds
	}
	return time.Duration(seconds) * time.Second
}

func configString(raw map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	s, _ := raw[key].(string)
	return s
}

func configBool(raw map[string]any, key string) bool {
	if raw == nil {
		return false
	}
	b, _ := raw[key].(bool)
	return b
}

// configInt reads a numeric config value. Numbers cross the Host gRPC
// boundary as protobuf Struct values, which are always float64 on arrival —
// the int cases are for direct construction in tests.
func configInt(raw map[string]any, key string) int {
	if raw == nil {
		return 0
	}
	switch v := raw[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}
