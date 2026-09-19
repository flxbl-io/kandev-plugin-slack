package main

import (
	"context"
	"errors"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"sort"
	"strings"
)

type digestItem struct {
	Target                                                                     pluginsdk.ConversationTarget
	Title, Repository, Category, Workspace, UpdatedAt, SessionState, TaskState string
}

func digestPages[T any](read func(pluginsdk.Page) ([]T, *pluginsdk.PageInfo, error)) ([]T, error) {
	var all []T
	page := pluginsdk.Page{Limit: 200}
	seen := map[string]bool{}
	for range 50 {
		rows, next, err := read(page)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
		if next == nil || !next.HasMore {
			return all, nil
		}
		if next.NextCursor == "" || seen[next.NextCursor] {
			return nil, errors.New("invalid pagination")
		}
		seen[next.NextCursor] = true
		page.Cursor = next.NextCursor
	}
	return nil, errors.New("digest scan limit reached")
}
func terminalDigestTask(t pluginsdk.Task) bool {
	switch strings.ToUpper(t.State) {
	case "COMPLETED", "CANCELLED", "DONE":
		return true
	}
	return t.ArchivedAt != nil || t.IsEphemeral
}
func (b *conversationBridge) collectDigest(ctx context.Context, p digestPreference) ([]digestItem, error) {
	h := b.host()
	api, ok := pluginsdk.Attention(h)
	if !ok {
		return nil, errors.New("attention Host API required")
	}
	ws, err := digestPages(func(page pluginsdk.Page) ([]pluginsdk.Workspace, *pluginsdk.PageInfo, error) {
		return h.Workspaces().List(ctx, page)
	})
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, w := range ws {
		names[w.ID] = w.Name
	}
	tasks, err := digestPages(func(page pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error) {
		return h.Tasks().List(ctx, pluginsdk.TaskFilter{}, page)
	})
	if err != nil {
		return nil, err
	}
	var items []digestItem
	seen := map[string]bool{}
	for _, task := range tasks {
		if terminalDigestTask(task) || seen[task.ID] {
			continue
		}
		seen[task.ID] = true
		sessions, err := digestPages(func(page pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
			return h.Sessions().List(ctx, pluginsdk.SessionFilter{TaskIDs: []string{task.ID}}, page)
		})
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			target := pluginsdk.ConversationTarget{ActorID: p.Actor, WorkspaceID: task.WorkspaceID, TaskID: task.ID, SessionID: session.ID}
			err = api.Resolve(ctx, target)
			if err != nil {
				switch grpcstatus.Code(err) {
				case codes.PermissionDenied, codes.NotFound, codes.FailedPrecondition:
					continue
				default:
					return nil, err
				}
			}
			category, err := b.digestCategory(ctx, task, session)
			if err != nil {
				return nil, err
			}
			if category != "" {
				title, repo := digestHeading(task)
				workspace := names[task.WorkspaceID]
				if workspace == "" {
					workspace = task.WorkspaceID
				}
				items = append(items, digestItem{Target: target, Title: title, Repository: repo, Category: category, Workspace: workspace, UpdatedAt: task.UpdatedAt, SessionState: session.State, TaskState: task.State})
			}
			break // Resolve accepted exactly the current primary session.
		}
	}
	sort.Slice(items, func(i, j int) bool {
		a, z := items[i], items[j]
		if a.Workspace != z.Workspace {
			return a.Workspace < z.Workspace
		}
		if a.Category != z.Category {
			return a.Category < z.Category
		}
		if a.UpdatedAt != z.UpdatedAt {
			return a.UpdatedAt < z.UpdatedAt
		}
		return a.Target.TaskID < z.Target.TaskID
	})
	return items, nil
}
func (b *conversationBridge) digestCategory(ctx context.Context, t pluginsdk.Task, s pluginsdk.Session) (string, error) {
	if s.State == "CANCELLED" {
		return "", nil
	}
	if s.State == "FAILED" {
		return "Blocked or failed — open Workfloor to recover", nil
	}
	api, ok := pluginsdk.Interactions(b.host())
	if !ok {
		return "", errors.New("pending interaction reader required")
	}
	rows, _, err := api.ListPending(ctx, pluginsdk.InteractionFilter{TaskIDs: []string{t.ID}, SessionIDs: []string{s.ID}}, pluginsdk.Page{Limit: 1})
	if err != nil {
		return "", err
	}
	if len(rows) > 0 {
		return "Input needed — answer the pending request in Workfloor", nil
	}
	if s.State == "WAITING_FOR_INPUT" {
		return "Input needed — the agent is waiting for you", nil
	}
	if s.State == "RUNNING" || s.State == "STARTING" {
		return "", nil
	}
	if t.State == "FAILED" || t.State == "BLOCKED" {
		return "Blocked or failed — open Workfloor to recover", nil
	}
	if t.WorkflowID != "" {
		steps, err := b.host().Workflows().ListSteps(ctx, t.WorkflowID)
		if err != nil {
			return "", err
		}
		for _, step := range steps {
			if step.ID == t.WorkflowStepID && step.StageType == "review" {
				return "Human review — review the card in Workfloor", nil
			}
		}
	}
	return "", nil
}
