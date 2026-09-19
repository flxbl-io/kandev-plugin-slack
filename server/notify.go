package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

var slackUserID = regexp.MustCompile(`^[UW][A-Z0-9]{8,63}$`)

func notificationResult(status, user string, duplicate bool) *pluginsdk.AgentToolResult {
	return &pluginsdk.AgentToolResult{Text: "Slack notification: " + status,
		StructuredContent: map[string]any{"status": status, "recipient": user, "duplicate": duplicate}, IsError: status != "sent"}
}

func (p *slackPlugin) InvokeAgentTool(ctx context.Context, req *pluginsdk.AgentToolRequest) (result *pluginsdk.AgentToolResult, err error) {
	// Older hosts omit StructuredContent on MCP errors. Always carry the same
	// safe machine-readable payload in the required fallback text as well.
	defer func() {
		if result != nil {
			raw, _ := json.Marshal(result.StructuredContent)
			result.Text = string(raw)
		}
	}()
	if req != nil && req.Name == "notify_task_user" {
		return p.notifyTaskUser(ctx, req), nil
	}
	if req == nil || req.Name != "notify_user" {
		return notificationResult("invalid_request", "", false), nil
	}
	user, _ := req.Arguments["user_id"].(string)
	text, _ := req.Arguments["text"].(string)
	key, _ := req.Arguments["idempotency_key"].(string)
	c := req.Context
	if !slackUserID.MatchString(user) || strings.TrimSpace(text) == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 4000 || len(key) == 0 || len(key) > 200 ||
		strings.ContainsAny(text, "<>\x00") || strings.Contains(text, "@channel") || strings.Contains(text, "@here") || strings.Contains(text, "@everyone") ||
		c.WorkspaceID == "" || c.TaskID == "" || c.SessionID == "" || (c.Surface != "kanban-task" && c.Surface != "office-task" && c.Surface != "automation") || len(req.Arguments) != 3 {
		return notificationResult("invalid_request", user, false), nil
	}
	return p.deliverNotification(ctx, c.WorkspaceID, user, text, key), nil
}

// An exclusive, plugin-owned journal claim survives restarts. A missing or
// incomplete result is deliberately ambiguous; it never licenses another send.
// Host state has no compare-and-swap, so it cannot arbitrate competing callers.
type notificationRecord struct {
	Fingerprint string         `json:"fingerprint"`
	Result      map[string]any `json:"result,omitempty"`
}

func notificationDigest(parts ...string) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (p *slackPlugin) deliverNotification(ctx context.Context, workspace, user, text, key string) *pluginsdk.AgentToolResult {
	root := os.Getenv("KANDEV_PLUGIN_DATA_DIR")
	if root == "" {
		return notificationResult("storage_unavailable", user, false)
	}
	dir := filepath.Join(root, "notifications-v1")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return notificationResult("storage_unavailable", user, false)
	}
	path := filepath.Join(dir, notificationDigest(workspace, user, key)+".jsonl")
	fingerprint := notificationDigest(text)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return replayNotification(path, fingerprint, user)
	}
	if err != nil {
		return notificationResult("storage_unavailable", user, false)
	}
	defer file.Close()
	record := notificationRecord{Fingerprint: fingerprint}
	if err := appendNotificationRecord(file, record); err != nil {
		return notificationResult("storage_unavailable", user, false)
	}
	result := notificationResult("not_configured", user, false)
	if host := p.Host(); host != nil {
		if cfg, err := host.GetConfig(ctx); err == nil {
			if token, ok := cfg["bot_token"].(string); ok && strings.HasPrefix(token, "xoxb-") {
				result = newClient(token, "").sendNotification(ctx, user, text)
			}
		}
	}
	record.Result = result.StructuredContent
	if err := appendNotificationRecord(file, record); err != nil {
		return notificationResult("unknown", user, false)
	}
	return result
}

func appendNotificationRecord(file *os.File, record notificationRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func replayNotification(path, fingerprint, user string) *pluginsdk.AgentToolResult {
	raw, err := os.ReadFile(path)
	if err != nil {
		return notificationResult("unknown", user, true)
	}
	lines := bytes.Split(raw, []byte{'\n'})
	var record notificationRecord
	// Only newline-terminated records are committed. A crash can leave a tail.
	for _, line := range lines[:len(lines)-1] {
		if err := json.Unmarshal(line, &record); err != nil {
			return notificationResult("unknown", user, true)
		}
		if record.Fingerprint != fingerprint {
			return notificationResult("idempotency_conflict", user, true)
		}
	}
	if record.Result == nil {
		return notificationResult("unknown", user, true)
	}
	status, _ := record.Result["status"].(string)
	result := notificationResult(status, user, true)
	result.StructuredContent = record.Result
	result.StructuredContent["duplicate"] = true
	return result
}

func (c *client) sendNotification(ctx context.Context, user, text string) *pluginsdk.AgentToolResult {
	// Never let redirects replay a POST, even if a proxy returns 307/308.
	transport := *c.http
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	sender := *c
	sender.http = &transport
	params := url.Values{"channel": {user}, "text": {text}, "mrkdwn": {"false"}, "link_names": {"false"}, "unfurl_links": {"false"}, "unfurl_media": {"false"}}
	var response struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := sender.post(ctx, "chat.postMessage", params, &response); err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			if apiErr.StatusCode == 429 {
				result := notificationResult("rate_limited", user, false)
				if apiErr.RetryAfter > 0 {
					result.StructuredContent["retry_after_seconds"] = apiErr.RetryAfter
				}
				return result
			}
			if apiErr.StatusCode == 200 {
				switch apiErr.Message {
				case "missing_scope", "invalid_auth", "token_revoked", "account_inactive", "channel_not_found", "user_not_found", "user_is_bot", "not_authed", "not_allowed_token_type", "ekm_access_denied", "access_denied", "is_archived":
					return notificationResult(apiErr.Message, user, false)
				}
			}
		}
		return notificationResult("unknown", user, false)
	}
	if !strings.HasPrefix(response.Channel, "D") || response.TS == "" {
		return notificationResult("unknown", user, false)
	}
	result := notificationResult("sent", user, false)
	result.StructuredContent["channel"] = response.Channel
	result.StructuredContent["timestamp"] = response.TS
	result.Text = fmt.Sprintf("Slack notification sent to %s.", user)
	return result
}
