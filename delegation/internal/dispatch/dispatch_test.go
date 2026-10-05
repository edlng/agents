package dispatch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/catalog"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/task"
)

// submitter answers every request by calling the request's submit tool with
// the payload registered for that tool.
type submitter struct {
	payloads map[string]string
	inputs   []string
}

func (s *submitter) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	s.inputs = append(s.inputs, req.Messages[0].Content[0].Text)
	submit := req.Tools[len(req.Tools)-1].Name
	return provider.Response{ID: "r", Model: req.Model, StopReason: "tool_use", Usage: provider.Usage{CostUSD: 0.01},
		Message: provider.Message{Role: "assistant", Content: []provider.Block{{Type: "tool_use", ID: "t", Name: submit, Input: json.RawMessage(s.payloads[submit])}}}}, nil
}

const approve = `{"verdict":"APPROVE","summary":"ok","findings":[]}`

func runner(t *testing.T, p provider.Provider, limits Limits) *Runner {
	c, err := catalog.Load("../../workflows")
	if err != nil {
		t.Fatal(err)
	}
	tk, err := task.Load("../../fixtures/endpoint-allowlist")
	if err != nil {
		t.Fatal(err)
	}
	log, err := audit.Create(t.TempDir(), "cid")
	if err != nil {
		t.Fatal(err)
	}
	return New(p, log, c, tk, limits)
}

func TestLaunchLimits(t *testing.T) {
	r := runner(t, &submitter{payloads: map[string]string{"submit_review": approve}}, Limits{PerArtifact: 2, PerRun: 3, BudgetUSD: 1})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if res := r.LaunchStep(ctx, "code-review", "security", nil, nil, ""); !res.OK {
			t.Fatalf("launch %d: %+v", i, res.Error)
		}
	}
	if res := r.LaunchStep(ctx, "code-review", "security", nil, nil, ""); res.Error == nil || res.Error.Code != "launch_limit" {
		t.Fatalf("third launch of one step = %+v", res)
	}
	r.LaunchStep(ctx, "code-review", "correctness", nil, nil, "")
	if res := r.LaunchStep(ctx, "code-review", "correctness", nil, nil, ""); res.Error == nil || !strings.Contains(res.Error.Message, "run limit") {
		t.Fatalf("launch past run limit = %+v", res)
	}
}

func TestStepInputsAndRefusals(t *testing.T) {
	r := runner(t, &submitter{}, DefaultLimits)
	ctx := context.Background()
	if res := r.LaunchStep(ctx, "spec-validation", "criteria", nil, nil, ""); res.Error == nil || res.Error.Code != "missing_input" {
		t.Fatalf("criteria before tests = %+v", res)
	}
	if res := r.LaunchStep(ctx, "adversarial-review", "review", nil, nil, ""); res.Error == nil || res.Error.Code != "unknown_workflow" {
		t.Fatalf("reviewer as a client workflow = %+v", res)
	}
	if res := r.LaunchStep(ctx, "code-review", "security", []string{"nope/x"}, nil, ""); res.Error == nil || res.Error.Code != "missing_input" {
		t.Fatalf("unknown context artifact = %+v", res)
	}
}

func TestFinalNeedsCurrentReview(t *testing.T) {
	p := &submitter{payloads: map[string]string{
		"submit_review":       approve,
		"submit_challenges":   `{"reviews":[{"artifact_id":"code-review/security","verdict":"UPHELD","challenges":[]}]}`,
		"submit_final_review": `{"summary":"Approved.","sections":[{"artifact_id":"code-review/security","prose":"No security defects."}]}`,
	}}
	r := runner(t, p, DefaultLimits)
	ctx := context.Background()
	accept := []Disposition{{ArtifactID: "code-review/security", Decision: "accepted", Reason: "upheld"}}

	r.LaunchStep(ctx, "code-review", "security", nil, nil, "")
	if res := r.LaunchFinal(ctx, accept, ""); res.Error == nil || res.Error.Code != "review_pending" {
		t.Fatalf("final before review = %+v", res)
	}
	if res := r.LaunchReview(ctx, []string{"code-review/security"}, ""); !res.OK {
		t.Fatalf("review: %+v", res.Error)
	}
	// Re-running the step makes the review stale.
	r.LaunchStep(ctx, "code-review", "security", nil, json.RawMessage(`[{"id":"C1"}]`), "")
	if !strings.Contains(p.inputs[len(p.inputs)-1], "Prior challenges against your artifact") {
		t.Fatal("challenges not passed to the re-run")
	}
	if got := r.Unreviewed(); len(got) != 1 {
		t.Fatalf("unreviewed after re-run = %v", got)
	}
	r.LaunchReview(ctx, []string{"code-review/security"}, "")
	if res := r.LaunchFinal(ctx, nil, ""); res.Error == nil || res.Error.Code != "invalid_disposition" {
		t.Fatalf("final without dispositions = %+v", res)
	}
	if res := r.LaunchFinal(ctx, accept, ""); !res.OK || r.Final() == nil {
		t.Fatalf("final = %+v", res)
	}
}
