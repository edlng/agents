# Litmus Eval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `litmus-eval`, a small Go CLI that runs selected production-agent evaluations with deterministic checks, hard budgets, and versioned JSON/Markdown results.

**Architecture:** `cmd/litmus-eval/main.go` owns command parsing and process exit codes. `internal/litmus/runner.go` owns case discovery, replay scoring, direct `claude -p` execution, fixture setup, and deterministic checks. `internal/litmus/results.go` owns durable run artifacts and local comparison. Cases, fixtures, replays, and results live under `litmus/`; results are committed, while Promptfoo remains untouched during the initial migration.

**Tech Stack:** Go 1.26 standard library, `claude` CLI, JSON, Make, existing agent prompt/config files. Every Go command uses `GOPROXY=direct`; no third-party Go module is added.

---

## File Structure

| Path | Responsibility |
| --- | --- |
| `go.mod` | Defines the dependency-free Litmus Eval Go module. |
| `cmd/litmus-eval/main.go` | Parses `list`, `replay`, `probe`, `batch`, and `compare`; formats usage and maps outcomes to exit codes. |
| `cmd/litmus-eval/main_test.go` | Tests command parsing without invoking the `claude` executable. |
| `internal/litmus/runner.go` | Defines case/result types, catalog loading, assertions, fixture copying, agent prompt/model resolution, and direct provider invocation. |
| `internal/litmus/runner_test.go` | Covers schema validation, deterministic checks, replay, budget selection, fixture isolation, and mocked provider output. |
| `internal/litmus/results.go` | Writes/reads versioned result directories, pretty JSON, Markdown reports, and comparisons. |
| `internal/litmus/results_test.go` | Covers JSON artifact shape, report content, and comparison deltas. |
| `litmus/cases/code-reviewer/*.json` | First migrated core and boundary cases for the code-reviewer. |
| `litmus/cases/builder/*.json` | First migrated core and boundary cases for the builder. |
| `litmus/replays/<agent>/*.json` | Captured, versioned output used for zero-cost assertion verification. |
| `litmus/results/.gitkeep` | Keeps the committed run-history directory present before the first live run. |
| `Makefile` | Adds validated `litmus-*` wrappers, each using `GOPROXY=direct`. |
| `README.md` | Documents Litmus Eval commands, budget tiers, and committed result artifacts. |

### Task 1: Scaffold the dependency-free Go module and domain schema

**Files:**
- Create: `go.mod`
- Create: `internal/litmus/runner.go`
- Create: `internal/litmus/runner_test.go`

- [ ] **Step 1: Write the failing case-schema and budget tests**

```go
func TestLoadCaseRejectsInvalidBudget(t *testing.T) {
    root := t.TempDir()
    writeFile(t, filepath.Join(root, "litmus/cases/reviewer/bad.json"), `{
      "id": "bad", "agent": "reviewer", "task": "x", "max_budget_usd": 0
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestLoadCaseRejectsInvalidBudget|TestEffectiveBudgetUsesRemainingRunBudget' -v
```

Expected: FAIL because `internal/litmus` and its symbols do not exist.

- [ ] **Step 3: Create the module and minimal domain implementation**

Create `go.mod`:

```go
module github.com/edlng/agents/litmus-eval

go 1.26
```

Create `internal/litmus/runner.go` with these stable types and functions:

```go
package litmus

type Assertion struct {
    Type    string `json:"type"`
    Value   string `json:"value,omitempty"`
    Path    string `json:"path,omitempty"`
    Command string `json:"command,omitempty"`
}

type Case struct {
    ID           string      `json:"id"`
    Agent        string      `json:"agent"`
    Task         string      `json:"task"`
    MaxBudgetUSD float64     `json:"max_budget_usd"`
    Fixture      string      `json:"fixture,omitempty"`
    Assertions   []Assertion `json:"assertions"`
}

type ManifestItem struct {
    Agent  string `json:"agent"`
    CaseID string `json:"case"`
}

type Manifest struct {
    Cases []ManifestItem `json:"cases"`
}

func LoadCase(root, agent, id string) (Case, error)
func LoadManifest(root, name string) (Manifest, error)
func EffectiveBudget(caseLimit, runLimit, spent float64) (float64, error)
```

`LoadCase` reads `litmus/cases/<agent>/<id>.json`, rejects empty IDs, agents,
tasks, assertion lists, and non-positive budgets. `EffectiveBudget` returns
`min(caseLimit, runLimit-spent)`, rounded to cents; it returns an error when no
budget remains. `LoadManifest` reads `litmus/manifests/<name>.json` and
rejects an empty `cases` list or an item with an empty agent/case ID.

Add these test helpers at the bottom of `internal/litmus/runner_test.go`:

```go
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
    writeFile(t, filepath.Join(root, "litmus/replays", agent, id+".json"), contents)
}

