package pluginsdk

import (
	"context"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

// ConversationHost is optional. Old hosts return Unimplemented over gRPC.
// Declaring api_write:conversations trusts the plugin to authenticate mapped
// humans. The host still checks their current assignment and workspace rights.
type ConversationHost interface{ Conversations() ConversationAccessor }
type ConversationTarget struct{ ActorID, WorkspaceID, TaskID, SessionID string }
type ExternalMessageRequest struct {
	Target        ConversationTarget
	EventID, Text string
}
type ExternalMessageReceipt struct{ ID, Status, QueueID, TurnID, Output string }
type ConversationAccessor interface {
	Resolve(context.Context, ConversationTarget) error
	Submit(context.Context, ExternalMessageRequest) (*ExternalMessageReceipt, error)
	Get(context.Context, string, string) (*ExternalMessageReceipt, error)
}

func Conversations(host Host) (ConversationAccessor, bool) {
	h, ok := host.(ConversationHost)
	if !ok {
		return nil, false
	}
	return h.Conversations(), true
}
func (t ConversationTarget) toProto() *pluginv1.ConversationTarget {
	return &pluginv1.ConversationTarget{ActorId: t.ActorID, WorkspaceId: t.WorkspaceID, TaskId: t.TaskID, SessionId: t.SessionID}
}
func conversationTargetFromProto(t *pluginv1.ConversationTarget) ConversationTarget {
	return ConversationTarget{t.GetActorId(), t.GetWorkspaceId(), t.GetTaskId(), t.GetSessionId()}
}
func (r *ExternalMessageReceipt) toProto() *pluginv1.ExternalMessageReceipt {
	if r == nil {
		return nil
	}
	return &pluginv1.ExternalMessageReceipt{Id: r.ID, Status: r.Status, QueueId: r.QueueID, TurnId: r.TurnID, Output: r.Output}
}
func receiptFromProto(r *pluginv1.ExternalMessageReceipt) *ExternalMessageReceipt {
	return &ExternalMessageReceipt{r.GetId(), r.GetStatus(), r.GetQueueId(), r.GetTurnId(), r.GetOutput()}
}
func (h *grpcHostClient) Conversations() ConversationAccessor {
	return grpcConversationAccessor{h.client}
}

type grpcConversationAccessor struct{ client pluginv1.HostClient }

func (a grpcConversationAccessor) Resolve(ctx context.Context, t ConversationTarget) error {
	_, err := a.client.ResolveConversation(ctx, t.toProto())
	return err
}
func (a grpcConversationAccessor) Submit(ctx context.Context, r ExternalMessageRequest) (*ExternalMessageReceipt, error) {
	out, err := a.client.SubmitExternalMessage(ctx, &pluginv1.ExternalMessageRequest{Target: r.Target.toProto(), EventId: r.EventID, Text: r.Text})
	if err != nil {
		return nil, err
	}
	return receiptFromProto(out), nil
}
func (a grpcConversationAccessor) Get(ctx context.Context, actor, id string) (*ExternalMessageReceipt, error) {
	out, err := a.client.GetExternalMessage(ctx, &pluginv1.ExternalMessageLookup{ActorId: actor, ReceiptId: id})
	if err != nil {
		return nil, err
	}
	return receiptFromProto(out), nil
}
func (s *grpcHostServer) conversationAccessor() (ConversationAccessor, error) {
	if h, ok := s.impl.(ConversationHost); ok {
		return h.Conversations(), nil
	}
	return nil, errUnimplementedHostData("conversations")
}
func (s *grpcHostServer) ResolveConversation(ctx context.Context, in *pluginv1.ConversationTarget) (*pluginv1.ConversationTarget, error) {
	a, err := s.conversationAccessor()
	if err != nil {
		return nil, err
	}
	if err = a.Resolve(ctx, conversationTargetFromProto(in)); err != nil {
		return nil, err
	}
	return in, nil
}
func (s *grpcHostServer) SubmitExternalMessage(ctx context.Context, in *pluginv1.ExternalMessageRequest) (*pluginv1.ExternalMessageReceipt, error) {
	a, err := s.conversationAccessor()
	if err != nil {
		return nil, err
	}
	r, err := a.Submit(ctx, ExternalMessageRequest{Target: conversationTargetFromProto(in.GetTarget()), EventID: in.GetEventId(), Text: in.GetText()})
	if err != nil {
		return nil, err
	}
	return r.toProto(), nil
}
func (s *grpcHostServer) GetExternalMessage(ctx context.Context, in *pluginv1.ExternalMessageLookup) (*pluginv1.ExternalMessageReceipt, error) {
	a, err := s.conversationAccessor()
	if err != nil {
		return nil, err
	}
	r, err := a.Get(ctx, in.GetActorId(), in.GetReceiptId())
	if err != nil {
		return nil, err
	}
	return r.toProto(), nil
}
