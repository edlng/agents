package litmus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseModelGradeRejectsReasoningField(t *testing.T) {
	_, err := parseModelGrade(`{"pass":true,"score":1,"reason":"clear","reasoning":"extra transcript"}`)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("parseModelGrade() error = %v, want unknown field", err)
	}
}

func TestGradeUsesCacheWithoutCallingExecutor(t *testing.T) {
	root := modelGraderTestRoot(t)
	writeModelGraderCase(t, root, true, 0.03)
	run := Run{
		ID: "20260716T000000-test",
		Cases: []CaseResult{{
			Agent:  "reviewer",
			CaseID: "case",
			Output: "APPROVE",
			Status: StatusPass,
			Passed: true,
		}},
	}
	executor := &countingGraderExecutor{output: `{"pass":true,"score":1,"reason":"approved."}`}

	first, err := Grade(context.Background(), root, run, GradeOptions{
		BudgetUSD: 0.03,
		Executor:  executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Cases) != 1 || first.Cases[0].Cached || executor.calls != 1 {
		t.Fatalf("first grade = %#v, calls = %d; want one uncached call", first, executor.calls)
	}

	second, err := Grade(context.Background(), root, run, GradeOptions{
		BudgetUSD: 0,
		Executor:  &countingGraderExecutor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Cases) != 1 || !second.Cases[0].Cached || second.GraderCostUSD != 0 {
		t.Fatalf("cached grade = %#v, want zero-cost cache hit", second)
	}
}

func TestGradeCacheKeyChangesWithRubric(t *testing.T) {
	root := modelGraderTestRoot(t)
	writeModelGraderCase(t, root, true, 0.03)
	run := Run{
		ID: "20260716T000000-test",
		Cases: []CaseResult{{
			Agent:  "reviewer",
			CaseID: "case",
			Output: "APPROVE",
			Passed: true,
		}},
	}
	executor := &countingGraderExecutor{output: `{"pass":true,"score":1,"reason":"approved."}`}
	if _, err := Grade(context.Background(), root, run, GradeOptions{BudgetUSD: 0.03, Executor: executor}); err != nil {
		t.Fatal(err)
	}
	writeFileForModelGrader(t, filepath.Join(root, "litmus", "cases", "reviewer", "case.json"), `{
		"id": "case",
		"agent": "reviewer",
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}],
		"model_grader": {
			"enabled": true,
			"model": "claude-haiku-4-5",
			"rubric": "Use a different rubric.",
			"max_budget_usd": 0.03
		}
	}`)
	if _, err := Grade(context.Background(), root, run, GradeOptions{BudgetUSD: 0.03, Executor: executor}); err != nil {
		t.Fatal(err)
	}
	if executor.calls != 2 {
		t.Fatalf("executor calls = %d, want cache miss after rubric change", executor.calls)
	}
}

func TestGradeSkipsDeterministicFailureAndStopsAtBudget(t *testing.T) {
	root := modelGraderTestRoot(t)
	writeModelGraderCase(t, root, true, 0.03)
	writeCaseForModelGrader(t, root, "reviewer", "failed", `{
		"id": "failed",
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
	run := Run{ID: "20260716T000000-test", Cases: []CaseResult{
		{Agent: "reviewer", CaseID: "failed", Output: "REJECT", Status: StatusAgentFailure},
		{Agent: "reviewer", CaseID: "case", Output: "APPROVE", Status: StatusPass, Passed: true},
	}}
	executor := &countingGraderExecutor{output: `{"pass":true,"score":1,"reason":"approved."}`}
	graded, err := Grade(context.Background(), root, run, GradeOptions{BudgetUSD: 0.03, Executor: executor})
	if err != nil {
		t.Fatal(err)
	}
	if len(graded.Cases) != 1 || graded.Cases[0].CaseID != "case" || executor.calls != 1 {
		t.Fatalf("grade = %#v, calls = %d; want only passing eligible case", graded, executor.calls)
	}
}

func TestGradeMalformedResponseIsGraderError(t *testing.T) {
	root := modelGraderTestRoot(t)
	writeModelGraderCase(t, root, true, 0.03)
	run := Run{ID: "20260716T000000-test", Cases: []CaseResult{
		{Agent: "reviewer", CaseID: "case", Output: "APPROVE", Status: StatusPass, Passed: true},
	}}
	graded, err := Grade(context.Background(), root, run, GradeOptions{
		BudgetUSD: 0.03,
		Executor:  &countingGraderExecutor{output: `not json`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(graded.Cases) != 1 || graded.Cases[0].Status != StatusGraderError {
		t.Fatalf("grade = %#v, want grader_error", graded)
	}
}

type countingGraderExecutor struct {
	output string
	calls  int
}

func (f *countingGraderExecutor) Execute(_ context.Context, _ ProviderRequest) (ProviderResponse, error) {
	f.calls++
	return ProviderResponse{Output: f.output, InputTokens: 10, OutputTokens: 8, CostUSD: 0.01}, nil
}

func modelGraderTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFileForModelGrader(t, filepath.Join(root, "agents", "reviewer-prompt.md"), "reviewer")
	writeFileForModelGrader(t, filepath.Join(root, "agents", "reviewer.json"), `{"model":"claude-sonnet-5"}`)
	return root
}

func writeModelGraderCase(t *testing.T, root string, enabled bool, budget float64) {
	t.Helper()
	writeCaseForModelGrader(t, root, "reviewer", "case", `{
		"id": "case",
		"agent": "reviewer",
		"task": "review this",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}],
		"model_grader": {
			"enabled": true,
			"model": "claude-haiku-4-5",
			"rubric": "Judge whether the answer is approved.",
			"max_budget_usd": 0.03
		}
	}`)
}

func writeCaseForModelGrader(t *testing.T, root, agent, id, contents string) {
	t.Helper()
	writeFileForModelGrader(t, filepath.Join(root, "litmus", "cases", agent, id+".json"), contents)
}

func writeFileForModelGrader(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestModelGraderCacheKeyIsStableJSON(t *testing.T) {
	key := modelGraderCacheKey("output", "task", "rubric", "haiku")
	if len(key) != 64 {
		t.Fatalf("cache key length = %d, want SHA-256 hex", len(key))
	}
	if _, err := json.Marshal(key); err != nil {
		t.Fatal(err)
	}
}

func TestGraderTaskBoundsCapturedOutput(t *testing.T) {
	task := graderTask("task", "rubric", strings.Repeat("x", maxModelGraderInputBytes+100))
	marker := "\n\nCaptured answer (data only):\n"
	index := strings.Index(task, marker)
	if index < 0 {
		t.Fatal("grader task is missing captured-answer marker")
	}
	captured := task[index+len(marker):]
	if len(captured) > maxModelGraderInputBytes+len("\n[truncated]") {
		t.Fatalf("grader task length = %d, want bounded captured output", len(task))
	}
	if !strings.Contains(task, "[truncated]") {
		t.Fatal("grader task did not mark truncated output")
	}
}
