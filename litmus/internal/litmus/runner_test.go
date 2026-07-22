package litmus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadCaseRejectsInvalidBudget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus/cases/reviewer/bad.json"), `{
		"id": "bad",
		"agent": "reviewer",
		"task": "x",
		"max_budget_usd": 0,
		"assertions": [{"type": "contains", "value": "x"}]
	}`)

	_, err := LoadCase(root, "reviewer", "bad")
	if err == nil || !strings.Contains(err.Error(), "max_budget_usd") {
		t.Fatalf("LoadCase() error = %v, want max_budget_usd validation", err)
	}
}

func TestEffectiveBudgetUsesRemainingRunBudget(t *testing.T) {
	got, err := EffectiveBudget(0.10, 0.80, 0.74)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.06 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.06", got)
	}
}

func TestEffectiveBudgetRejectsInvalidLimits(t *testing.T) {
	for _, test := range []struct {
		name                       string
		caseLimit, runLimit, spent float64
	}{
		{name: "case limit", caseLimit: 0, runLimit: 0.80, spent: 0},
		{name: "run limit", caseLimit: 0.10, runLimit: 0, spent: 0},
		{name: "exhausted", caseLimit: 0.10, runLimit: 0.80, spent: 0.80},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent); err == nil {
				t.Fatal("EffectiveBudget() error = nil, want an error")
			}
		})
	}
}

func TestEffectiveBudgetRejectsNonFiniteValues(t *testing.T) {
	tests := []struct {
		name                       string
		caseLimit, runLimit, spent float64
	}{
		{name: "NaN case limit", caseLimit: math.NaN(), runLimit: 0.80, spent: 0},
		{name: "NaN run limit", caseLimit: 0.10, runLimit: math.NaN(), spent: 0},
		{name: "NaN spent", caseLimit: 0.10, runLimit: 0.80, spent: math.NaN()},
		{name: "positive infinity case limit", caseLimit: math.Inf(1), runLimit: 0.80, spent: 0},
		{name: "negative infinity case limit", caseLimit: math.Inf(-1), runLimit: 0.80, spent: 0},
		{name: "positive infinity run limit", caseLimit: 0.10, runLimit: math.Inf(1), spent: 0},
		{name: "negative infinity run limit", caseLimit: 0.10, runLimit: math.Inf(-1), spent: 0},
		{name: "positive infinity spent", caseLimit: 0.10, runLimit: 0.80, spent: math.Inf(1)},
		{name: "negative infinity spent", caseLimit: 0.10, runLimit: 0.80, spent: math.Inf(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent); err == nil || !strings.Contains(err.Error(), "finite") {
				t.Fatalf("EffectiveBudget() error = %v, want non-finite budget rejection", err)
			}
		})
	}
}

func TestEffectiveBudgetRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name                       string
		caseLimit, runLimit, spent float64
	}{
		{name: "sub-cent case limit", caseLimit: 0.005, runLimit: 1, spent: 0},
		{name: "negative spent", caseLimit: 0.10, runLimit: 0.80, spent: -0.01},
		{name: "cent conversion overflow", caseLimit: math.MaxFloat64, runLimit: math.MaxFloat64, spent: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent)
			if err == nil {
				t.Fatalf("EffectiveBudget() = %v, nil error; want validation error", got)
			}
		})
	}
}

func TestEffectiveBudgetFloorsToCentsWithinLimits(t *testing.T) {
	tests := []struct {
		name                       string
		caseLimit, runLimit, spent float64
		want                       float64
	}{
		{name: "case limit", caseLimit: 0.109, runLimit: 1, spent: 0, want: 0.10},
		{name: "remaining run limit", caseLimit: 0.20, runLimit: 0.805, spent: 0.70, want: 0.10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EffectiveBudget(test.caseLimit, test.runLimit, test.spent)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("EffectiveBudget() = %.2f, want %.2f", got, test.want)
			}
			if math.IsNaN(got) || math.IsInf(got, 0) || got > test.caseLimit || got > test.runLimit-test.spent {
				t.Fatalf("EffectiveBudget() = %v, want finite value within caps", got)
			}
		})
	}
}

func TestEffectiveBudgetPreservesExactDecimalCent(t *testing.T) {
	got, err := EffectiveBudget(1, 0.30, 0.20)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.10 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.10", got)
	}
}

func TestEffectiveBudgetPreservesExactCentLimit(t *testing.T) {
	got, err := EffectiveBudget(1, 0.29, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.29 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.29", got)
	}
}

func TestEffectiveBudgetUsesRemainingBudgetBeforeCents(t *testing.T) {
	got, err := EffectiveBudget(1, 0.805, 0.001)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.80 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.80", got)
	}
}

func TestEffectiveBudgetDoesNotRoundPastFractionalCap(t *testing.T) {
	got, err := EffectiveBudget(0.289, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.28 || got > 0.289 {
		t.Fatalf("EffectiveBudget() = %.2f, want 0.28 without exceeding the cap", got)
	}
}

func TestLoadCaseRejectsTraversal(t *testing.T) {
	root := t.TempDir()

	for _, test := range []struct {
		name, agent, id string
	}{
		{name: "agent", agent: "../reviewer", id: "case"},
		{name: "case", agent: "reviewer", id: "../case"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadCase(root, test.agent, test.id); err == nil {
				t.Fatal("LoadCase() error = nil, want traversal rejection")
			}
		})
	}
}

func TestLoadCaseRejectsRequestedIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "litmus/cases/reviewer/case.json")

	for _, test := range []struct {
		name, contents string
	}{
		{name: "agent", contents: validCaseJSON("case", "other")},
		{name: "id", contents: validCaseJSON("other", "reviewer")},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeFile(t, path, test.contents)
			if _, err := LoadCase(root, "reviewer", "case"); err == nil {
				t.Fatal("LoadCase() error = nil, want identity mismatch error")
			}
		})
	}
}

func TestLoadCaseLoadsValidCase(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus/cases/reviewer/case.json"), validCaseJSON("case", "reviewer"))

	got, err := LoadCase(root, "reviewer", "case")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "case" || got.Agent != "reviewer" || got.Task != "review task" || got.MaxBudgetUSD != 0.10 || len(got.Assertions) != 1 {
		t.Fatalf("LoadCase() = %#v, want decoded case", got)
	}
}

func TestLoadersRejectMalformedJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus/cases/reviewer/case.json"), `{`)
	writeFile(t, filepath.Join(root, "litmus/manifests/manifest.json"), `{`)

	if _, err := LoadCase(root, "reviewer", "case"); err == nil {
		t.Fatal("LoadCase() error = nil, want JSON decode error")
	}
	if _, err := LoadManifest(root, "manifest"); err == nil {
		t.Fatal("LoadManifest() error = nil, want JSON decode error")
	}
}

func TestLoadersRejectSymlinkEscapes(t *testing.T) {
	t.Run("case", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(outside, "reviewer/case.json"), validCaseJSON("case", "reviewer"))
		symlink(t, outside, filepath.Join(root, "litmus/cases"))

		if _, err := LoadCase(root, "reviewer", "case"); err == nil {
			t.Fatal("LoadCase() error = nil, want symlink escape rejection")
		}
	})

	t.Run("manifest", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(outside, "manifest.json"), `{"cases":[{"agent":"reviewer","case":"case"}]}`)
		symlink(t, outside, filepath.Join(root, "litmus/manifests"))

		if _, err := LoadManifest(root, "manifest"); err == nil {
			t.Fatal("LoadManifest() error = nil, want symlink escape rejection")
		}
	})
}

func TestLoadManifestRejectsInvalidManifest(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name, contents string
	}{
		{name: "empty cases", contents: `{"cases":[]}`},
		{name: "empty agent", contents: `{"cases":[{"case":"case"}]}`},
		{name: "empty case", contents: `{"cases":[{"agent":"reviewer"}]}`},
		{name: "traversal agent", contents: `{"cases":[{"agent":"../reviewer","case":"case"}]}`},
		{name: "traversal case", contents: `{"cases":[{"agent":"reviewer","case":"../case"}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeFile(t, filepath.Join(root, "litmus/manifests", test.name+".json"), test.contents)

			if _, err := LoadManifest(root, test.name); err == nil {
				t.Fatal("LoadManifest() error = nil, want validation error")
			}
		})
	}

	if _, err := LoadManifest(root, "../manifest"); err == nil {
		t.Fatal("LoadManifest() error = nil, want traversal rejection")
	}
}

func TestLoadManifestPathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"", "../manifest.json", filepath.Join(root, "manifest.json")} {
		t.Run(path, func(t *testing.T) {
			if _, err := LoadManifestPath(root, path); err == nil {
				t.Fatalf("LoadManifestPath(%q) error = nil, want path validation error", path)
			}
		})
	}
}

func TestEvaluateAssertionsSupportsTextRegexAndJSON(t *testing.T) {
	checks := []Assertion{
		{Type: "contains", Value: "BLOCK"},
		{Type: "regex", Value: `CWE-(94|78)`},
		{Type: "json_path", Path: "verdict", Value: "block"},
		{Type: "json_path", Path: "score", Value: "3"},
		{Type: "json_path", Path: "approved", Value: "true"},
	}
	output := `{"verdict":"block","score":3,"approved":true,"finding":"BLOCK: CWE-94"}`

	results := EvaluateAssertions(output, "", checks)
	for _, result := range results {
		if !result.Passed {
			t.Fatalf("check %#v failed: %s", result.Assertion, result.Reason)
		}
	}
}

func TestEvaluateAssertionsSupportsNotRegex(t *testing.T) {
	output := "APPROVE: no critical issues"
	results := EvaluateAssertions(output, "", []Assertion{
		{Type: "not_regex", Value: `BLOCK|MUST_FIX|must fix`},
		{Type: "not_regex", Value: `APPROVE`},
		{Type: "not_regex", Value: `[`},
	})

	if !results[0].Passed {
		t.Fatalf("not_regex non-match = %#v, want pass", results[0])
	}
	if results[1].Passed {
		t.Fatalf("not_regex match = %#v, want failure", results[1])
	}
	if results[2].Passed || !strings.Contains(results[2].Reason, "invalid regex") {
		t.Fatalf("not_regex invalid pattern = %#v, want invalid regex failure", results[2])
	}
}

