package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/edlng/agents/litmus-eval/litmus/internal/litmus"
)

func TestParseProbeCommand(t *testing.T) {
	command, err := parseArgs([]string{"probe", "code-reviewer", "eval-exec-injection", "--budget", "0.10"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Name != "probe" || command.Agent != "code-reviewer" ||
		command.CaseID != "eval-exec-injection" || command.BudgetUSD != 0.10 {
		t.Fatalf("parseArgs() = %#v", command)
	}
}

func TestParseWorkflowCommands(t *testing.T) {
	probe, err := parseArgs([]string{
		"workflow-probe", "node-safe-merge", "--budget", "2.50",
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Name != "workflow-probe" || probe.Agent != "team-lead" ||
		probe.CaseID != "node-safe-merge" || probe.WorkflowName != "stage4-team-lead" ||
		probe.BudgetUSD != 2.50 {
		t.Fatalf("workflow probe = %#v", probe)
	}
	batch, err := parseArgs([]string{
		"workflow-batch", "stage4-team-lead-implementation", "--budget", "45",
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Name != "workflow-batch" ||
		batch.Manifest != "stage4-team-lead-implementation" ||
		batch.WorkflowName != "stage4-team-lead" || batch.Jobs != 1 {
		t.Fatalf("workflow batch = %#v", batch)
	}
}

func TestRunWorkflowProbePersistsFiveStepsAsPendingHuman(t *testing.T) {
	root := measuredWorkflowCLIRoot(t, []string{"case"})
	executor := &workflowCLIExecutor{}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"workflow-probe", "case", "--budget", "2.50"},
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run(workflow-probe) = %d, stderr = %s", code, stderr.String())
	}
	run := readOnlyRun(t, root)
	if executor.calls != 5 || len(run.Cases) != 1 ||
		run.Cases[0].Workflow == nil ||
		run.Cases[0].Workflow.Status != litmus.WorkflowPendingHuman ||
		len(run.Cases[0].Workflow.Steps) != 5 {
		t.Fatalf("run = %#v, calls = %d; want five-step PENDING_HUMAN evidence", run, executor.calls)
	}
	if !strings.Contains(stdout.String(), "workflow=PENDING_HUMAN") ||
		strings.Contains(stdout.String(), "COMPLETED") ||
		strings.Contains(stdout.String(), "APPROVED") {
		t.Fatalf("workflow probe output = %q, want explicit unauthenticated pending state", stdout.String())
	}
}

func TestRunWorkflowBatchIsSerialAndSharesGlobalBudget(t *testing.T) {
	root := measuredWorkflowCLIRoot(t, []string{"first", "second"})
	executor := &workflowCLIExecutor{}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"workflow-batch", "measured", "--budget", "2.50"},
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run(workflow-batch) = %d, stderr = %s", code, stderr.String())
	}
	run := readOnlyRun(t, root)
	if executor.calls != 10 || len(run.Cases) != 2 {
		t.Fatalf("run = %#v, calls = %d; want two serial five-step workflows", run, executor.calls)
	}
	for _, result := range run.Cases {
		if result.Workflow == nil || result.Workflow.Status != litmus.WorkflowPendingHuman ||
			result.Workflow.Totals.CostUSD != 0.05 {
			t.Fatalf("workflow result = %#v, want reconciled pending workflow", result)
		}
	}
}

func TestRunWorkflowBatchStopsAfterGlobalBudgetExhaustion(t *testing.T) {
	root := measuredWorkflowCLIRoot(t, []string{"first", "second"})
	executor := &workflowCLIExecutor{firstCost: 0.40}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"workflow-batch", "measured", "--budget", "0.75"},
		&stdout,
		&stderr,
	)
	if code == 0 {
		t.Fatalf("run(workflow-batch) = %d, want budget failure", code)
	}
	run := readOnlyRun(t, root)
	if executor.calls != 1 || len(run.Cases) != 1 ||
		run.Cases[0].Workflow == nil ||
		run.Cases[0].Workflow.Failure == nil ||
		run.Cases[0].Workflow.Failure.Code != "BUDGET_EXHAUSTED" {
		t.Fatalf("run = %#v, calls = %d; want stop after first case exhausts global budget", run, executor.calls)
	}
}

