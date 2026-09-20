package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// notificationCard is deliberately narrower than arbitrary Slack Block Kit.
// The plugin owns layout and actions; callers supply current task facts.
type notificationCard struct {
	Title        string `json:"title"`
	Workspace    string `json:"workspace_name"`
	Attention    string `json:"attention"`
	Summary      string `json:"summary"`
	WorkfloorURL string `json:"workfloor_url"`
	Repository   string `json:"repository,omitempty"`
	IssueNumber  int    `json:"issue_number,omitempty"`
	IssueURL     string `json:"issue_url,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
}
type notificationMessage struct {
	text, blocks, fingerprint string
}

func notificationArguments(args map[string]any, bound bool) bool {
	allowed := map[string]bool{"user_id": true, "text": true, "idempotency_key": true, "card": true}
	required := 3
	if bound {
		allowed["task_id"] = true
		allowed["session_id"] = true
		required = 5
	}
	if _, ok := args["card"]; ok {
		required++
	}
	if len(args) != required {
		return false
	}
	for k := range args {
		if !allowed[k] {
			return false
		}
	}
	return true
}

func (p *slackPlugin) notificationMessage(ctx context.Context, req *pluginsdk.AgentToolRequest, text, footer string) (notificationMessage, string) {
	message := notificationMessage{text: text + footer, fingerprint: notificationDigest(text + footer)}
	raw, present := req.Arguments["card"]
	if !present {
		return message, ""
	}
	invalid := notificationMessage{}
	object, ok := raw.(map[string]any)
	if !ok || object == nil {
		return invalid, "invalid_request"
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return invalid, "invalid_request"
	}
	var card notificationCard
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&card) != nil || !card.validText() {
		return invalid, "invalid_request"
	}
	for _, key := range []string{"repository", "issue_url", "pr_url"} {
		if value, present := object[key]; present {
			if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" {
				return invalid, "invalid_request"
			}
		}
	}
	if _, present := object["issue_number"]; present && card.IssueNumber < 1 {
		return invalid, "invalid_request"
	}
	if p.Host() == nil {
		return invalid, "not_configured"
	}
	config, err := p.Host().GetConfig(ctx)
	if err != nil {
		return invalid, "not_configured"
	}
	origin, _ := config["workfloor_url"].(string)
	if origin == "" {
		return invalid, "not_configured"
	}
	if !card.validURLs(origin, req) {
		return invalid, "invalid_request"
	}
	message.text, message.blocks = card.render(footer, config)
	if utf8.RuneCountInString(message.text) > 4000 {
		return invalid, "invalid_request"
	}
	canonical, _ := json.Marshal(card)
	message.fingerprint = notificationDigest("notification-card-v1", text, string(canonical), footer)
	return message, ""
}

func safeCardText(s string, max int, multiline bool) bool {
	if strings.TrimSpace(s) == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > max || strings.ContainsAny(s, "<>") {
		return false
	}
	for _, mention := range []string{"@channel", "@here", "@everyone"} {
		if strings.Contains(s, mention) {
			return false
		}
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && r == '\n') {
			return false
		}
	}
	return true
}
func (c notificationCard) validText() bool {
	return safeCardText(c.Title, 500, false) && safeCardText(c.Workspace, 100, false) && safeCardText(c.Summary, 1200, true) &&
		(c.Repository == "" || safeCardText(c.Repository, 200, false)) && c.IssueNumber >= 0 &&
		(c.Attention == "input" || c.Attention == "review" || c.Attention == "error")
}
func cardURL(raw string) (*url.URL, bool) {
	if !safeCardText(raw, 800, false) {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, false
	}
	for _, r := range u.Path + u.RawQuery {
		if unicode.IsControl(r) {
			return nil, false
		}
	}
	return u, true
}

var githubCardPath = regexp.MustCompile(`^/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)/(issues|pull)/([1-9][0-9]*)$`)

func (c notificationCard) validURLs(origin string, req *pluginsdk.AgentToolRequest) bool {
	base, ok := cardURL(origin)
	if !ok || (base.Path != "" && base.Path != "/") || base.RawQuery != "" {
		return false
	}
	link, ok := cardURL(c.WorkfloorURL)
	if !ok || !strings.EqualFold(base.Host, link.Host) || (link.Path != "" && link.Path != "/") {
		return false
	}
	q, err := url.ParseQuery(link.RawQuery)
	if err != nil || q.Get("workspaceId") != req.Context.WorkspaceID || q.Get("taskId") == "" || q.Get("sessionId") == "" {
		return false
	}
	for key, values := range q {
		if len(values) != 1 || !safeCardText(values[0], 200, false) || (key != "workspaceId" && key != "taskId" && key != "sessionId" && key != "home") {
			return false
		}
	}
	if req.Name == "notify_task_user" && (q.Get("taskId") != req.Arguments["task_id"] || q.Get("sessionId") != req.Arguments["session_id"]) {
		return false
	}
	for kind, raw := range map[string]string{"issues": c.IssueURL, "pull": c.PRURL} {
		if raw == "" {
			continue
		}
		u, ok := cardURL(raw)
		if !ok || u.Host != "github.com" || u.RawQuery != "" {
			return false
		}
		match := githubCardPath.FindStringSubmatch(u.Path)
		if match == nil || match[2] != kind || (c.Repository != "" && c.Repository != match[1]) {
			return false
		}
		if kind == "issues" && c.IssueNumber > 0 && strconv.Itoa(c.IssueNumber) != match[3] {
			return false
		}
	}
	return true
}
func plainSlackText(s string) map[string]any {
	return map[string]any{"type": "plain_text", "text": s, "emoji": true}
}
func (c notificationCard) render(footer string, config map[string]any) (string, string) {
	title := c.Title
	if c.IssueNumber > 0 {
		title = fmt.Sprintf("#%d · %s", c.IssueNumber, title)
	}
	context := c.Workspace
	if c.Repository != "" {
		context += " · " + c.Repository
	}
	status := map[string]string{"input": "🟡 Needs your decision", "review": "🟣 Ready for review", "error": "🔴 Needs attention"}[c.Attention]
	// Rich-text text elements preserve punctuation literally and cannot turn
	// caller-controlled title content into Slack links or mentions.
	blocks := []any{
		map[string]any{"type": "rich_text", "elements": []any{map[string]any{"type": "rich_text_section", "elements": []any{map[string]any{"type": "text", "text": title, "style": map[string]bool{"bold": true}}}}}},
		map[string]any{"type": "context", "elements": []any{plainSlackText(context)}},
		map[string]any{"type": "divider"},
		map[string]any{"type": "section", "text": plainSlackText(status + "\n" + c.Summary)},
	}
	button := func(id, label, target string, primary bool) map[string]any {
		b := map[string]any{"type": "button", "action_id": id, "text": plainSlackText(label), "url": target}
		if primary {
			b["style"] = "primary"
		}
		return b
	}
	actions := []any{button("open_workfloor", "Open in Workfloor", c.WorkfloorURL, true)}
	fallback := title + "\n" + context + "\n" + status + "\n" + c.Summary + "\n" + c.WorkfloorURL
	if c.IssueURL != "" {
		actions = append(actions, button("open_issue", "View issue", c.IssueURL, false))
		fallback += "\n" + c.IssueURL
	}
	if c.PRURL != "" {
		actions = append(actions, button("open_pr", "View PR", c.PRURL, false))
		fallback += "\n" + c.PRURL
	}
	blocks = append(blocks, map[string]any{"type": "actions", "elements": actions})
	if footer != "" {
		blocks = append(blocks, map[string]any{"type": "context", "elements": []any{plainSlackText(strings.TrimSpace(footer))}})
		fallback += footer
	}
	// Branding is operator-owned presentation, deliberately excluded from the
	// request fingerprint so a rename cannot resend an existing delivery.
	name, identity := notificationIdentity(config)
	if identity != nil && utf8.RuneCountInString(name+"\n"+fallback) <= 4000 {
		blocks = append([]any{identity}, blocks...)
		fallback = name + "\n" + fallback
	}
	encoded, _ := json.Marshal(blocks)
	return fallback, string(encoded)
}

func notificationIdentity(config map[string]any) (string, map[string]any) {
	name, _ := config["notification_brand_name"].(string)
	if !safeCardText(name, 80, false) {
		return "", nil
	}
	elements := []any{}
	icon, _ := config["notification_brand_icon_url"].(string)
	decoded, err := url.PathUnescape(icon)
	if _, ok := cardURL(icon); ok && err == nil && safeCardText(decoded, 800, false) {
		elements = append(elements, map[string]any{"type": "image", "image_url": icon, "alt_text": name})
	}
	elements = append(elements, plainSlackText(name))
	return name, map[string]any{"type": "context", "elements": elements}
}