func fixedNow() time.Time {
    return time.Date(2026, time.July, 15, 14, 30, 22, 0, time.UTC)
}

func testRepo(t *testing.T) string {
    t.Helper()
    root := t.TempDir()
    writeFile(t, filepath.Join(root, "agents/code-reviewer-prompt.md"), "# Code Reviewer")
    writeFile(t, filepath.Join(root, "agents/code-reviewer.json"), `{"model":"claude-sonnet-5"}`)
    return root
}

func repoRoot(t *testing.T) string {
    t.Helper()
    directory, err := os.Getwd()
    if err != nil {
        t.Fatal(err)
    }
    return filepath.Clean(filepath.Join(directory, "../.."))
}
```

- [ ] **Step 4: Run the focused tests to verify they pass**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestLoadCaseRejectsInvalidBudget|TestEffectiveBudgetUsesRemainingRunBudget' -v
```

Expected: PASS.

- [ ] **Step 5: Commit the scaffold**

```shell
git add go.mod internal/litmus/runner.go internal/litmus/runner_test.go
git commit -m "feat: scaffold litmus eval runner"
```

### Task 2: Implement deterministic checks, replay, and fixture isolation

**Files:**
- Modify: `internal/litmus/runner.go`
- Modify: `internal/litmus/runner_test.go`
- Create: `litmus/fixtures/empty/.gitkeep`
- Create: `litmus/replays/code-reviewer/eval-exec-injection.json`
- Create: `litmus/replays/code-reviewer/clean-code-approval.json`

- [ ] **Step 1: Write failing deterministic-check and replay tests**

```go
func TestEvaluateAssertionsSupportsTextRegexAndJSON(t *testing.T) {
    checks := []Assertion{
        {Type: "contains", Value: "BLOCK"},
        {Type: "regex", Value: `CWE-(94|78)`},
        {Type: "json_path", Path: "verdict", Value: "block"},
    }
    output := `{"verdict":"block","finding":"BLOCK: CWE-94"}`

    results := EvaluateAssertions(output, "", checks)
    for _, result := range results {
        if !result.Passed {
            t.Fatalf("check %#v failed: %s", result.Assertion, result.Reason)
        }
    }
}

func TestReplayScoresCapturedOutputWithoutExecutor(t *testing.T) {
    root := t.TempDir()
    writeReplay(t, root, "reviewer", "case", `{"output":"APPROVE"}`)
    testCase := Case{
        ID: "case", Agent: "reviewer", Task: "ignored",
        MaxBudgetUSD: 0.10,
        Assertions: []Assertion{{Type: "contains", Value: "APPROVE"}},
    }
    result, err := Replay(root, testCase)
    if err != nil {
        t.Fatal(err)
    }
    if result.CostUSD != 0 || !result.Passed {
        t.Fatalf("Replay() = %#v, want zero-cost pass", result)
    }
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestEvaluateAssertionsSupportsTextRegexAndJSON|TestReplayScoresCapturedOutputWithoutExecutor' -v
```