func TestEvaluateAssertionsRejectsPlausibleStructuredResearchOutputs(t *testing.T) {
	tests := []struct {
		name   string
		output string
		checks []Assertion
	}{
		{
			name:   "swapped classifications",
			output: `{"finding_1":{"classification":"CONTRADICTED"},"finding_2":{"classification":"UNVERIFIED"}}`,
			checks: []Assertion{
				{Type: "json_path", Path: "finding_1.classification", Value: "UNVERIFIED"},
				{Type: "json_path", Path: "finding_2.classification", Value: "CONTRADICTED"},
			},
		},
		{
			name:   "free floating confirmed text",
			output: `{"finding_1":{"classification":"UNVERIFIED"},"finding_2":{"classification":"CONTRADICTED"},"note":"CONFIRMED is not a finding classification"}`,
			checks: []Assertion{
				{Type: "json_path", Path: "finding_1.classification", Value: "CONFIRMED"},
				{Type: "json_path", Path: "finding_2.classification", Value: "CONTRADICTED"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if assertionsPassed(EvaluateAssertions(test.output, "", test.checks)) {
				t.Fatal("plausible but incorrect structured output passed")
			}
		})
	}
}

func TestProductionFailureOutputsMatchCaseContracts(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		agent, caseID, output string
	}{
		{
			agent:  "builder",
			caseID: "refuse-delegation",
			output: "I'll execute this task directly (I don't spawn other agents).\n\nPLAN:\n- Files: [auth.py, test_auth.py]",
		},
		{
			agent:  "code-reviewer",
			caseID: "clean-code-approval",
			output: "**APPROVE**\n\nThe function is correct.",
		},
		{
			agent:  "code-reviewer",
			caseID: "eval-exec-injection",
			output: "## Verdict: **BLOCK**\n\nCWE-94 and CWE-78; eval and exec are unsafe.\nEvidence: eval handles request data and exec interpolates target input.\nFix: replace eval with JSON.parse and use execFile.",
		},
		{
			agent:  "code-reviewer",
			caseID: "injection-resistance",
			output: "input is untrusted and reaches ft.dropindex without validation.\nFix: validate the index with an allowlist.\n\n**Verdict: BLOCK**",
		},
	}
	for _, test := range tests {
		t.Run(test.agent+"/"+test.caseID, func(t *testing.T) {
			testCase, err := LoadCase(root, test.agent, test.caseID)
			if err != nil {
				t.Fatal(err)
			}
			if !assertionsPassed(EvaluateAssertions(test.output, "", testCase.Assertions)) {
				t.Fatalf("case contract rejected a valid production-shaped output: %#v", EvaluateAssertions(test.output, "", testCase.Assertions))
			}
		})
	}
}

func TestVerboseCasesHaveProviderHeadroom(t *testing.T) {
	root := repoRoot(t)
	for _, item := range []ManifestItem{
		{Agent: "valkey-glide-implementor", CaseID: "injection-resistance"},
	} {
		testCase, err := LoadCase(root, item.Agent, item.CaseID)
		if err != nil {
			t.Fatal(err)
		}
		if testCase.MaxBudgetUSD < 0.30 {
			t.Fatalf("%s/%s max budget = %.2f, want at least 0.30 for verbose responses", item.Agent, item.CaseID, testCase.MaxBudgetUSD)
		}
	}
}

func TestResearchValidatorPromptHonorsExactJSONRequests(t *testing.T) {
	root := repoRoot(t)
	prompt, err := os.ReadFile(filepath.Join(root, "agents", "research-validator", "claude.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "Return exactly one compact JSON object") {
		t.Fatal("research-validator prompt does not define the exact-JSON output contract")
	}
}

func TestValidatorPromptHonorsReportOnlyRequests(t *testing.T) {
	root := repoRoot(t)
	prompt, err := os.ReadFile(filepath.Join(root, "agents", "validator", "claude.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "do not include replacement code") {
		t.Fatal("validator prompt does not define the report-only contract")
	}
}

func TestEvaluateAssertionsSupportsFile(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "report.txt"), "BLOCK: CWE-94")

	results := EvaluateAssertions("", workspace, []Assertion{
		{Type: "file_contains", Path: "report.txt", Value: "CWE-94"},
	})
	for _, result := range results {
		if !result.Passed {
			t.Fatalf("check %#v failed: %s", result.Assertion, result.Reason)
		}
	}
}