func TestParseProbeRejectsMissingCase(t *testing.T) {
	_, err := parseArgs([]string{"probe", "code-reviewer"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("parseArgs() error = %v, want usage error", err)
	}
}

func TestParseGradeDefaultsToSerial(t *testing.T) {
	command, err := parseArgs([]string{"grade", "litmus/results/run", "--budget", "0.03"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Name != "grade" || command.RunDirectory != "litmus/results/run" ||
		command.BudgetUSD != 0.03 || command.Jobs != 1 {
		t.Fatalf("parseArgs() = %#v, want serial grade command", command)
	}
}

func TestRunGradeWritesSeparateArtifact(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "reviewer")
	writeCase(t, root, "reviewer", "case", `{
		"id": "case",
		"agent": "reviewer",
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}],
		"model_grader": {
			"enabled": true,
			"model": "claude-haiku-4-5",
			"rubric": "Judge the answer.",
			"max_budget_usd": 0.03
		}
	}`)
	runDirectory := writeRun(t, root, "grade", true, 0.04)
	executor := &gradeExecutor{output: `{"pass":true,"score":1,"reason":"approved."}`}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"grade", runDirectory, "--budget", "0.03"},
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run(grade) = %d, stderr = %s", code, stderr.String())
	}
	if executor.calls != 1 {
		t.Fatalf("grader calls = %d, want 1", executor.calls)
	}
	if _, err := os.Stat(filepath.Join(runDirectory, "grader.json")); err != nil {
		t.Fatalf("grader artifact missing: %v", err)
	}
}

func TestParseFullBatchCommand(t *testing.T) {
	command, err := parseArgs([]string{
		"batch", "full", "--budget", "1000", "--include-replay-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Name != "batch" || command.Manifest != "full" ||
		command.BudgetUSD != 1000 || !command.IncludeReplayOnly {
		t.Fatalf("parseArgs() = %#v, want full batch override", command)
	}
}

func TestParseBatchJobs(t *testing.T) {
	command, err := parseArgs([]string{
		"batch", "full", "--budget", "1.50", "--jobs", "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Jobs != 2 {
		t.Fatalf("jobs = %d, want 2", command.Jobs)
	}
}

func TestParseBatchDefaultsToThreeJobs(t *testing.T) {
	command, err := parseArgs([]string{"batch", "core", "--budget", "0.80"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Jobs != 3 {
		t.Fatalf("jobs = %d, want 3", command.Jobs)
	}
}

func TestParseBatchRejectsInvalidJobs(t *testing.T) {
	_, err := parseArgs([]string{"batch", "core", "--budget", "0.80", "--jobs", "0"})
	if err == nil || !strings.Contains(err.Error(), "jobs") {
		t.Fatalf("parseArgs() error = %v, want jobs validation error", err)
	}
}

func TestParseBatchRejectsDuplicateJobs(t *testing.T) {
	_, err := parseArgs([]string{
		"batch", "core", "--budget", "0.80", "--jobs", "3", "--jobs", "3",
	})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("parseArgs() error = %v, want usage error", err)
	}
}

func TestRunListPrintsCases(t *testing.T) {
	root := testRoot(t)
	writeCase(t, root, "reviewer", "case", `{
		"id": "case",
		"agent": "reviewer",
		"live": true,
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}]
	}`)

	var stdout, stderr bytes.Buffer
	code := testApplication(root, nil).run([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(list) = %d, stderr = %s", code, stderr.String())
	}
	if got := stdout.String(); got != "reviewer/case\n" {
		t.Fatalf("list output = %q, want reviewer/case", got)
	}
}

func TestRunReplayPersistsZeroCostResult(t *testing.T) {
	root := testRoot(t)
	writeCase(t, root, "reviewer", "case", `{
		"id": "case",
		"agent": "reviewer",
		"live": true,
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}]
	}`)
	writeFile(t, filepath.Join(root, "litmus", "replays", "reviewer", "case.json"), `{"output":"APPROVE"}`)

	var stdout, stderr bytes.Buffer
	code := testApplication(root, nil).run([]string{"replay", "reviewer", "case"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(replay) = %d, stderr = %s", code, stderr.String())
	}
	run := readOnlyRun(t, root)
	if run.BudgetUSD != 0 || len(run.Cases) != 1 || run.Cases[0].CostUSD != 0 || !run.Cases[0].Passed {
		t.Fatalf("persisted replay = %#v, want zero-cost pass", run)
	}
}

func TestRunProbePersistsProviderFailure(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "reviewer")
	writeCase(t, root, "reviewer", "case", `{
		"id": "case",
		"agent": "reviewer",
		"live": true,
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}]
	}`)
	executor := &fakeExecutor{
		response: litmus.ProviderResponse{
			Output:       "partial output",
			InputTokens:  10,
			OutputTokens: 2,
			CostUSD:      0.04,
			Duration:     20 * time.Millisecond,
		},
		err: fmt.Errorf("provider unavailable"),
	}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"probe", "reviewer", "case", "--budget", "0.10"},
		&stdout,
		&stderr,
	)
	if code == 0 {
		t.Fatalf("run(probe) = %d, want failure", code)
	}
	run := readOnlyRun(t, root)
	if len(run.Cases) != 1 || run.Cases[0].Passed || run.Cases[0].ProviderError == "" ||
		run.Cases[0].Output != "partial output" || run.Cases[0].CostUSD != 0.04 {
		t.Fatalf("persisted probe failure = %#v, want provider failure result", run)
	}
}

func TestRunBatchStopsBeforeExhaustedBudget(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "reviewer")
	for _, id := range []string{"first", "second"} {
		writeCase(t, root, "reviewer", id, fmt.Sprintf(`{
			"id": %q,
			"agent": "reviewer",
			"live": true,
			"task": "review this",
			"max_budget_usd": 0.10,
			"assertions": [{"type": "contains", "value": "APPROVE"}]
		}`, id))
	}
	writeFile(t, filepath.Join(root, "litmus", "manifests", "budget.json"), `{
		"cases": [
			{"agent": "reviewer", "case": "first"},
			{"agent": "reviewer", "case": "second"}
		]
	}`)
	executor := &fakeExecutor{response: litmus.ProviderResponse{
		Output: "APPROVE", CostUSD: 0.10,
	}}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"batch", "budget", "--budget", "0.10"},
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run(batch) = %d, stderr = %s", code, stderr.String())
	}
	run := readOnlyRun(t, root)
	if len(run.Cases) != 1 || run.Cases[0].CaseID != "first" || executor.calls != 1 {
		t.Fatalf("batch run = %#v, executor calls = %d; want first case only", run, executor.calls)
	}
}

