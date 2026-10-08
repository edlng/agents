// Package harness wires one review run: it loads the task and catalogs, runs
// the coordinator with a Dispatcher backed by dispatch.Runner, assembles the
// report, applies the substance gate, and writes the run record. The provider
// is injected, so the graded binary passes the API provider and dev tools
// under evals/ pass the CLI provider.
package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/edlng/agents/litmus-eval/delegation/internal/audit"
	"github.com/edlng/agents/litmus-eval/delegation/internal/catalog"
	"github.com/edlng/agents/litmus-eval/delegation/internal/coordinator"
	"github.com/edlng/agents/litmus-eval/delegation/internal/dispatch"
	"github.com/edlng/agents/litmus-eval/delegation/internal/gate"
	"github.com/edlng/agents/litmus-eval/delegation/internal/provider"
	"github.com/edlng/agents/litmus-eval/delegation/internal/report"
	"github.com/edlng/agents/litmus-eval/delegation/internal/schema"
	"github.com/edlng/agents/litmus-eval/delegation/internal/subagent"
	"github.com/edlng/agents/litmus-eval/delegation/internal/task"
)

type Config struct {
	Provider provider.Provider
	// ProviderName is recorded in run.json and the audit trail, for example
	// "anthropic-api" for the graded runtime or "replay".
	ProviderName   string
	TaskDir        string
	WorkflowsDir   string // default delegation/workflows
	CoordinatorDir string // default delegation/coordinator
	RunsRoot       string // default delegation/runs
	Limits         dispatch.Limits
	// CoordinatorModel and WorkerModel override the manifests when set.
	CoordinatorModel string
	WorkerModel      string
}

type Result struct {
	CorrelationID string  `json:"correlation_id"`
	Dir           string  `json:"dir"`
	Status        string  `json:"status"`
	CostUSD       float64 `json:"cost_usd"`
}

func (c *Config) defaults() {
	if c.WorkflowsDir == "" {
		c.WorkflowsDir = "delegation/workflows"
	}
	if c.CoordinatorDir == "" {
		c.CoordinatorDir = "delegation/coordinator"
	}
	if c.RunsRoot == "" {
		c.RunsRoot = "delegation/runs"
	}
	if c.Limits == (dispatch.Limits{}) {
		c.Limits = dispatch.DefaultLimits
	}
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	cfg.defaults()
	cat, err := catalog.Load(cfg.WorkflowsDir)
	if err != nil {
		return Result{}, err
	}
	if cfg.WorkerModel != "" {
		for _, w := range cat.Workflows {
			w.Model = cfg.WorkerModel
		}
	}
	man, err := coordinator.LoadManifest(cfg.CoordinatorDir)
	if err != nil {
		return Result{}, err
	}
	if cfg.CoordinatorModel != "" {
		man.Model = cfg.CoordinatorModel
	}
	for _, t := range man.Tools {
		if t.Workflow != "human" && cat.Workflows[t.Workflow] == nil {
			return Result{}, fmt.Errorf("coordinator tool %s targets unknown workflow %s", t.Name, t.Workflow)
		}
	}
	tk, err := task.Load(cfg.TaskDir)
	if err != nil {
		return Result{}, err
	}
	log, err := audit.Create(cfg.RunsRoot, audit.NewID())
	if err != nil {
		return Result{}, err
	}
	res := Result{CorrelationID: log.CorrelationID(), Dir: log.Dir()}
	scratch, err := os.MkdirTemp("", "delegate-run-"+log.CorrelationID()+"-")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(scratch)
	h := &dispatcher{runner: dispatch.New(cfg.Provider, log, cat, tk, cfg.Limits), log: log, man: man}
	h.runner.Scratch = scratch
	substance := gate.Assess(cat)

	outcome, runErr := coordinator.Run(ctx, cfg.Provider, log, man, h, brief(tk, cat, substance))
	rec := gate.RunRecord{
		CorrelationID: log.CorrelationID(), TaskID: tk.ID, Provider: cfg.ProviderName, Workflows: cfg.WorkflowsDir, Substance: substance,
		Requests: h.requests, Coordinator: outcome, CostUSD: h.runner.Spent() + outcome.CostUSD,
	}
	switch {
	case runErr != nil:
		rec.Status = gate.FailedAutomated
	case h.runner.Final() == nil:
		rec.Status = gate.FailedAutomated
		runErr = fmt.Errorf("coordinator stopped (%s) without a final review", outcome.StopReason)
	case substance.Elevated:
		rec.Status = gate.Elevated
	default:
		rec.Status = gate.PendingHuman
	}
	if rec.Status != gate.FailedAutomated {
		md, err := buildReport(h.runner, cat, tk, log, rec)
		if err != nil {
			rec.Status, runErr = gate.FailedAutomated, err
		} else {
			sum := sha256.Sum256([]byte(md))
			rec.ReportSHA256 = hex.EncodeToString(sum[:])
			if err := os.WriteFile(filepath.Join(log.Dir(), "report.md"), []byte(md), 0o644); err != nil {
				return res, err
			}
		}
	}
	res.Status, res.CostUSD = rec.Status, rec.CostUSD
	recJSON, _ := json.MarshalIndent(rec, "", "  ")
	if err := os.WriteFile(filepath.Join(log.Dir(), "run.json"), append(recJSON, '\n'), 0o644); err != nil {
		return res, err
	}
	detail := map[string]any{"status": rec.Status, "report_sha256": rec.ReportSHA256, "provider": cfg.ProviderName}
	if runErr != nil {
		detail["error"] = runErr.Error()
	}
	if _, err := log.Append(audit.Event{Kind: audit.KindTerminal, Agent: "harness", Detail: mustJSON(detail)}); err != nil {
		return res, err
	}
	if err := log.Close(); err != nil {
		return res, err
	}
	return res, runErr
}