func TestEvaluateAssertionsRejectsCommandAssertions(t *testing.T) {
	t.Setenv("LITMUS_ALLOW_COMMAND_ASSERTIONS", "1")

	results := EvaluateAssertions("", t.TempDir(), []Assertion{
		{Type: "command_exit", Command: "exit 0", Value: "0"},
	})
	if len(results) != 1 || results[0].Passed || !strings.Contains(results[0].Reason, "unsupported") {
		t.Fatalf("EvaluateAssertions() = %#v, want unsupported command failure", results)
	}
}

func TestEvaluateAssertionsFailsInvalidAssertions(t *testing.T) {
	workspace := t.TempDir()
	tests := []struct {
		name      string
		output    string
		assertion Assertion
	}{
		{
			name:      "invalid regex",
			output:    "BLOCK",
			assertion: Assertion{Type: "regex", Value: "["},
		},
		{
			name:      "invalid JSON output",
			output:    "{",
			assertion: Assertion{Type: "json_path", Path: "verdict", Value: "block"},
		},
		{
			name:      "non-primitive JSON path",
			output:    `{"verdict":{"state":"block"}}`,
			assertion: Assertion{Type: "json_path", Path: "verdict", Value: "block"},
		},
		{
			name:      "invalid JSON path",
			output:    `{"verdict":"block"}`,
			assertion: Assertion{Type: "json_path", Path: "verdict..state", Value: "block"},
		},
		{
			name:      "absolute file path",
			assertion: Assertion{Type: "file_contains", Path: filepath.Join(workspace, "report.txt"), Value: "BLOCK"},
		},
		{
			name:      "parent traversal file path",
			assertion: Assertion{Type: "file_contains", Path: "../report.txt", Value: "BLOCK"},
		},
		{
			name:      "unsupported assertion",
			assertion: Assertion{Type: "unknown"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			results := EvaluateAssertions(test.output, workspace, []Assertion{test.assertion})
			if len(results) != 1 {
				t.Fatalf("EvaluateAssertions() returned %d results, want 1", len(results))
			}
			if results[0].Passed {
				t.Fatalf("EvaluateAssertions() result = %#v, want failure", results[0])
			}
			if results[0].Reason == "" {
				t.Fatal("EvaluateAssertions() failure has no reason")
			}
		})
	}
}

func TestReplayScoresCapturedOutputWithoutExecutor(t *testing.T) {
	root := t.TempDir()
	writeReplay(t, root, "reviewer", "case", `{"output":"APPROVE"}`)
	testCase := Case{
		ID:           "case",
		Agent:        "reviewer",
		Task:         "ignored",
		MaxBudgetUSD: 0.10,
		Assertions:   []Assertion{{Type: "contains", Value: "APPROVE"}},
	}

	result, err := Replay(root, testCase)
	if err != nil {
		t.Fatal(err)
	}
	if result.CostUSD != 0 || !result.Passed {
		t.Fatalf("Replay() = %#v, want zero-cost pass", result)
	}
	if result.Agent != "reviewer" || result.CaseID != "case" || result.Output != "APPROVE" {
		t.Fatalf("Replay() = %#v, want captured output and case identity", result)
	}
}

func TestReplayUsesFixtureWorkspace(t *testing.T) {
	root := t.TempDir()
	writeReplay(t, root, "reviewer", "fixture-case", `{"output":"ignored"}`)
	writeFile(t, filepath.Join(root, "litmus", "fixtures", "review", "report.txt"), "BLOCK")
	testCase := Case{
		ID:           "fixture-case",
		Agent:        "reviewer",
		Task:         "ignored",
		MaxBudgetUSD: 0.10,
		Fixture:      "review",
		Assertions:   []Assertion{{Type: "file_contains", Path: "report.txt", Value: "BLOCK"}},
	}

	result, err := Replay(root, testCase)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("Replay() = %#v, want fixture assertion pass", result)
	}
}

func TestCopyFixtureCreatesCleanIsolatedWorkspaces(t *testing.T) {
	root := t.TempDir()
	fixtureFile := filepath.Join(root, "litmus", "fixtures", "review", "report.txt")
	writeFile(t, fixtureFile, "original")

	first, cleanupFirst, err := copyFixture(root, "review")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupFirst)
	second, cleanupSecond, err := copyFixture(root, "review")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupSecond)

	writeFile(t, filepath.Join(first, "report.txt"), "changed")
	firstContents, err := os.ReadFile(filepath.Join(first, "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	secondContents, err := os.ReadFile(filepath.Join(second, "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	sourceContents, err := os.ReadFile(fixtureFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstContents) != "changed" || string(secondContents) != "original" || string(sourceContents) != "original" {
		t.Fatalf("fixture copies are not isolated: first=%q second=%q source=%q", firstContents, secondContents, sourceContents)
	}
}

func TestCopyFixtureRejectsSymlinkedFixtureRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "litmus", "fixtures", "target")
	writeFile(t, filepath.Join(target, "report.txt"), "BLOCK")
	symlink(t, target, filepath.Join(root, "litmus", "fixtures", "alias"))

	workspace, cleanup, err := copyFixture(root, "alias")
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("copyFixture() = %q, %v, want symlink rejection", workspace, err)
	}
}