func TestRunBatchLoadsRelativeManifestPath(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "case")
	writeCase(t, root, "case", "case", `{
		"id": "case",
		"agent": "case",
		"live": true,
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}]
	}`)
	writeFile(t, filepath.Join(root, "certification", "stage3", "manifest.json"), `{
		"cases": [{"agent": "case", "case": "case"}]
	}`)
	executor := &fakeExecutor{response: litmus.ProviderResponse{
		Output: "APPROVE", CostUSD: 0.04,
	}}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"batch", "certification/stage3/manifest.json", "--budget", "0.10"},
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run(batch path) = %d, stderr = %s", code, stderr.String())
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

func TestRunBatchIncludesReplayOnlyCaseWhenRequested(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "reviewer")
	writeCase(t, root, "reviewer", "case", `{
		"id": "case",
		"agent": "reviewer",
		"live": false,
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}]
	}`)
	writeFile(t, filepath.Join(root, "litmus", "manifests", "full.json"), `{
		"cases": [{"agent": "reviewer", "case": "case"}]
	}`)
	executor := &fakeExecutor{response: litmus.ProviderResponse{
		Output: "APPROVE", CostUSD: 0.04,
	}}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"batch", "full", "--budget", "1000", "--include-replay-only"},
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run(batch) = %d, stderr = %s", code, stderr.String())
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

func TestRunBatchLimitsConcurrentProbes(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "reviewer")
	for _, id := range []string{"first", "second", "third", "fourth"} {
		writeCase(t, root, "reviewer", id, fmt.Sprintf(`{
			"id": %q,
			"agent": "reviewer",
			"live": true,
			"task": "review this",
			"max_budget_usd": 0.10,
			"assertions": [{"type": "contains", "value": "APPROVE"}]
		}`, id))
	}
	writeFile(t, filepath.Join(root, "litmus", "manifests", "parallel.json"), `{
		"cases": [
			{"agent": "reviewer", "case": "first"},
			{"agent": "reviewer", "case": "second"},
			{"agent": "reviewer", "case": "third"},
			{"agent": "reviewer", "case": "fourth"}
		]
	}`)
	executor := newBlockingExecutor()
	app := testApplication(root, executor)

	var stdout, stderr bytes.Buffer
	finished := make(chan int, 1)
	go func() {
		finished <- app.run(
			[]string{"batch", "parallel", "--budget", "0.40"},
			&stdout,
			&stderr,
		)
	}()

	for count := 0; count < defaultBatchJobs; count++ {
		select {
		case <-executor.started:
		case <-time.After(200 * time.Millisecond):
			close(executor.release)
			<-finished
			t.Fatalf("only %d concurrent probes started, want %d", count, defaultBatchJobs)
		}
	}
	close(executor.release)

	select {
	case code := <-finished:
		if code != 0 {
			t.Fatalf("run(batch) = %d, stderr = %s", code, stderr.String())
		}
	case <-time.After(time.Second):
		t.Fatal("batch did not finish")
	}
	if peak := executor.maxActive(); peak != defaultBatchJobs {
		t.Fatalf("maximum active probes = %d, want %d", peak, defaultBatchJobs)
	}
	run := readOnlyRun(t, root)
	if len(run.Cases) != 4 {
		t.Fatalf("persisted cases = %d, want 4", len(run.Cases))
	}
}

