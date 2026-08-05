package main

import (
	"strings"
	"testing"
)

func testTopology() []workspaceTopology {
	return []workspaceTopology{
		{
			ID:   "ws-1",
			Name: "Platform",
			Workflows: []workflowTopology{
				{ID: "wf-1", Name: "Engineering", Steps: []string{"Backlog", "In Progress"}, StepIDs: []string{"step-1", "step-2"}},
			},
		},
		{
			ID:   "ws-2",
			Name: "Marketing",
			Workflows: []workflowTopology{
				{ID: "wf-2", Name: "Campaigns", Steps: []string{"Ideas"}, StepIDs: []string{"step-3"}},
			},
		},
	}
}

func TestParseDecisionPlainJSON(t *testing.T) {
	got, err := parseDecision(`{"workspace_id":"ws-2","workflow_id":"wf-2","column":"Ideas","title":"Fix login","description":"d","reply":"r"}`)
	if err != nil {
		t.Fatalf("parseDecision: %v", err)
	}
	if got.WorkspaceID != "ws-2" || got.Title != "Fix login" || got.Reply != "r" {
		t.Fatalf("decision = %+v", got)
	}
}

// Models wrap JSON in a fence or a sentence often enough that recovering the
// object is worth doing before failing the message and stalling the watermark.
func TestParseDecisionRecoversWrappedJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{name: "code fence", in: "```json\n{\"title\":\"T\"}\n```"},
		{name: "bare fence", in: "```\n{\"title\":\"T\"}\n```"},
		{name: "prose prefix", in: "Sure! Here is the decision:\n{\"title\":\"T\"}"},
		{name: "prose suffix", in: "{\"title\":\"T\"}\nHope that helps."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDecision(tc.in)
			if err != nil {
				t.Fatalf("parseDecision(%q): %v", tc.in, err)
			}
			if got.Title != "T" {
				t.Fatalf("Title = %q, want T", got.Title)
			}
		})
	}
}

func TestParseDecisionRejectsUnusableAnswers(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{name: "empty", in: "   ", wantErr: "empty response"},
		{name: "not json", in: "I could not decide.", wantErr: "did not return JSON"},
		{name: "no title", in: `{"workspace_id":"ws-1"}`, wantErr: "no task title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDecision(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parseDecision error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolvePicksTheNamedTarget(t *testing.T) {
	d := &triageDecision{WorkspaceID: "ws-1", WorkflowID: "wf-1", Column: "In Progress"}
	ws, wf, step := d.resolve(testTopology())
	if ws != "ws-1" || wf != "wf-1" {
		t.Fatalf("resolve = (%q, %q), want ws-1/wf-1", ws, wf)
	}
	if step == nil || *step != "step-2" {
		t.Fatalf("step = %v, want step-2", step)
	}
}

// A hallucinated id must land the task somewhere visible rather than drop the
// user's request.
func TestResolveFallsBackToTheFirstRealTarget(t *testing.T) {
	d := &triageDecision{WorkspaceID: "ws-does-not-exist", WorkflowID: "wf-nope"}
	ws, wf, step := d.resolve(testTopology())
	if ws != "ws-1" || wf != "wf-1" {
		t.Fatalf("resolve = (%q, %q), want the first workspace/workflow", ws, wf)
	}
	if step != nil {
		t.Fatalf("step = %v, want nil so the host applies its default column", step)
	}
}

func TestResolveUnknownColumnDefersToTheHost(t *testing.T) {
	d := &triageDecision{WorkspaceID: "ws-1", WorkflowID: "wf-1", Column: "Nowhere"}
	if _, _, step := d.resolve(testTopology()); step != nil {
		t.Fatalf("step = %v, want nil for an unknown column", step)
	}
}

func TestResolveMatchesColumnCaseInsensitively(t *testing.T) {
	d := &triageDecision{WorkspaceID: "ws-1", WorkflowID: "wf-1", Column: "backlog"}
	_, _, step := d.resolve(testTopology())
	if step == nil || *step != "step-1" {
		t.Fatalf("step = %v, want step-1", step)
	}
}

func TestResolveEmptyTopology(t *testing.T) {
	d := &triageDecision{WorkspaceID: "ws-1"}
	ws, wf, step := d.resolve(nil)
	if ws != "" || wf != "" || step != nil {
		t.Fatalf("resolve(nil) = (%q, %q, %v), want zero values", ws, wf, step)
	}
}

// Every task must link back to the conversation, even when the model leaves
// the provenance out of its description.
func TestBuildDescriptionAlwaysCarriesProvenance(t *testing.T) {
	trigger := message{UserName: "alice"}
	got := buildDescription(&triageDecision{Description: "Do the thing."}, trigger, "https://acme.slack.com/p1")
	if !strings.Contains(got, "Do the thing.") {
		t.Fatalf("description dropped the agent's body: %q", got)
	}
	if !strings.Contains(got, "@alice") || !strings.Contains(got, "https://acme.slack.com/p1") {
		t.Fatalf("description = %q, want requester and permalink", got)
	}
}

func TestBuildDescriptionWithoutPermalink(t *testing.T) {
	got := buildDescription(&triageDecision{}, message{UserID: "U1"}, "")
	if !strings.Contains(got, "<@U1>") || strings.Contains(got, ": http") {
		t.Fatalf("description = %q", got)
	}
}

func TestBuildTriagePromptCarriesContext(t *testing.T) {
	trigger := message{UserName: "alice", TS: "2.0", ChannelID: "C1"}
	thread := []message{{UserName: "bob", TS: "1.0", Text: "only on safari"}}
	got, err := buildTriagePrompt(testTopology(), trigger, "investigate the login bug", "https://acme.slack.com/p2", thread)
	if err != nil {
		t.Fatalf("buildTriagePrompt: %v", err)
	}
	for _, want := range []string{"ws-1", "Engineering", "investigate the login bug", "@alice", "bob (1.0): only on safari", "https://acme.slack.com/p2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

// StepIDs is an internal join column; leaking it invites the model to echo an
// opaque id instead of picking a column by name.
func TestTopologyJSONHidesStepIDs(t *testing.T) {
	got, err := buildTriagePrompt(testTopology(), message{}, "x", "", nil)
	if err != nil {
		t.Fatalf("buildTriagePrompt: %v", err)
	}
	if strings.Contains(got, "step-1") || strings.Contains(got, "StepIDs") {
		t.Fatalf("prompt leaked step ids:\n%s", got)
	}
}

func TestFormatThreadEmpty(t *testing.T) {
	if got := formatThread(nil); got != "" {
		t.Fatalf("formatThread(nil) = %q, want empty", got)
	}
}

func TestFormatThreadLabelsUnknownSenders(t *testing.T) {
	got := formatThread([]message{{TS: "1.0", Text: "hi"}})
	if !strings.Contains(got, "(unknown)") {
		t.Fatalf("formatThread = %q, want an (unknown) label", got)
	}
}