func TestCopyFixtureWithoutFixtureCreatesEmptyWorkspace(t *testing.T) {
	workspace, cleanup, err := copyFixture(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("workspace has %d entries, want empty directory", len(entries))
	}
}

func TestCopyFixtureIgnoresGitkeep(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "litmus", "fixtures", "empty", ".gitkeep"), "")

	workspace, cleanup, err := copyFixture(root, "empty")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("workspace has %d entries, want empty directory", len(entries))
	}
}

func TestProbeBuildsExpectedProductionRequest(t *testing.T) {
	root := testRepo(t)
	writeFile(t, filepath.Join(root, "litmus", "fixtures", "review", "input.txt"), "fixture data")
	fake := &fakeExecutor{response: ProviderResponse{
		Output:       "BLOCK: CWE-94",
		InputTokens:  100,
		OutputTokens: 20,
		CostUSD:      0.04,
		Duration:     250 * time.Millisecond,
	}}
	runner := Runner{Root: root, Executor: fake, Now: fixedNow}
	testCase := Case{
		ID:           "eval-exec-injection",
		Agent:        "code-reviewer",
		Task:         "Review this code",
		MaxBudgetUSD: 0.10,
		Live:         true,
		Fixture:      "review",
		Assertions:   []Assertion{{Type: "contains", Value: "BLOCK"}},
	}

	result, err := runner.Probe(context.Background(), testCase, 0.80, 0.74)
	if err != nil {
		t.Fatal(err)
	}

	request := fake.request
	workspace := request.Workspace
	request.Workspace = ""
	want := ProviderRequest{
		Agent:        "code-reviewer",
		Task:         "Review this code",
		SystemPrompt: "# Code Reviewer",
		Model:        "sonnet",
		BudgetUSD:    0.06,
		AllowTools:   false,
	}
	if !reflect.DeepEqual(request, want) {
		t.Fatalf("Probe() request = %#v, want %#v", request, want)
	}
	if workspace == "" || filepath.Base(workspace) == "review" {
		t.Fatalf("Probe() workspace = %q, want isolated fixture workspace", workspace)
	}
	if !result.Passed || result.CostUSD != 0.04 || result.DurationMS != 250 {
		t.Fatalf("Probe() result = %#v, want passing provider metrics", result)
	}
	if result.PromptHash == "" || result.FixtureHash != "" {
		t.Fatalf("Probe() hashes = prompt %q fixture %q, want prompt hash only", result.PromptHash, result.FixtureHash)
	}
	if result.ProviderRequestModel != "claude-sonnet-5" {
		t.Fatalf("Probe() provider request model = %q, want claude-sonnet-5", result.ProviderRequestModel)
	}
}

func TestProbeRejectsReplayOnlyCase(t *testing.T) {
	root := testRepo(t)
	fake := &fakeExecutor{}
	runner := Runner{Root: root, Executor: fake}
	testCase := Case{
		ID:           "case",
		Agent:        "code-reviewer",
		Task:         "Review this code",
		MaxBudgetUSD: 0.10,
		Live:         false,
		Assertions:   []Assertion{{Type: "contains", Value: "APPROVE"}},
	}

	if _, err := runner.Probe(context.Background(), testCase, 0.10, 0); err == nil ||
		!strings.Contains(err.Error(), "not enabled for live probes") {
		t.Fatalf("Probe() error = %v, want replay-only rejection", err)
	}
	if fake.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", fake.calls)
	}
}

func TestProbeDisablesToolsForLiveCases(t *testing.T) {
	root := testRepo(t)
	writeFile(t, filepath.Join(root, "agents", "builder", "manifest.json"),
		`{"name":"builder","profile":"haiku"}`)
	writeFile(t, filepath.Join(root, "agents", "builder", "claude.md"), `---
name: builder
description: Builder
model: claude-haiku-5
effort: medium
---
# Builder`)
	fake := &fakeExecutor{response: ProviderResponse{Output: "done"}}
	runner := Runner{Root: root, Executor: fake}
	testCase := Case{
		ID:           "case",
		Agent:        "builder",
		Task:         "Do one task",
		MaxBudgetUSD: 0.10,
		Live:         true,
		Assertions:   []Assertion{{Type: "contains", Value: "done"}},
	}

	if _, err := runner.Probe(context.Background(), testCase, 0.10, 0); err != nil {
		t.Fatal(err)
	}
	if fake.request.AllowTools || fake.request.Model != "haiku" {
		t.Fatalf("Probe() request = %#v, want disabled tools and haiku alias", fake.request)
	}
}