// brief is the coordinator's first message: the task without the patch, and
// the workflow catalog. The coordinator never sees code.
func brief(tk *task.Task, cat *catalog.Catalog, s gate.Substance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Review task %s: %s\n\n%s\n\nAcceptance criteria:\n", tk.ID, tk.Title, tk.Description)
	for _, c := range tk.AcceptanceCriteria {
		fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Text)
	}
	b.WriteString("\n# Workflow catalog\n")
	for _, w := range cat.Client() {
		fmt.Fprintf(&b, "\n## %s (substance: %s)\n%s\nSteps:\n", w.ID, w.Substance, w.Purpose)
		for _, st := range w.Steps {
			fmt.Fprintf(&b, "- %s → artifact %s", st.ID, dispatch.ArtifactID(w.ID, st.ID))
			if len(st.Inputs) > 0 {
				fmt.Fprintf(&b, " (needs %s)", strings.Join(st.Inputs, ", "))
			}
			fmt.Fprintf(&b, ": %s\n", st.Description)
		}
	}
	if s.Elevated {
		fmt.Fprintf(&b, "\nNote: the substance gate has elevated this review (%s). A human must record continue or reject after you finish.\n", strings.Join(s.Reasons, "; "))
	}
	return b.String()
}

type dispatcher struct {
	runner   *dispatch.Runner
	log      *audit.Log
	man      *coordinator.Manifest
	requests []string
}

// Dispatch logs the call and the harness's answer, so refusals (limits,
// budget, unreviewed artifacts, bad arguments) are in the audit trail and not
// only in the coordinator's reasoning.
func (h *dispatcher) Dispatch(ctx context.Context, tool string, input json.RawMessage, parent string) (json.RawMessage, error) {
	if _, err := h.log.Append(audit.Event{Kind: audit.KindToolCall, ParentSpanID: parent, Agent: "coordinator",
		Detail: mustJSON(map[string]any{"tool": tool, "input": input})}); err != nil {
		return nil, err
	}
	out, err := h.route(ctx, tool, input, parent)
	if err != nil {
		return nil, err
	}
	var summary struct {
		OK         bool            `json:"ok"`
		ArtifactID string          `json:"artifact_id,omitempty"`
		Error      json.RawMessage `json:"error,omitempty"`
	}
	json.Unmarshal(out, &summary)
	if _, err := h.log.Append(audit.Event{Kind: audit.KindToolResult, ParentSpanID: parent, Agent: "harness",
		Detail: mustJSON(map[string]any{"tool": tool, "ok": summary.OK, "artifact_id": summary.ArtifactID, "error": summary.Error})}); err != nil {
		return nil, err
	}
	return out, nil
}