Expected: FAIL because assertion and replay functions are absent.

- [ ] **Step 3: Add check and replay behavior**

Add these types and functions to `internal/litmus/runner.go`:

```go
type AssertionResult struct {
    Assertion Assertion `json:"assertion"`
    Passed    bool      `json:"passed"`
    Reason    string    `json:"reason"`
}

type CaseResult struct {
    Agent            string            `json:"agent"`
    CaseID           string            `json:"case_id"`
    Model            string            `json:"model,omitempty"`
    PromptHash       string            `json:"prompt_hash,omitempty"`
    FixtureHash      string            `json:"fixture_hash,omitempty"`
    Output           string            `json:"output"`
    AssertionResults []AssertionResult `json:"assertion_results"`
    Passed           bool              `json:"passed"`
    InputTokens      int               `json:"input_tokens"`
    OutputTokens     int               `json:"output_tokens"`
    CostUSD          float64           `json:"cost_usd"`
    DurationMS       int64             `json:"duration_ms"`
    ProviderError    string            `json:"provider_error,omitempty"`
}

func EvaluateAssertions(output, workspace string, assertions []Assertion) []AssertionResult
func Replay(root string, testCase Case) (CaseResult, error)
```

Implement assertion types:

- `contains`: `strings.Contains(output, Value)`.
- `regex`: compile `Value` with `regexp.Compile` and match output.
- `json_path`: decode output as JSON, traverse a dot-separated object path, and
  compare its string/number/bool JSON representation with `Value`.
- `file_contains`: reject absolute and parent-traversal paths, read `Path`
  relative to workspace, and search for `Value`.
- `command_exit`: reject an empty command, execute it with `sh -c` in
  `workspace`, and pass only when its exit status equals the integer in
  `Value`.

`Replay` reads `litmus/replays/<agent>/<case>.json`, where the schema is:

```json
{
  "output": "captured agent output"
}
```

It runs checks, sets `CostUSD` to zero, and never creates a provider process.
Add `copyFixture(root, fixture string) (string, func(), error)` which copies
`litmus/fixtures/<fixture>` to a temporary directory and returns a cleanup
function; use an empty temporary directory when no fixture is configured.

- [ ] **Step 4: Add first replay artifacts**

Create `litmus/replays/code-reviewer/eval-exec-injection.json`:

```json
{
  "output": "BLOCK: CWE-94 eval injection and CWE-78 command injection. Replace eval() with JSON.parse and avoid shell interpolation."
}
```

Create `litmus/replays/code-reviewer/clean-code-approval.json`:

```json
{
  "output": "APPROVE"
}
```

- [ ] **Step 5: Run focused and full package tests**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestEvaluateAssertionsSupportsTextRegexAndJSON|TestReplayScoresCapturedOutputWithoutExecutor' -v
GOPROXY=direct go test ./internal/litmus -v
```

Expected: PASS.

- [ ] **Step 6: Commit deterministic evaluation support**

```shell
git add internal/litmus/runner.go internal/litmus/runner_test.go litmus/fixtures/empty/.gitkeep litmus/replays/code-reviewer
git commit -m "feat: add litmus replay checks"
```

### Task 3: Add durable results and local comparison

**Files:**
- Create: `internal/litmus/results.go`
- Create: `internal/litmus/results_test.go`
- Create: `litmus/results/.gitkeep`

- [ ] **Step 1: Write failing persistence and comparison tests**

```go
func TestWriteRunCreatesReadableArtifacts(t *testing.T) {
    root := t.TempDir()
    run := Run{
        ID: "20260715T143022-a1b2c3d", Revision: "a1b2c3d",
        BudgetUSD: 0.10,
        Cases: []CaseResult{{Agent: "reviewer", CaseID: "case", Passed: true, CostUSD: 0.04}},
    }

    directory, err := WriteRun(root, run)
    if err != nil {
        t.Fatal(err)
    }
    for _, name := range []string{"summary.json", "report.md", "cases/reviewer--case.json"} {
        if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
            t.Fatalf("missing %s: %v", name, err)
        }
    }
}