func TestProbeReturnsProviderFailureWithCapturedMetrics(t *testing.T) {
	root := testRepo(t)
	fake := &fakeExecutor{
		response: ProviderResponse{
			Output:       "partial response",
			InputTokens:  12,
			OutputTokens: 4,
			CostUSD:      0.03,
			Duration:     15 * time.Millisecond,
		},
		err: fmt.Errorf("provider unavailable"),
	}
	runner := Runner{Root: root, Executor: fake}
	testCase := Case{
		ID:           "case",
		Agent:        "code-reviewer",
		Task:         "Review this",
		MaxBudgetUSD: 0.10,
		Live:         true,
		Assertions:   []Assertion{{Type: "contains", Value: "partial"}},
	}

	result, err := runner.Probe(context.Background(), testCase, 0.10, 0)
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("Probe() error = %v, want provider error", err)
	}
	if result.ProviderError == "" || result.Status != StatusInfrastructureErr ||
		result.Passed || result.Output != "partial response" ||
		result.InputTokens != 12 || result.OutputTokens != 4 || result.CostUSD != 0.03 ||
		result.DurationMS != 15 {
		t.Fatalf("Probe() result = %#v, want failed result retaining provider response", result)
	}
}

func TestProbeClassifiesAssertionFailureAsAgentFailure(t *testing.T) {
	root := testRepo(t)
	fake := &fakeExecutor{response: ProviderResponse{Output: "not approved"}}
	runner := Runner{Root: root, Executor: fake}
	testCase := Case{
		ID:           "case",
		Agent:        "code-reviewer",
		Task:         "Review this",
		MaxBudgetUSD: 0.10,
		Live:         true,
		Assertions:   []Assertion{{Type: "contains", Value: "APPROVE"}},
	}

	result, err := runner.Probe(context.Background(), testCase, 0.10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || result.Status != StatusAgentFailure {
		t.Fatalf("Probe() result = %#v, want agent failure status", result)
	}
}

func TestProbeStopsWhenNoBudgetRemains(t *testing.T) {
	runner := Runner{Root: t.TempDir(), Executor: &fakeExecutor{}}
	_, err := runner.Probe(context.Background(), Case{MaxBudgetUSD: 0.10}, 0.10, 0.10)
	if err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("Probe() error = %v, want budget error", err)
	}
}

func TestProviderBudgetReservesTwoCents(t *testing.T) {
	tests := []struct {
		requested float64
		want      float64
	}{
		{requested: 0.10, want: 0.08},
		{requested: 0.03, want: 0.01},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("$%.2f", test.requested), func(t *testing.T) {
			got, err := providerBudget(test.requested)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("providerBudget(%.2f) = %.2f, want %.2f", test.requested, got, test.want)
			}
		})
	}
}

func TestProviderBudgetRejectsTwoCentCap(t *testing.T) {
	if _, err := providerBudget(0.02); err == nil {
		t.Fatal("providerBudget() error = nil, want minimum-cap error")
	}
}

func TestClaudeArgsDisableBuiltInTools(t *testing.T) {
	args, err := claudeArgs(ProviderRequest{
		Model:        "sonnet",
		SystemPrompt: "# Builder",
		BudgetUSD:    0.10,
		AllowTools:   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-p",
		"--output-format", "json",
		"--model", "claude-sonnet-5",
		"--max-budget-usd", "0.08",
		"--system-prompt", "# Builder",
		"--tools", "",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("claudeArgs() = %#v, want %#v", args, want)
	}
}

func TestClaudeArgsAddsJSONSchema(t *testing.T) {
	args, err := claudeArgs(ProviderRequest{
		Model:        "sonnet",
		SystemPrompt: "# Validator",
		BudgetUSD:    0.10,
		JSONSchema:   `{"type":"object","required":["finding_1"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args[len(args)-2:], []string{"--json-schema", `{"type":"object","required":["finding_1"]}`}) {
		t.Fatalf("claudeArgs() tail = %#v, want JSON schema flag", args[len(args)-2:])
	}
}

func TestResolveProductionAgentLoadsNativeClaudeVariant(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agents", "builder", "manifest.json"),
		`{"name":"builder","profile":"haiku"}`)
	writeFile(t, filepath.Join(root, "agents", "builder", "claude.md"), `---
name: builder
description: Builder
model: claude-haiku-4-5
effort: medium
---
# Builder`)

	prompt, model, err := resolveProductionAgent(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "# Builder" || model != "haiku" {
		t.Fatalf("resolveProductionAgent() = (%q, %q), want native prompt and haiku", prompt, model)
	}
}

func TestResolveProductionAgentRejectsMalformedFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agents", "builder", "manifest.json"),
		`{"name":"builder","profile":"haiku"}`)
	writeFile(t, filepath.Join(root, "agents", "builder", "claude.md"), `---
name: builder
model: claude-haiku-4-5
`)

	if _, _, err := resolveProductionAgent(root, "builder"); err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("resolveProductionAgent() error = %v, want malformed frontmatter", err)
	}
}

func TestResolveProductionAgentRequiresModel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agents", "builder", "manifest.json"),
		`{"name":"builder","profile":"haiku"}`)
	writeFile(t, filepath.Join(root, "agents", "builder", "claude.md"), `---
name: builder
description: Builder
effort: medium
---
# Builder`)

	if _, _, err := resolveProductionAgent(root, "builder"); err == nil || !strings.Contains(err.Error(), "agent model is required") {
		t.Fatalf("resolveProductionAgent() error = %v, want missing model", err)
	}
}

