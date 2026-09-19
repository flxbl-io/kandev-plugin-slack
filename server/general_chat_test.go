package main

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"strings"
	"testing"
)

func TestGeneralChatRejectsAmbiguousRoutes(t *testing.T) {
	snapshot := &pluginsdk.AssistantSnapshot{Workspaces: []pluginsdk.AssistantWorkspace{{AssistantChoice: pluginsdk.AssistantChoice{ID: "w", Name: "Codev"}, Workflows: []pluginsdk.AssistantChoice{{ID: "flow", Name: "Normal"}}, Repositories: []pluginsdk.AssistantChoice{{ID: "repo", Name: "Core"}}}}}
	intent := generalIntent{Action: "create", Title: "Fix validation"}
	if generalRouteValid(intent, snapshot) {
		t.Fatal("missing workspace accepted")
	}
	intent.WorkspaceID = "w"
	intent.WorkflowID = "flow"
	intent.RepositoryID = "repo"
	if !generalRouteValid(intent, snapshot) {
		t.Fatal("valid route refused")
	}
	intent.RepositoryID = "foreign"
	if generalRouteValid(intent, snapshot) {
		t.Fatal("foreign repository accepted")
	}
}
func TestGeneralChatStrictIntent(t *testing.T) {
	for _, raw := range []string{`{"action":"shell","command":"ls"}`, `{"action":"create","extra":"x"}`, `{"action":"list"} garbage`} {
		if _, err := parseGeneralIntent(raw); err == nil {
			t.Fatalf("unsafe intent accepted %s", raw)
		}
	}
	if _, err := parseGeneralIntent(`{"action":"status","query":"validation"}`); err != nil {
		t.Fatal(err)
	}
}
func TestGeneralChatIdentityAndThreadIsolation(t *testing.T) {
	a := &conversationInbox{Team: "T", Channel: "D", User: "U", TS: "1"}
	b := *a
	b.TS = "2"
	if generalThreadKey(a) == generalThreadKey(&b) {
		t.Fatal("top-level threads conflated")
	}
	b = *a
	b.User = "other"
	if generalThreadKey(a) == generalThreadKey(&b) {
		t.Fatal("different users conflated")
	}
}

type generalTestHost struct {
	*fakeHost
	intents  []string
	requests []pluginsdk.AssistantCreate
	queries  []pluginsdk.AssistantQuery
	snapshot pluginsdk.AssistantSnapshot
	denied   bool
}

func (h *generalTestHost) Assistant() pluginsdk.AssistantAccessor { return h }
func (h *generalTestHost) InvokeProfile(_ context.Context, key, prompt string) (string, error) {
	if key != "general_chat_agent_profile_id" {
		return "", fmt.Errorf("wrong profile key")
	}
	if len(h.intents) == 0 {
		return "", fmt.Errorf("unexpected model call")
	}
	out := h.intents[0]
	h.intents = h.intents[1:]
	return out, nil
}
func (h *generalTestHost) Query(_ context.Context, q pluginsdk.AssistantQuery) (*pluginsdk.AssistantSnapshot, error) {
	h.queries = append(h.queries, q)
	if h.denied || q.ActorID != "human" {
		return nil, grpcstatus.Error(codes.PermissionDenied, "denied")
	}
	if q.TaskID != "" {
		for _, task := range h.snapshot.Tasks {
			if task.ID == q.TaskID {
				return &pluginsdk.AssistantSnapshot{Tasks: []pluginsdk.AssistantTask{task}}, nil
			}
		}
		return nil, grpcstatus.Error(codes.PermissionDenied, "unknown")
	}
	return &h.snapshot, nil
}
func (h *generalTestHost) Create(_ context.Context, q pluginsdk.AssistantCreate) (*pluginsdk.AssistantCreated, error) {
	h.requests = append(h.requests, q)
	task := pluginsdk.AssistantTask{ID: "created", WorkspaceID: q.WorkspaceID, Title: q.Title, State: "TODO", AssigneeID: q.ActorID}
	h.snapshot.Tasks = append(h.snapshot.Tasks, task)
	return &pluginsdk.AssistantCreated{Task: task, Status: "queued"}, nil
}
func generalFixture(t *testing.T) *generalTestHost {
	t.Helper()
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", t.TempDir())
	cfg := conversationConfig()
	cfg["general_chat_enabled"] = true
	return &generalTestHost{fakeHost: newFakeHost(cfg), snapshot: pluginsdk.AssistantSnapshot{Workspaces: []pluginsdk.AssistantWorkspace{{AssistantChoice: pluginsdk.AssistantChoice{ID: "w", Name: "Codev"}, Workflows: []pluginsdk.AssistantChoice{{ID: "flow", Name: "Normal"}}, Repositories: []pluginsdk.AssistantChoice{{ID: "repo", Name: "Core"}}}}, Tasks: []pluginsdk.AssistantTask{{ID: "task", WorkspaceID: "w", Title: "A real issue", State: "REVIEW", AssigneeID: "human"}}}}
}
func TestGeneralChatDMStatusCardAndReplay(t *testing.T) {
	h := generalFixture(t)
	h.intents = []string{`{"action":"list"}`}
	posts := captureCardPosts(t)
	b := &conversationBridge{host: func() pluginsdk.Host { return h }}
	if err := b.persist(context.Background(), dmEvent("general-status", "")); err != nil {
		t.Fatal(err)
	}
	b.reconcile(context.Background())
	if len(*posts) != 1 || !strings.Contains((*posts)[0].Get("blocks"), "Open in Workfloor") || (*posts)[0].Get("thread_ts") != "2.000001" {
		t.Fatalf("wrong DM card: %+v", posts)
	}
	b = &conversationBridge{host: func() pluginsdk.Host { return h }}
	_ = b.persist(context.Background(), dmEvent("general-status", ""))
	b.reconcile(context.Background())
	if len(*posts) != 1 || len(h.requests) != 0 {
		t.Fatal("replay sent or created twice")
	}
}
func TestGeneralChatDMCreateAndRevocation(t *testing.T) {
	h := generalFixture(t)
	posts := captureCardPosts(t)
	h.intents = []string{`{"action":"create"}`, `{"action":"create","workspace_id":"w","workflow_id":"flow","repository_id":"repo","title":"Test DM task","description":"From Slack","start":false}`}
	b := &conversationBridge{host: func() pluginsdk.Host { return h }}
	_ = b.persist(context.Background(), dmEvent("general-create", ""))
	b.reconcile(context.Background())
	if len(h.requests) != 1 || h.requests[0].ActorID != "human" || h.requests[0].Start || len(*posts) != 1 {
		t.Fatalf("create path: %+v posts=%d", h.requests, len(*posts))
	}
	_ = b.persist(context.Background(), dmEvent("general-create", ""))
	b.reconcile(context.Background())
	if len(h.requests) != 1 {
		t.Fatal("replayed creation")
	}
	h.denied = true
	_ = b.persist(context.Background(), dmEvent("revoked", ""))
	b.reconcile(context.Background())
	if len(h.intents) != 0 || len(h.requests) != 1 {
		t.Fatal("revoked human reached model or create")
	}
}