func TestRunBatchHonorsExplicitJobLimit(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "reviewer")
	for _, id := range []string{"first", "second"} {
		writeCase(t, root, "reviewer", id, fmt.Sprintf(`{
			"id": %q,
			"agent": "reviewer",
			"live": true,
			"task": "review this",
			"max_budget_usd": 0.10,
			"assertions": [{"type": "contains", "value": "APPROVE"}]
		}`, id))
	}
	writeFile(t, filepath.Join(root, "litmus", "manifests", "serial.json"), `{
		"cases": [
			{"agent": "reviewer", "case": "first"},
			{"agent": "reviewer", "case": "second"}
		]
	}`)
	executor := newBlockingExecutor()
	app := testApplication(root, executor)

	var stdout, stderr bytes.Buffer
	finished := make(chan int, 1)
	go func() {
		finished <- app.run(
			[]string{"batch", "serial", "--budget", "0.20", "--jobs", "1"},
			&stdout,
			&stderr,
		)
	}()

	select {
	case <-executor.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("first probe did not start")
	}
	select {
	case <-executor.started:
		close(executor.release)
		<-finished
		t.Fatal("second probe started before the first completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(executor.release)

	select {
	case code := <-finished:
		if code != 0 {
			t.Fatalf("run(batch) = %d, stderr = %s", code, stderr.String())
		}
	case <-time.After(time.Second):
		t.Fatal("batch did not finish")
	}
	if peak := executor.maxActive(); peak != 1 {
		t.Fatalf("maximum active probes = %d, want 1", peak)
	}
}

func TestRunBatchPersistsProviderFailures(t *testing.T) {
	root := testRoot(t)
	writeAgent(t, root, "reviewer")
	for _, id := range []string{"first", "second", "third"} {
		writeCase(t, root, "reviewer", id, fmt.Sprintf(`{
			"id": %q,
			"agent": "reviewer",
			"live": true,
			"task": "review this",
			"max_budget_usd": 0.10,
			"assertions": [{"type": "contains", "value": "APPROVE"}]
		}`, id))
	}
	writeFile(t, filepath.Join(root, "litmus", "manifests", "provider-errors.json"), `{
		"cases": [
			{"agent": "reviewer", "case": "first"},
			{"agent": "reviewer", "case": "second"},
			{"agent": "reviewer", "case": "third"}
		]
	}`)
	executor := &failingExecutor{}

	var stdout, stderr bytes.Buffer
	code := testApplication(root, executor).run(
		[]string{"batch", "provider-errors", "--budget", "0.30", "--jobs", "2"},
		&stdout,
		&stderr,
	)
	if code == 0 {
		t.Fatalf("run(batch) = %d, want failure", code)
	}
	if calls := executor.callCount(); calls != 3 {
		t.Fatalf("executor calls = %d, want 3", calls)
	}
	run := readOnlyRun(t, root)
	if len(run.Cases) != 3 {
		t.Fatalf("persisted cases = %d, want 3", len(run.Cases))
	}
	for _, result := range run.Cases {
		if result.Passed || result.ProviderError == "" || result.CostUSD != 0.04 {
			t.Fatalf("persisted provider failure = %#v", result)
		}
	}
}

func TestReserveBatchCaseAccountsForOutstandingReservations(t *testing.T) {
	reservation, err := reserveBatchCase(0.10, 0.15, 0, 0.10)
	if err != nil {
		t.Fatal(err)
	}
	if reservation != 0.05 {
		t.Fatalf("reservation = %.2f, want 0.05", reservation)
	}
}

func TestRunComparePrintsJSONAndMarkdown(t *testing.T) {
	root := testRoot(t)
	baseline := writeRun(t, root, "baseline", true, 0.01)
	current := writeRun(t, root, "current", false, 0.02)

	var stdout, stderr bytes.Buffer
	code := testApplication(root, nil).run([]string{"compare", baseline, current}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(compare) = %d, stderr = %s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"\"status\": \"regressed\"",
		"## Litmus Comparison",
		"| reviewer | case | regressed | 0.01 |",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("comparison output missing %q:\n%s", want, output)
		}
	}
}