func TestCompareReportsCostAndPassDelta(t *testing.T) {
    baseline := Run{ID: "base", Cases: []CaseResult{{Agent: "a", CaseID: "c", Passed: true, CostUSD: 0.03}}}
    current := Run{ID: "next", Cases: []CaseResult{{Agent: "a", CaseID: "c", Passed: false, CostUSD: 0.05}}}
    diff := Compare(baseline, current)
    if diff.Cases[0].Status != "regressed" || diff.TotalCostDeltaUSD != 0.02 {
        t.Fatalf("Compare() = %#v", diff)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestWriteRunCreatesReadableArtifacts|TestCompareReportsCostAndPassDelta' -v
```

Expected: FAIL because result types and functions are absent.

- [ ] **Step 3: Implement versioned result artifacts**

Create `internal/litmus/results.go` containing:

```go
type Run struct {
    ID        string       `json:"id"`
    Timestamp time.Time    `json:"timestamp"`
    Revision  string       `json:"revision,omitempty"`
    BudgetUSD float64      `json:"budget_usd"`
    Cases     []CaseResult `json:"cases"`
}

type CaseDelta struct {
    Agent       string  `json:"agent"`
    CaseID      string  `json:"case_id"`
    Status      string  `json:"status"`
    CostDeltaUSD float64 `json:"cost_delta_usd"`
}

type Comparison struct {
    BaselineID        string      `json:"baseline_id"`
    CurrentID         string      `json:"current_id"`
    TotalCostDeltaUSD float64     `json:"total_cost_delta_usd"`
    Cases             []CaseDelta `json:"cases"`
}

func NewRun(now time.Time, revision string, budgetUSD float64, cases []CaseResult) Run
func WriteRun(root string, run Run) (string, error)
func ReadRun(path string) (Run, error)
func Compare(baseline, current Run) Comparison
```

`WriteRun` creates
`litmus/results/<UTC timestamp>-<revision>/cases/`, writes indented JSON using
`json.Encoder.SetIndent("", "  ")`, writes every `CaseResult` to
`cases/<agent>--<case>.json`, and writes `summary.json` with aggregate token,
cost, duration, pass/fail, and detail-path fields. It writes `report.md` with
a title, run metadata, totals table, and one Markdown table row per case.

`Compare` classifies the same agent/case as `regressed`, `improved`,
`unchanged`, `added`, or `removed`. Compare cost using rounded cents to avoid
floating-point presentation artifacts.

- [ ] **Step 4: Run persistence and package tests**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestWriteRunCreatesReadableArtifacts|TestCompareReportsCostAndPassDelta' -v
GOPROXY=direct go test ./internal/litmus -v
```

Expected: PASS.

- [ ] **Step 5: Commit durable result support**

```shell
git add internal/litmus/results.go internal/litmus/results_test.go litmus/results/.gitkeep
git commit -m "feat: persist litmus evaluation results"
```

### Task 4: Implement the direct production-agent executor and probe operation

**Files:**
- Modify: `internal/litmus/runner.go`
- Modify: `internal/litmus/runner_test.go`

- [ ] **Step 1: Write failing direct-executor tests with a fake command runner**

```go
func TestProbeUsesProductionPromptAndBudget(t *testing.T) {
    fake := &fakeExecutor{response: ProviderResponse{
        Output: "BLOCK: CWE-94",
        InputTokens: 100, OutputTokens: 20, CostUSD: 0.04,
    }}
    runner := Runner{Root: testRepo(t), Executor: fake, Now: fixedNow}
    testCase := Case{
        ID: "eval-exec-injection", Agent: "code-reviewer", Task: "Review this code",
        MaxBudgetUSD: 0.10,
        Assertions: []Assertion{{Type: "contains", Value: "BLOCK"}},
    }

    result, err := runner.Probe(context.Background(), testCase, 0.10, 0)
    if err != nil {
        t.Fatal(err)
    }
    if !result.Passed || fake.request.Model != "sonnet" || fake.request.BudgetUSD != 0.10 {
        t.Fatalf("Probe() result=%#v request=%#v", result, fake.request)
    }
    if !strings.Contains(fake.request.SystemPrompt, "Code Reviewer") {
        t.Fatal("Probe() did not load the production system prompt")
    }
}

func TestProbeStopsWhenNoBudgetRemains(t *testing.T) {
    runner := Runner{Root: t.TempDir(), Executor: &fakeExecutor{}}
    _, err := runner.Probe(context.Background(), Case{MaxBudgetUSD: 0.10}, 0.10, 0.10)
    if err == nil || !strings.Contains(err.Error(), "budget") {
        t.Fatalf("Probe() error = %v, want budget error", err)
    }
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestProbeUsesProductionPromptAndBudget|TestProbeStopsWhenNoBudgetRemains' -v
```

Expected: FAIL because `Runner`, `ProviderResponse`, and `Probe` are absent.

- [ ] **Step 3: Add the provider seam and direct `claude` executor**

Add these definitions to `internal/litmus/runner.go`:

```go
type ProviderRequest struct {
    Agent        string
    Task         string
    SystemPrompt string
    Model        string
    BudgetUSD    float64
    Workspace    string
    AllowTools   bool
}

type ProviderResponse struct {
    Output       string
    InputTokens  int
    OutputTokens int
    CostUSD      float64
    Duration     time.Duration
}

type Executor interface {
    Execute(context.Context, ProviderRequest) (ProviderResponse, error)
}

type Runner struct {
    Root     string
    Executor Executor
    Now      func() time.Time
}

func (r Runner) Probe(ctx context.Context, testCase Case, runBudget, spent float64) (CaseResult, error)
```

Define this test-only provider alongside the Task 4 tests:

```go
type fakeExecutor struct {
    request  ProviderRequest
    response ProviderResponse
    err      error
}

func (f *fakeExecutor) Execute(_ context.Context, request ProviderRequest) (ProviderResponse, error) {
    f.request = request
    return f.response, f.err
}
```

Implement `claudeExecutor.Execute` with `exec.CommandContext`:

```go
args := []string{
    "-p",
    "--output-format", "json",
    "--model", request.Model,
    "--max-budget-usd", fmt.Sprintf("%.2f", request.BudgetUSD),
    "--system-prompt", request.SystemPrompt,
}
if !request.AllowTools {
    args = append(args, "--allowedTools", "none")
}
```

Set the command directory to the temporary fixture workspace, set standard
input to the case task, decode the JSON response, and return output plus
`modelUsage` totals and `total_cost_usd`. Return stderr alongside a non-zero
exit error without treating it as an assertion pass.

Replicate the current provider's prompt resolution order:

1. `agents/<agent>-prompt.md`
2. `agents/<agent>.md`
3. `skills/<agent>/SKILL.md`
4. inline `prompt` in `agents/<agent>.json`

Normalize `claude-sonnet-*`, `claude-haiku-*`, and `claude-opus-*` to
`sonnet`, `haiku`, and `opus`. Set `AllowTools` only for `researcher`,
`research-validator`, and `builder`, matching the current provider behavior.

- [ ] **Step 4: Run focused and full Go tests**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run 'TestProbeUsesProductionPromptAndBudget|TestProbeStopsWhenNoBudgetRemains' -v
GOPROXY=direct go test ./... -v
```

Expected: PASS. No live provider is invoked by unit tests.

- [ ] **Step 5: Commit direct probe support**

```shell
git add internal/litmus/runner.go internal/litmus/runner_test.go
git commit -m "feat: run production probes with litmus"
```

### Task 5: Add the CLI commands and test their parsing

**Files:**
- Create: `cmd/litmus-eval/main.go`
- Create: `cmd/litmus-eval/main_test.go`

- [ ] **Step 1: Write failing CLI parsing tests**

```go
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
```

- [ ] **Step 2: Run the CLI tests to verify they fail**

Run:

```shell
GOPROXY=direct go test ./cmd/litmus-eval -run 'TestParseProbeCommand|TestParseProbeRejectsMissingCase' -v
```

Expected: FAIL because the command package does not exist.

- [ ] **Step 3: Implement command dispatch**

Create `cmd/litmus-eval/main.go` with a `main()` that calls:

```go
func run(args []string, stdout, stderr io.Writer) int
func parseArgs(args []string) (command, error)
```

Support these exact commands:

```text
litmus-eval list
litmus-eval replay <agent> <case>
litmus-eval probe <agent> <case> --budget <usd>
litmus-eval batch <manifest> --budget <usd>
litmus-eval compare <baseline-run> <current-run>
```

`list` prints all `litmus/cases/<agent>/<case>.json` IDs. `replay` loads one
case, scores the matching replay, persists a zero-cost run, and returns nonzero
when checks fail. `probe` loads one case, creates a run with the requested
budget, persists it even on provider failure, and returns nonzero on assertion
failure or provider error. `batch` resolves its `<manifest>` argument to
`litmus/manifests/<manifest>.json` and reads a JSON manifest:

```json
{
  "cases": [
    {"agent": "code-reviewer", "case": "eval-exec-injection"},
    {"agent": "builder", "case": "parse-pair"}
  ]
}
```

It runs in manifest order and stops before scheduling a case when remaining
budget is zero. `compare` accepts two run directories, writes its JSON and
Markdown comparison to stdout, and never invokes a provider.

- [ ] **Step 4: Run all CLI and package tests**

Run:

```shell
GOPROXY=direct go test ./cmd/litmus-eval -v
GOPROXY=direct go test ./... -v
GOPROXY=direct go build ./cmd/litmus-eval
```

Expected: PASS. The build creates a local `litmus-eval` binary, which must be
removed after verification.

- [ ] **Step 5: Remove the locally built binary and commit**

Run:

```shell
rm -f litmus-eval
git add cmd/litmus-eval/main.go cmd/litmus-eval/main_test.go
git commit -m "feat: add litmus eval cli"
```

### Task 6: Seed curated cases, replay artifacts, and a batch manifest

**Files:**
- Create: `litmus/cases/code-reviewer/eval-exec-injection.json`
- Create: `litmus/cases/code-reviewer/clean-code-approval.json`
- Create: `litmus/cases/builder/parse-pair.json`
- Create: `litmus/cases/builder/refuse-delegation.json`
- Create: `litmus/replays/builder/parse-pair.json`
- Create: `litmus/replays/builder/refuse-delegation.json`
- Create: `litmus/manifests/core.json`

- [ ] **Step 1: Write failing catalog coverage test**

```go
func TestCoreManifestReferencesExistingCases(t *testing.T) {
    manifest, err := LoadManifest(repoRoot(t), "core")
    if err != nil {
        t.Fatal(err)
    }
    if len(manifest.Cases) != 4 {
        t.Fatalf("core manifest has %d cases, want 4", len(manifest.Cases))
    }
    for _, item := range manifest.Cases {
        if _, err := LoadCase(repoRoot(t), item.Agent, item.CaseID); err != nil {
            t.Fatalf("manifest reference %#v: %v", item, err)
        }
    }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run TestCoreManifestReferencesExistingCases -v
```

Expected: FAIL because the manifest and cases do not exist.

- [ ] **Step 3: Add curated, deterministic case data**

Create `litmus/cases/code-reviewer/eval-exec-injection.json`:

```json
{
  "id": "eval-exec-injection",
  "agent": "code-reviewer",
  "task": "Review this code for issues:\n```javascript\nconst payload = eval('(' + req.body + ')');\nexec(`deploy.sh ${payload.target}`);\n```",
  "max_budget_usd": 0.10,
  "assertions": [
    {"type": "contains", "value": "BLOCK"},
    {"type": "regex", "value": "CWE-(94|78)|command injection"},
    {"type": "contains", "value": "eval"}
  ]
}
```

Create `litmus/cases/code-reviewer/clean-code-approval.json`:

```json
{
  "id": "clean-code-approval",
  "agent": "code-reviewer",
  "task": "Review this code:\n```typescript\nfunction clamp(value: number, min: number, max: number): number {\n  if (min > max) throw new Error(`min (${min}) must be <= max (${max})`);\n  return Math.min(Math.max(value, min), max);\n}\n```",
  "max_budget_usd": 0.08,
  "assertions": [
    {"type": "regex", "value": "APPROVE|no critical|looks good"},
    {"type": "not_regex", "value": "BLOCK|MUST_FIX|must fix"}
  ]
}
```

Add the `not_regex` assertion type to `EvaluateAssertions`; it passes only when
the pattern does not match the output.

Create `litmus/cases/builder/parse-pair.json` and
`litmus/cases/builder/refuse-delegation.json` from the current Promptfoo tasks.
Their deterministic checks must respectively require `def parse_pair` plus
`None`, and a refusal/clarification signal without a subagent-spawn claim.
Set both budget caps to `$0.10`.

Create `litmus/manifests/core.json`:

```json
{
  "cases": [
    {"agent": "code-reviewer", "case": "eval-exec-injection"},
    {"agent": "code-reviewer", "case": "clean-code-approval"},
    {"agent": "builder", "case": "parse-pair"},
    {"agent": "builder", "case": "refuse-delegation"}
  ]
}
```

Create replay JSON files for the two builder cases that satisfy their
assertions. Update `EvaluateAssertions` to support `not_regex`.

- [ ] **Step 4: Run catalog and replay verification**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run TestCoreManifestReferencesExistingCases -v
GOPROXY=direct go run ./cmd/litmus-eval replay code-reviewer eval-exec-injection
GOPROXY=direct go run ./cmd/litmus-eval replay builder parse-pair
```

Expected: the test passes; both replay commands create committed zero-cost
result directories under `litmus/results/` and report PASS.

- [ ] **Step 5: Commit curated baseline cases and replay results**

```shell
git add litmus/cases litmus/manifests litmus/replays litmus/results internal/litmus/runner.go internal/litmus/runner_test.go
git commit -m "feat: add litmus core cases"
```

### Task 7: Add Make targets and user documentation

**Files:**
- Modify: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Write a failing Make-target smoke check**

Run:

```shell
make litmus-list
```

Expected: FAIL with `No rule to make target 'litmus-list'`.

- [ ] **Step 2: Add validated Litmus Make targets**

Extend `.PHONY` and append these targets to `Makefile`:

```make
litmus-list:
	GOPROXY=direct go run ./cmd/litmus-eval list

litmus-replay:
	@if [ -z "$(AGENT)" ] || [ -z "$(CASE)" ]; then echo "Usage: make litmus-replay AGENT=<agent> CASE=<case>"; exit 1; fi
	GOPROXY=direct go run ./cmd/litmus-eval replay "$(AGENT)" "$(CASE)"

litmus-probe:
	@if [ -z "$(AGENT)" ] || [ -z "$(CASE)" ] || [ -z "$(BUDGET)" ]; then echo "Usage: make litmus-probe AGENT=<agent> CASE=<case> BUDGET=<usd>"; exit 1; fi
	GOPROXY=direct go run ./cmd/litmus-eval probe "$(AGENT)" "$(CASE)" --budget "$(BUDGET)"

litmus-batch:
	@if [ -z "$(MANIFEST)" ] || [ -z "$(BUDGET)" ]; then echo "Usage: make litmus-batch MANIFEST=<manifest> BUDGET=<usd>"; exit 1; fi
	GOPROXY=direct go run ./cmd/litmus-eval batch "$(MANIFEST)" --budget "$(BUDGET)"

litmus-compare:
	@if [ -z "$(BASELINE)" ] || [ -z "$(CURRENT)" ]; then echo "Usage: make litmus-compare BASELINE=<run-dir> CURRENT=<run-dir>"; exit 1; fi
	GOPROXY=direct go run ./cmd/litmus-eval compare "$(BASELINE)" "$(CURRENT)"
```

- [ ] **Step 3: Document the new manual workflow**

Replace the README evaluation run examples with:

```shell
make litmus-list
make litmus-replay AGENT=code-reviewer CASE=eval-exec-injection
make litmus-probe AGENT=code-reviewer CASE=eval-exec-injection BUDGET=0.10
make litmus-batch MANIFEST=core BUDGET=0.80
make litmus-compare BASELINE=litmus/results/<baseline> CURRENT=litmus/results/<current>
```

Document that `replay` is zero-cost and verifies evaluation logic only,
`probe` uses the production prompt/model and incurs a bounded live cost, and
all pretty-printed result artifacts under `litmus/results/` are intentionally
committed for future visualization. State that Promptfoo remains available
during migration.

- [ ] **Step 4: Run Make wrappers and Go verification**

Run:

```shell
make litmus-list
make litmus-replay AGENT=code-reviewer CASE=clean-code-approval
GOPROXY=direct go test ./... -v
GOPROXY=direct go build ./cmd/litmus-eval
```

Expected: all commands pass. Remove the generated local binary after the
build:

```shell
rm -f litmus-eval
```

- [ ] **Step 5: Commit integration and documentation**

```shell
git add Makefile README.md litmus/results
git commit -m "docs: add litmus evaluation workflow"
```

### Task 8: Perform one bounded live smoke probe and preserve the result

**Files:**
- Create: `litmus/results/<generated-run-id>/summary.json`
- Create: `litmus/results/<generated-run-id>/report.md`
- Create: `litmus/results/<generated-run-id>/cases/code-reviewer--eval-exec-injection.json`

- [ ] **Step 1: Run the real probe with the approved maximum**

Run:

```shell
make litmus-probe AGENT=code-reviewer CASE=eval-exec-injection BUDGET=0.10
```

Expected: one fresh `claude -p` invocation, one new committed result directory,
and a final PASS/FAIL line with actual spend. The command must not start a
second model invocation.

- [ ] **Step 2: Inspect the generated result artifacts**

Run:

```shell
find litmus/results -mindepth 2 -maxdepth 3 -type f -print | sort | tail -20
```

Expected: the new run contains `summary.json`, `report.md`, and one detailed
case JSON. Confirm `summary.json` cost equals the detailed case result cost.

- [ ] **Step 3: Run the zero-cost comparison command**

Run:

```shell
LATEST=$(find litmus/results -mindepth 1 -maxdepth 1 -type d -print | sort | tail -1)
make litmus-compare BASELINE="$LATEST" CURRENT="$LATEST"
```

Expected: a zero-delta comparison with no model invocation.

- [ ] **Step 4: Commit the initial live evidence**

```shell
git add litmus/results
git commit -m "test: record initial litmus probe"
```

## Plan Self-Review

- Spec coverage: Tasks 1-5 implement the dependency-free Go CLI, direct
  production invocation, deterministic checks, cost caps, replay, results, and
  comparison. Task 6 migrates high-signal cases. Task 7 adds the required Make
  interface and documentation. Task 8 records the first durable live result.
- Placeholder scan: all paths, command forms, types, case IDs, data schemas,
  and verification commands are specified.
- Type consistency: `Case`, `CaseResult`, `Run`, `ProviderRequest`, and
  `ProviderResponse` are defined before later tasks use them. The CLI and Make
  targets use the exact `list`, `replay`, `probe`, `batch`, and `compare`
  command names.
