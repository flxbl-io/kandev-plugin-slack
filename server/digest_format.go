package main

import (
	"fmt"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"net/url"
	"regexp"
	"strings"
)

var digestIssuePath = regexp.MustCompile(`^/([^/]+)/([^/]+)/issues/([1-9][0-9]*)/?$`)

func digestPlain(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.NewReplacer("<", "(", ">", ")", "@channel", "@ channel", "@here", "@ here", "@everyone", "@ everyone").Replace(s)
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}
func digestHeading(t pluginsdk.Task) (string, string) {
	title := t.Title
	raw, _ := t.Metadata["issue_url"].(string)
	u, err := url.Parse(raw)
	if err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		parts := digestIssuePath.FindStringSubmatch(u.Path)
		if len(parts) == 4 {
			if issueTitle, ok := t.Metadata["issue_title"].(string); ok && strings.TrimSpace(issueTitle) != "" {
				title = issueTitle
			}
			title = strings.TrimPrefix(title, "Issue #"+parts[3]+": ")
			return "#" + parts[3] + " · " + digestPlain(title, 150), parts[1] + "/" + parts[2]
		}
	}
	return digestPlain(title, 170), ""
}
func formatDigest(c conversationSettings, p digestPreference, date string, items []digestItem) string {
	if len(items) == 0 {
		return ""
	}
	text := fmt.Sprintf("Daily attention digest · %s (%s)\n%d tasks need your attention.\n", date, p.Zone, len(items))
	shown := 0
	workspace := ""
	for _, item := range items {
		block := ""
		if item.Workspace != workspace {
			block = "\n" + digestPlain(item.Workspace, 80) + "\n"
		}
		block += "\n" + item.Title + "\n"
		if item.Repository != "" {
			block += item.Repository + "\n"
		}
		block += item.Category + "\n" + c.link(item.Target) + "\n"
		if len(text)+len(block) > 3200 {
			break
		}
		text += block
		shown++
		workspace = item.Workspace
	}
	if shown < len(items) {
		text += fmt.Sprintf("\n%d additional tasks need attention. Open Workfloor: %s\n", len(items)-shown, c.BaseURL)
	}
	return text + "\nUse digest status or digest off in a new DM. Open an individual card to act."
}
