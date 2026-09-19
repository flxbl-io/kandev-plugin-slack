package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"time"
)

type conversationDelivery struct {
	Status    string
	RetryAt   time.Time
	Timestamp string
}

var errConversationDeliveryUncertain = errors.New("Slack delivery uncertain; inspect the thread before any manual resend")

func (b *conversationBridge) postOnce(ctx context.Context, c conversationSettings, key, channel, thread, text string) error {
	key = notificationDigest(key)
	record := conversationDelivery{Status: "sending"}
	err := writeConversation("outbox", key, record, true)
	if os.IsExist(err) {
		if err = readConversation("outbox", key, &record); err != nil {
			return err
		}
		switch record.Status {
		case "sent":
			return nil
		case "rate_limited":
			if time.Now().Before(record.RetryAt) {
				return errors.New("Slack retry-after pending")
			}
			attemptKey := notificationDigest(key, record.RetryAt.UTC().Format(time.RFC3339Nano))
			if claimErr := writeConversation("outbox-attempts", attemptKey, record, true); claimErr != nil {
				return errConversationDeliveryUncertain
			}
			record.Status = "sending"
			if err = writeConversation("outbox", key, record, false); err != nil {
				return err
			}
		default:
			return errConversationDeliveryUncertain
		}
	} else if err != nil {
		return err
	}
	client := newClient(c.Token, "")
	transport := *client.http
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.http = &transport
	var response struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	err = client.post(ctx, "chat.postMessage", url.Values{"channel": {channel}, "thread_ts": {thread}, "text": {text}, "mrkdwn": {"false"}, "parse": {"none"}, "link_names": {"false"}, "unfurl_links": {"false"}, "unfurl_media": {"false"}}, &response)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 429 {
			record.Status = "rate_limited"
			record.RetryAt = time.Now().Add(time.Duration(max(1, apiErr.RetryAfter)) * time.Second)
			if saveErr := writeConversation("outbox", key, record, false); saveErr != nil {
				return saveErr
			}
			return errors.New("Slack rate limited")
		}
		return errConversationDeliveryUncertain
	}
	if response.Channel != channel || response.TS == "" {
		return errConversationDeliveryUncertain
	}
	record.Status = "sent"
	record.Timestamp = response.TS
	return writeConversation("outbox", key, record, false)
}
