package main

import (
	"context"
	"errors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"os"
	"strings"
	"time"
)

func (b *conversationBridge) process(ctx context.Context, c conversationSettings, key string, item *conversationInbox) error {
	if item.Team != c.Team || item.App != c.App {
		return archiveConversation(key)
	}
	if isDigestCommand(item) {
		return b.digestCommand(ctx, c, key, item)
	}
	if !c.Conversations && !c.GeneralChat {
		return b.guide(ctx, c, key, item, digestUsage)
	}
	var binding conversationBinding
	err := readConversation("bindings", notificationDigest(item.Team, item.Channel, item.Thread), &binding)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	actor := c.actor(item.Team, item.User)
	if c.GeneralChat && (os.IsNotExist(err) || item.Thread == "" || item.GeneralCreate != nil) {
		return b.generalChat(ctx, c, key, item)
	}
	if !c.Conversations {
		return b.guide(ctx, c, key, item, "Card replies are disabled. Open Workfloor to continue this task.")
	}
	if err != nil || item.Thread == "" || actor == "" || binding.User != item.User || binding.Target.ActorID != actor {
		return b.guide(ctx, c, key, item, "Reply in a new card notification thread to continue work. Ask your Workfloor administrator if your Slack account has not been linked.")
	}
	api, ok := pluginsdk.Conversations(b.host())
	if !ok {
		return b.guide(ctx, c, key, item, "Workfloor needs an update before it can receive Slack replies.")
	}
	if err = api.Resolve(ctx, binding.Target); err != nil {
		if transientConversationError(err) {
			return err
		}
		return b.guide(ctx, c, key, item, "This card conversation is no longer available. Open Workfloor to check its assignment and session.")
	}
	var receipt *pluginsdk.ExternalMessageReceipt
	if item.Receipt == "" {
		receipt, err = api.Submit(ctx, pluginsdk.ExternalMessageRequest{Target: binding.Target, EventID: item.Team + ":" + item.Event, Text: item.Text})
		if err == nil && receipt != nil {
			item.Receipt = receipt.ID
			if err = writeConversation("inbox", key, item, false); err != nil {
				return err
			}
		}
	} else {
		receipt, err = api.Get(ctx, actor, item.Receipt)
	}
	if err != nil {
		if transientConversationError(err) {
			return err
		}
		return b.guide(ctx, c, key, item, "This reply could not be applied. Answer any pending question or permission request in Workfloor: "+c.link(binding.Target))
	}
	if receipt == nil {
		return errors.New("missing host receipt")
	}
	if receipt.Status != "completed" {
		if receipt.Status == "uncertain" && time.Since(item.Created) > 2*time.Minute {
			return b.guide(ctx, c, key, item, "Delivery needs checking in Workfloor. Do not resend this instruction until you confirm its result: "+c.link(binding.Target))
		}
		return nil
	}
	text := receipt.Output
	if strings.TrimSpace(text) == "" {
		text = "The agent finished this turn. See Workfloor for details."
	}
	text += "\n\n" + c.link(binding.Target)
	for i, chunk := range conversationChunks(text) {
		// Rights can change while a long response is being delivered.
		if err = api.Resolve(ctx, binding.Target); err != nil {
			return err
		}
		if err = b.postOnce(ctx, c, key+"-answer-"+receipt.TurnID+"-"+string(rune('0'+i)), item.Channel, item.Thread, chunk); err != nil {
			return err
		}
	}
	return archiveConversation(key)
}
func transientConversationError(err error) bool {
	switch grpcstatus.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.Internal, codes.Unknown:
		return true
	}
	return false
}
func (b *conversationBridge) guide(ctx context.Context, c conversationSettings, key string, item *conversationInbox, text string) error {
	thread := item.Thread
	if thread == "" {
		thread = item.TS
	}
	if err := b.postOnce(ctx, c, key+"-guidance", item.Channel, thread, text); err != nil {
		return err
	}
	return archiveConversation(key)
}
func conversationChunks(text string) []string {
	runes := []rune(text)
	if len(runes) > 12000 {
		runes = append(runes[:11500], []rune("\n[Response shortened; open the card for the full answer.]\n"+text[strings.LastIndex(text, "\n")+1:])...)
	}
	text = strings.NewReplacer("@channel", "@ channel", "@here", "@ here", "@everyone", "@ everyone").Replace(string(runes))
	var chunks []string
	var chunk strings.Builder
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	for _, r := range text {
		escaped := escape.Replace(string(r))
		if chunk.Len()+len(escaped) > 2800 {
			chunks = append(chunks, chunk.String())
			chunk.Reset()
		}
		chunk.WriteString(escaped)
	}
	if chunk.Len() > 0 {
		chunks = append(chunks, chunk.String())
	}
	return chunks
}
