package main

import (
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"net/url"
	"strings"
)

type conversationSettings struct {
	Team, App, BaseURL, Token string
	Users                     map[string]string
	Digests, Conversations    bool
	GeneralChat, GeneralStart bool
}

func loadConversations(raw map[string]any) (conversationSettings, error) {
	c := conversationSettings{Team: configString(raw, "conversation_team_id"), App: configString(raw, "conversation_app_id"), BaseURL: configString(raw, "workfloor_url"), Token: configString(raw, "bot_token")}
	c.GeneralChat = configBool(raw, "general_chat_enabled")
	c.GeneralStart = true
	if _, set := raw["general_chat_start_agent"]; set {
		c.GeneralStart = configBool(raw, "general_chat_start_agent")
	}
	c.Digests = configBool(raw, "digests_enabled")
	c.Conversations = configBool(raw, "conversations_enabled")
	if !c.Conversations && !c.Digests && !c.GeneralChat {
		return c, errors.New("conversations disabled")
	}
	if !strings.HasPrefix(c.Team, "T") || !strings.HasPrefix(c.App, "A") || !strings.HasPrefix(c.Token, "xoxb-") {
		return c, errors.New("conversation Slack app identity required")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return c, errors.New("HTTPS Workfloor origin required")
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if err = json.Unmarshal([]byte(configString(raw, "conversation_users")), &c.Users); err != nil || len(c.Users) == 0 {
		return c, errors.New("operator-managed conversation user mapping required")
	}
	return c, nil
}
func (c conversationSettings) actor(team, user string) string {
	if team != c.Team {
		return ""
	}
	return c.Users[team+":"+user]
}
func (c conversationSettings) link(t pluginsdk.ConversationTarget) string {
	q := url.Values{"workspaceId": {t.WorkspaceID}, "taskId": {t.TaskID}, "sessionId": {t.SessionID}}
	return c.BaseURL + "/?" + q.Encode()
}