func TestResolveProductionAgentRejectsManifestNameMismatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agents", "builder", "manifest.json"),
		`{"name":"other","profile":"haiku"}`)
	writeFile(t, filepath.Join(root, "agents", "builder", "claude.md"), `---
name: builder
description: Builder
model: claude-haiku-4-5
effort: medium
---
# Builder`)

	if _, _, err := resolveProductionAgent(root, "builder"); err == nil || !strings.Contains(err.Error(), "manifest name") {
		t.Fatalf("resolveProductionAgent() error = %v, want manifest mismatch", err)
	}
}

func TestDecodeProviderResponseAggregatesUsageAndPreservesFailures(t *testing.T) {
	response, err := decodeProviderResponse([]byte(`{
		"result": "BLOCK",
		"uuid": "response-id",
		"session_id": "session-id",
		"modelUsage": {
			"claude-sonnet-5": {"inputTokens": 10, "outputTokens": 3},
			"claude-haiku-5": {"inputTokens": 2, "outputTokens": 1}
		},
		"total_cost_usd": 0.04,
		"duration_ms": 125
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(response.ProviderModels, []string{"claude-haiku-5", "claude-sonnet-5"}) {
		t.Fatalf("decodeProviderResponse() models = %#v, want sorted provider model keys", response.ProviderModels)
	}
	if response.ProviderResponseID != "response-id" || response.ProviderSessionID != "session-id" {
		t.Fatalf("decodeProviderResponse() IDs = %#v/%#v, want response/session IDs", response.ProviderResponseID, response.ProviderSessionID)
	}
	if len(response.ProviderModelUsage) != 2 {
		t.Fatalf("decodeProviderResponse() model usage = %#v, want raw model usage entries", response.ProviderModelUsage)
	}
	want := ProviderResponse{
		Output:             "BLOCK",
		ProviderModels:     []string{"claude-haiku-5", "claude-sonnet-5"},
		ProviderResponseID: "response-id",
		ProviderSessionID:  "session-id",
		ProviderModelUsage: map[string]map[string]json.RawMessage{
			"claude-haiku-5": {
				"inputTokens":  json.RawMessage("2"),
				"outputTokens": json.RawMessage("1"),
			},
			"claude-sonnet-5": {
				"inputTokens":  json.RawMessage("10"),
				"outputTokens": json.RawMessage("3"),
			},
		},
		InputTokens:  12,
		OutputTokens: 4,
		CostUSD:      0.04,
		Duration:     125 * time.Millisecond,
	}
	if !reflect.DeepEqual(response, want) {
		t.Fatalf("decodeProviderResponse() = %#v, want %#v", response, want)
	}

	response, err = decodeProviderResponse([]byte(`{
		"result": "partial",
		"modelUsage": {"claude-sonnet-5": {"inputTokens": 5, "outputTokens": 2}},
		"total_cost_usd": 0.01,
		"is_error": true,
		"errors": ["rate limited"]
	}`))
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("decodeProviderResponse() error = %v, want provider error", err)
	}
	if response.Output != "partial" || response.InputTokens != 5 || response.OutputTokens != 2 || response.CostUSD != 0.01 {
		t.Fatalf("decodeProviderResponse() failure response = %#v, want retained fields", response)
	}

	if _, err := decodeProviderResponse([]byte(`{"result":1}`)); err == nil {
		t.Fatal("decodeProviderResponse() error = nil, want invalid result rejection")
	}
}

func TestDecodeProviderResponseClassifiesErrorEnvelopeWithoutResult(t *testing.T) {
	response, err := decodeProviderResponse([]byte(`{
		"type": "result",
		"subtype": "error_max_budget_usd",
		"is_error": true,
		"errors": ["Reached maximum budget ($0.05)"],
		"total_cost_usd": 0.05
	}`))
	if err == nil || !strings.Contains(err.Error(), "provider error") {
		t.Fatalf("decodeProviderResponse() error = %v, want provider error", err)
	}
	if response.CostUSD != 0.05 {
		t.Fatalf("decodeProviderResponse() cost = %v, want 0.05", response.CostUSD)
	}
}

func TestCoreManifestReferencesExistingCases(t *testing.T) {
	root := repoRoot(t)
	manifest, err := LoadManifest(root, "core")
	if err != nil {
		t.Fatal(err)
	}
	want := []ManifestItem{
		{Agent: "code-reviewer", CaseID: "eval-exec-injection"},
		{Agent: "code-reviewer", CaseID: "clean-code-approval"},
	}
	if !reflect.DeepEqual(manifest.Cases, want) {
		t.Fatalf("core manifest = %#v, want %#v", manifest.Cases, want)
	}
	for _, item := range manifest.Cases {
		if _, err := LoadCase(root, item.Agent, item.CaseID); err != nil {
			t.Fatalf("manifest reference %#v: %v", item, err)
		}
	}
}

func TestDeterministicCatalogContainsAllCases(t *testing.T) {
	root := repoRoot(t)
	want := []ManifestItem{
		{Agent: "builder", CaseID: "ambiguity"},
		{Agent: "builder", CaseID: "injection-resistance"},
		{Agent: "builder", CaseID: "parse-pair"},
		{Agent: "builder", CaseID: "refuse-delegation"},
		{Agent: "code-reviewer", CaseID: "clean-code-approval"},
		{Agent: "code-reviewer", CaseID: "eval-exec-injection"},
		{Agent: "code-reviewer", CaseID: "injection-resistance"},
		{Agent: "context-curator", CaseID: "context-only"},
		{Agent: "context-curator", CaseID: "research-boundary"},
		{Agent: "documenter", CaseID: "required-sections"},
		{Agent: "glide-code-reviewer", CaseID: "client-lifecycle"},
		{Agent: "researcher", CaseID: "valkey-glide-recommendation"},
		{Agent: "research-validator", CaseID: "classify-findings"},
		{Agent: "security-reviewer", CaseID: "clean-code"},
		{Agent: "security-reviewer", CaseID: "sql-ssrf"},
		{Agent: "tester", CaseID: "happy-error"},
		{Agent: "validator", CaseID: "missing-zero-check"},
		{Agent: "validator", CaseID: "report-only"},
		{Agent: "valkey-glide-implementor", CaseID: "injection-resistance"},
		{Agent: "valkey-glide-implementor", CaseID: "python-batch"},
	}

	for _, item := range want {
		testCase, err := LoadCase(root, item.Agent, item.CaseID)
		if err != nil {
			t.Fatalf("LoadCase(%s, %s): %v", item.Agent, item.CaseID, err)
		}
		if testCase.Live && !(item.Agent == "code-reviewer" &&
			(item.CaseID == "clean-code-approval" || item.CaseID == "eval-exec-injection" ||
				item.CaseID == "injection-resistance")) {
			t.Fatalf("case %s/%s is unexpectedly live-enabled", item.Agent, item.CaseID)
		}
	}
}

func TestReplayCatalog(t *testing.T) {
	root := repoRoot(t)
	casesRoot := filepath.Join(root, "litmus", "cases")
	agents, err := os.ReadDir(casesRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, agentEntry := range agents {
		if !agentEntry.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(casesRoot, agentEntry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			t.Run(agentEntry.Name()+"/"+id, func(t *testing.T) {
				testCase, err := LoadCase(root, agentEntry.Name(), id)
				if err != nil {
					t.Fatal(err)
				}
				result, err := Replay(root, testCase)
				if err != nil {
					t.Fatal(err)
				}
				if !result.Passed {
					t.Fatalf("replay failed: %#v", result)
				}
			})
		}
	}
}

func validCaseJSON(id, agent string) string {
	return fmt.Sprintf(`{
		"id": %q,
		"agent": %q,
		"task": "review task",
		"max_budget_usd": 0.10,
		"assertions": [{"type": "contains", "value": "APPROVE"}]
	}`, id, agent)
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
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

func writeReplay(t *testing.T, root, agent, id, contents string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "litmus", "replays", agent, id+".json"), contents)
}

type fakeExecutor struct {
	request  ProviderRequest
	response ProviderResponse
	err      error
	calls    int
}

func (f *fakeExecutor) Execute(_ context.Context, request ProviderRequest) (ProviderResponse, error) {
	f.calls++
	f.request = request
	return f.response, f.err
}

func fixedNow() time.Time {
	return time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC)
}

func testRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "agents", "code-reviewer", "manifest.json"),
		`{"name":"code-reviewer","profile":"sonnet"}`)
	writeFile(t, filepath.Join(root, "agents", "code-reviewer", "claude.md"), `---
name: code-reviewer
description: Code Reviewer
model: claude-sonnet-5
effort: medium
---
# Code Reviewer`)
	return root
}

func repoRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(directory, "../../.."))
}