type fakeExecutor struct {
	response litmus.ProviderResponse
	err      error
	calls    int
}

type workflowCLIExecutor struct {
	calls     int
	firstCost float64
}

func (executor *workflowCLIExecutor) Execute(
	_ context.Context,
	request litmus.ProviderRequest,
) (litmus.ProviderResponse, error) {
	executor.calls++
	output := ""
	switch {
	case request.Agent == "builder":
		output = `{"status":"done","summary":"built","files":["x.go"],"verification":"go test"}`
	case request.Agent == "validator" && strings.Contains(request.Task, "Step: review-challenge"):
		output = `{"decision":"PASS","reason":"verified","issues":[]}`
	case request.Agent == "validator":
		output = `{"decision":"PASS","reason":"verified","issues":[]}`
	case request.Agent == "code-reviewer":
		output = `{"decision":"APPROVE","reason":"sound","findings":[]}`
	case request.Agent == "documenter":
		if err := os.MkdirAll(filepath.Join(request.Workspace, "docs"), 0o755); err != nil {
			return litmus.ProviderResponse{}, err
		}
		if err := os.WriteFile(
			filepath.Join(request.Workspace, "docs", "result.md"),
			[]byte("# Generated document\n"),
			0o644,
		); err != nil {
			return litmus.ProviderResponse{}, err
		}
		output = `{"status":"done","path":"docs/result.md","summary":"documented"}`
	default:
		return litmus.ProviderResponse{}, fmt.Errorf("unexpected workflow agent %q", request.Agent)
	}
	cost := 0.01
	if executor.calls == 1 && executor.firstCost != 0 {
		cost = executor.firstCost
	}
	model := "claude-test"
	return litmus.ProviderResponse{
		Output:             output,
		ProviderModels:     []string{model},
		ProviderResponseID: fmt.Sprintf("response-%d", executor.calls),
		ProviderSessionID:  fmt.Sprintf("session-%d", executor.calls),
		ProviderModelUsage: map[string]map[string]json.RawMessage{
			model: {
				"inputTokens":  json.RawMessage("10"),
				"outputTokens": json.RawMessage("5"),
				"costUSD":      json.RawMessage(fmt.Sprintf("%.8f", cost)),
			},
		},
		InputTokens:  10,
		OutputTokens: 5,
		CostUSD:      cost,
		Duration:     time.Millisecond,
		TelemetryPresence: litmus.TelemetryPresence{
			ModelUsage:         true,
			InputTokens:        true,
			OutputTokens:       true,
			CostUSD:            true,
			DurationMS:         true,
			ProviderResponseID: true,
			ProviderSessionID:  true,
		},
	}, nil
}

type gradeExecutor struct {
	output string
	calls  int
}

func (f *gradeExecutor) Execute(_ context.Context, request litmus.ProviderRequest) (litmus.ProviderResponse, error) {
	f.calls++
	if request.Agent != "model-grader" {
		return litmus.ProviderResponse{}, fmt.Errorf("unexpected grader agent %q", request.Agent)
	}
	return litmus.ProviderResponse{
		Output:       f.output,
		InputTokens:  10,
		OutputTokens: 8,
		CostUSD:      0.01,
	}, nil
}

func (f *fakeExecutor) Execute(_ context.Context, _ litmus.ProviderRequest) (litmus.ProviderResponse, error) {
	f.calls++
	return f.response, f.err
}

type failingExecutor struct {
	mu    sync.Mutex
	calls int
}

func (f *failingExecutor) Execute(_ context.Context, _ litmus.ProviderRequest) (litmus.ProviderResponse, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return litmus.ProviderResponse{
		Output:  "APPROVE",
		CostUSD: 0.04,
	}, fmt.Errorf("provider unavailable")
}

