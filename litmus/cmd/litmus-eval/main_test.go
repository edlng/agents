package main

import (
	"bytes"
	"context"
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
