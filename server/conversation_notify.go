package main

import (
	"context"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"os"
	"strings"
	"unicode/utf8"
)

func (p *slackPlugin) notifyTaskUser(ctx context.Context, r *pluginsdk.AgentToolRequest) *pluginsdk.AgentToolResult {
	user, _ := r.Arguments["user_id"].(string)
	text, _ := r.Arguments["text"].(string)
	key, _ := r.Arguments["idempotency_key"].(string)
	task, _ := r.Arguments["task_id"].(string)
	session, _ := r.Arguments["session_id"].(string)
	invalid := notificationResult("invalid_request", user, false)
	c := r.Context
	if len(r.Arguments) != 5 || !slackUserID.MatchString(user) || strings.TrimSpace(text) == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 3500 || strings.ContainsAny(text, "<>\x00") || strings.Contains(text, "@channel") || strings.Contains(text, "@here") || strings.Contains(text, "@everyone") || key == "" || len(key) > 160 || task == "" || session == "" || c.WorkspaceID == "" || c.TaskID == "" || c.SessionID == "" {
		return invalid
	}
	if c.Surface != "automation" && (c.Surface != "kanban-task" && c.Surface != "office-task" || task != c.TaskID || session != c.SessionID) {
		return invalid
	}
	cfg, err := p.bridge.settings(ctx)
	if err != nil || !cfg.Conversations {
		return notificationResult("not_configured", user, false)
	}
	actor := cfg.actor(cfg.Team, user)
	if actor == "" {
		return notificationResult("unmapped_user", user, false)
	}
	target := pluginsdk.ConversationTarget{ActorID: actor, WorkspaceID: c.WorkspaceID, TaskID: task, SessionID: session}
	api, ok := pluginsdk.Conversations(p.Host())
	if !ok {
		return notificationResult("host_upgrade_required", user, false)
	}
	if err = api.Resolve(ctx, target); err != nil {
		return notificationResult("target_unavailable", user, false)
	}
	intent := conversationBinding{Team: cfg.Team, User: user, Target: target, Key: key, SourceTask: c.TaskID, SourceSession: c.SessionID, SourceInvocation: r.InvocationID, Fingerprint: notificationDigest(cfg.Team, text, actor, task, session)}
	intentID := notificationDigest(cfg.Team, c.WorkspaceID, user, key)
	if err = writeConversation("intents", intentID, intent, true); os.IsExist(err) {
		var old conversationBinding
		if readConversation("intents", intentID, &old) != nil || old.Fingerprint != intent.Fingerprint {
			return notificationResult("idempotency_conflict", user, true)
		}
	} else if err != nil {
		return notificationResult("storage_unavailable", user, false)
	}
	result := p.deliverNotification(ctx, c.WorkspaceID, user, text+"\n\nReply in this thread to continue this card. Questions and permissions must be answered in Workfloor.", "conversation:"+notificationDigest(cfg.Team, key))
	if result.IsError {
		return result
	}
	intent.Channel, _ = result.StructuredContent["channel"].(string)
	intent.Thread, _ = result.StructuredContent["timestamp"].(string)
	if !strings.HasPrefix(intent.Channel, "D") || intent.Thread == "" {
		return notificationResult("unknown", user, false)
	}
	bindingID := notificationDigest(intent.Team, intent.Channel, intent.Thread)
	if err = writeConversation("bindings", bindingID, intent, true); err != nil && !os.IsExist(err) {
		return notificationResult("binding_pending", user, false)
	}
	return result
}
