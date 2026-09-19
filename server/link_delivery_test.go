package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Slack auto-links bare URLs unless parse=none is sent. Keep its ordinary
// link parsing while disabling markup, mention expansion and previews.
func TestDeliveryPreservesClickableURLs(t *testing.T) {
	const text = "#2419 · Build complete\nhttps://github.com/flxbl-io/sfp-pro/issues/2419\nhttps://workfloor.flxbl.io/?home=overview&workspaceId=workspace&taskId=task&sessionId=session"
	for _, path := range []string{"notification", "conversation-and-digest"} {
		t.Run(path, func(t *testing.T) {
			t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("parse") == "none" {
					t.Error("parse=none disables clickable URLs in Slack")
				}
				if r.Form.Get("text") != text {
					t.Error("URL text or query parameters changed")
				}
				for _, key := range []string{"mrkdwn", "link_names", "unfurl_links", "unfurl_media"} {
					if r.Form.Get(key) != "false" {
						t.Errorf("%s must remain disabled", key)
					}
				}
				fmt.Fprint(w, `{"ok":true,"channel":"D12345678","ts":"12345.000001"}`)
			}))
			t.Cleanup(srv.Close)
			original := slackAPIBase
			slackAPIBase = srv.URL
			t.Cleanup(func() { slackAPIBase = original })
			if path == "notification" {
				req := notificationRequest()
				req.Arguments["text"] = text
				got, err := notificationPlugin().InvokeAgentTool(context.Background(), req)
				if err != nil || got.IsError {
					t.Fatalf("notification failed: %v %+v", err, got)
				}
			} else {
				b := &conversationBridge{}
				if err := b.postOnce(context.Background(), conversationSettings{Token: "xoxb-test-secret"}, "links", "D12345678", "1.0", text); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatalf("Slack calls = %d, want 1", calls)
			}
		})
	}
}