func (f *failingExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type blockingExecutor struct {
	mu      sync.Mutex
	active  int
	peak    int
	started chan struct{}
	release chan struct{}
}

func newBlockingExecutor() *blockingExecutor {
	return &blockingExecutor{
		started: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
}

func (f *blockingExecutor) Execute(_ context.Context, _ litmus.ProviderRequest) (litmus.ProviderResponse, error) {
	f.mu.Lock()
	f.active++
	if f.active > f.peak {
		f.peak = f.active
	}
	f.mu.Unlock()

	f.started <- struct{}{}
	<-f.release

	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	return litmus.ProviderResponse{Output: "APPROVE", CostUSD: 0.04}, nil
}

func (f *blockingExecutor) maxActive() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peak
}

func testApplication(root string, executor litmus.Executor) application {
	return application{
		root: root,
		runner: litmus.Runner{
			Root:     root,
			Executor: executor,
		},
		now: func() time.Time {
			return time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC)
		},
		revision: func(string) string { return "test" },
	}
}

func testRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func measuredWorkflowCLIRoot(t *testing.T, caseIDs []string) string {
	t.Helper()
	root := testRoot(t)
	for _, agent := range []string{"builder", "validator", "code-reviewer", "documenter"} {
		writeAgent(t, root, agent)
	}
	writeFile(t, filepath.Join(root, "scripts", "install.mjs"), `
import { mkdirSync } from 'node:fs';
import path from 'node:path';
const target = process.argv.at(-1);
mkdirSync(path.join(target, '.claude', 'agents'), { recursive: true });
mkdirSync(path.join(target, '.claude', 'skills'), { recursive: true });
`)
	writeFile(t, filepath.Join(root, "litmus", "workflows", "stage4-team-lead.json"), `{
  "schema_version":"stage4.measured-workflow.v1",
  "workflow_id":"team-lead-implementation-v1",
  "max_attempts":3,
  "steps":[
    {"id":"builder","agent":"builder","budget_usd":0.75,"allow_workspace_changes":true,"json_schema":{}},
    {"id":"validator","agent":"validator","budget_usd":0.45,"allow_workspace_changes":false,"json_schema":{}},
    {"id":"code-reviewer","agent":"code-reviewer","budget_usd":0.50,"allow_workspace_changes":false,"json_schema":{}},
    {"id":"review-challenge","agent":"validator","budget_usd":0.40,"allow_workspace_changes":false,"json_schema":{}},
    {"id":"documenter","agent":"documenter","budget_usd":0.40,"allow_workspace_changes":true,"json_schema":{}}
  ]
}`)
	manifestCases := make([]string, 0, len(caseIDs))
	for _, id := range caseIDs {
		writeCase(t, root, "team-lead", id, fmt.Sprintf(`{
  "id":%q,
  "agent":"team-lead",
  "task":"Implement the requested change.",
  "max_budget_usd":2.50,
  "live":true,
  "workflow":true,
  "assertions":[
    {"type":"contains","value":"builder: done"},
    {"type":"contains","value":"validator: PASS"},
    {"type":"contains","value":"code-reviewer: APPROVE"}
  ]
}`, id))
		manifestCases = append(
			manifestCases,
			fmt.Sprintf(`{"agent":"team-lead","case":%q}`, id),
		)
	}
	writeFile(
		t,
		filepath.Join(root, "litmus", "manifests", "measured.json"),
		`{"cases":[`+strings.Join(manifestCases, ",")+`]}`,
	)
	return root
}

func writeCase(t *testing.T, root, agent, id, contents string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "litmus", "cases", agent, id+".json"), contents)
}

func writeAgent(t *testing.T, root, agent string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "agents", agent, "manifest.json"),
		fmt.Sprintf(`{"name":%q,"profile":"sonnet"}`, agent))
	writeFile(t, filepath.Join(root, "agents", agent, "claude.md"),
		fmt.Sprintf(`---
name: %s
description: Reviewer
model: sonnet
effort: medium
---
# Reviewer`, agent))
}

func writeRun(t *testing.T, root, id string, passed bool, cost float64) string {
	t.Helper()
	directory, err := litmus.WriteRun(root, litmus.Run{
		ID:        "20260715T143022-" + id,
		Timestamp: time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC),
		Revision:  id,
		BudgetUSD: 0.10,
		Cases: []litmus.CaseResult{{
			Agent:   "reviewer",
			CaseID:  "case",
			Passed:  passed,
			CostUSD: cost,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func readOnlyRun(t *testing.T, root string) litmus.Run {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "litmus", "results"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("result directories = %d, want 1", len(entries))
	}
	run, err := litmus.ReadRun(filepath.Join(root, "litmus", "results", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