func (h *dispatcher) route(ctx context.Context, tool string, input json.RawMessage, parent string) (json.RawMessage, error) {
	var res subagent.Result
	switch tool {
	case "launch_code_reviewer", "launch_spec_validator", "launch_documenter":
		var in struct {
			Step             string          `json:"step"`
			ContextArtifacts []string        `json:"context_artifacts"`
			PriorChallenges  json.RawMessage `json:"prior_challenges"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return badArgs(err), nil
		}
		res = h.runner.LaunchStep(ctx, h.man.Workflow(tool), in.Step, in.ContextArtifacts, in.PriorChallenges, parent)
	case "launch_adversarial_reviewer":
		var in struct {
			ArtifactIDs []string `json:"artifact_ids"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return badArgs(err), nil
		}
		res = h.runner.LaunchReview(ctx, in.ArtifactIDs, parent)
	case "launch_final_review_writer":
		var in struct {
			Dispositions []dispatch.Disposition `json:"dispositions"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return badArgs(err), nil
		}
		res = h.runner.LaunchFinal(ctx, in.Dispositions, parent)
	case "invoke_human_review":
		var in struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(input, &in); err != nil || strings.TrimSpace(in.Reason) == "" {
			return badArgs(fmt.Errorf("reason is required")), nil
		}
		h.requests = append(h.requests, in.Reason)
		if _, err := h.log.Append(audit.Event{Kind: audit.KindGate, ParentSpanID: parent, Agent: "coordinator",
			Detail: mustJSON(map[string]string{"request": in.Reason})}); err != nil {
			return nil, err
		}
		return mustJSON(map[string]any{"ok": true, "recorded": true, "note": "The run will end awaiting a human; you cannot approve it."}), nil
	default:
		return mustJSON(map[string]any{"ok": false, "error": map[string]string{"code": "unknown_tool", "message": tool}}), nil
	}
	// The coordinator sees the result without the written doc bodies; the
	// reviewer and writer get those directly from the runner.
	res.Files = nil
	return mustJSON(res), nil
}

func badArgs(err error) json.RawMessage {
	return mustJSON(map[string]any{"ok": false, "error": map[string]string{"code": "invalid_arguments", "message": err.Error()}})
}

func buildReport(r *dispatch.Runner, cat *catalog.Catalog, tk *task.Task, log *audit.Log, rec gate.RunRecord) (string, error) {
	final := r.Final()
	var fr schema.FinalReview
	if err := json.Unmarshal(final.Artifact, &fr); err != nil {
		return "", err
	}
	prose := map[string]string{}
	for _, s := range fr.Sections {
		prose[s.ArtifactID] = s.Prose
	}
	disp := map[string]dispatch.Disposition{}
	for _, d := range r.Dispositions() {
		disp[d.ArtifactID] = d
	}
	in := report.Input{
		TaskID: tk.ID, Title: tk.Title, CorrelationID: log.CorrelationID(), Status: rec.Status,
		Summary: fr.Summary, Requests: rec.Requests,
	}
	for _, row := range rec.Substance.Rows {
		in.Substance = append(in.Substance, [2]string{row.Workflow, row.Substance})
	}
	if rec.Substance.Elevated {
		in.Elevation = rec.Substance.Reasons
	}
	ids := r.ArtifactIDs()
	sort.Strings(ids)
	for _, id := range ids {
		a, _ := r.Artifact(id)
		rv := r.Review(id)
		critical := 0
		for _, c := range rv.Challenges {
			if c.Severity == "critical" {
				critical++
			}
		}
		in.Artifacts = append(in.Artifacts, report.Artifact{
			ID: id, Verdict: a.Verdict, Raw: a.Artifact, Review: rv,
			Decision: disp[id].Decision, Reason: disp[id].Reason,
			Workflow: a.Workflow, Substance: cat.Workflows[a.Workflow].Substance,
			Prose: prose[id], OpenCritics: critical,
		})
	}
	sum, err := audit.Verify(filepath.Join(log.Dir(), "audit.jsonl"))
	if err != nil {
		return "", err
	}
	in.CostUSD, in.Calls = sum.Total.CostUSD, sum.Total.Calls
	in.Tokens = sum.Total.InputTokens + sum.Total.OutputTokens + sum.Total.CacheRead + sum.Total.CacheWrite
	return report.Build(in)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
