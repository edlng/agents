# Cost-Efficient Litmus Evaluations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Increase Litmus eval signal while keeping routine evaluation single-shot, token-bounded, and mostly replayable.

**Architecture:** Preserve the existing Go CLI and deterministic assertion engine. Add explicit result statuses for provider/grader failures, strengthen selected cases with structured outputs and local validators, and add an opt-in model-grader command that grades captured outputs rather than rerunning subject agents. Replay remains the default; live and model-grader budgets stay separate.

**Tech Stack:** Go standard library, JSON cases/replays, Python subprocess validation where available, existing `claude` CLI.

---

## Scope and Cost Policy

- Default commands perform no model grading.
- No automatic retries or multi-trial execution.
- Existing live cases remain the only default live cases.
- Deterministic replay and unit tests cost `$0`.
- Optional model grading is manual, separately budgeted, output-token constrained by prompt/schema, and cached.
- If a local validator cannot run because its runtime is unavailable, the case
  reports `grader_error` with a `validator_unavailable` reason instead of
  silently passing.

### Task 1: Add explicit result statuses

**Files:**
- Modify: `litmus/internal/litmus/runner.go`
- Modify: `litmus/internal/litmus/results.go`
- Modify: `litmus/cmd/litmus-eval/main.go`
- Test: `litmus/internal/litmus/runner_test.go`
- Test: `litmus/internal/litmus/results_test.go`
- Test: `litmus/cmd/litmus-eval/main_test.go`

- [ ] **Step 1: Add failing status tests**

Add tests covering:

```go
func TestProbeClassifiesProviderDecodeFailureAsInfrastructureError(t *testing.T) {
    // A provider error with no valid result must not be reported as agent_failure.
}

func TestProbeClassifiesAssertionFailureAsAgentFailure(t *testing.T) {
    // A valid provider output whose assertions fail is an agent failure.
}
```

The tests must assert that result persistence records distinct statuses and
that infrastructure failures are excluded from capability pass/fail totals.

- [ ] **Step 2: Run focused tests and verify they fail**

Run:

```bash
go test ./litmus/internal/litmus ./litmus/cmd/litmus-eval -run 'TestProbeClassifies|Test.*Status' -v
```

Expected: FAIL because results currently use only `Passed` plus optional
`ProviderError`.

- [ ] **Step 3: Add status fields and classification**

Add a stable status representation to `CaseResult`:

```go
type CaseStatus string

const (
    StatusPass              CaseStatus = "pass"
    StatusAgentFailure      CaseStatus = "agent_failure"
    StatusInfrastructureErr CaseStatus = "infra_error"
    StatusGraderError        CaseStatus = "grader_error"
)
```

Set status according to this order:

1. invalid assertion configuration or validator setup -> `grader_error`;
2. provider process/response failure -> `infra_error`;
3. valid provider output with failed assertions -> `agent_failure`;
4. all checks passed -> `pass`.

Keep `Passed` for backward-compatible report consumers, but derive it only
from `StatusPass`.

When reading older result artifacts that have no status field, normalize the
status from `ProviderError` and `Passed`: provider errors become `infra_error`,
passed cases become `pass`, and other cases become `agent_failure`.

- [ ] **Step 4: Update summaries and CLI exit behavior**

Add separate totals for passed, agent failures, infrastructure errors, and
grader errors. A run containing an infrastructure error still exits nonzero,
but reports that it was not a capability failure. Do not add automatic retry
behavior.

- [ ] **Step 5: Run focused and full Go tests**

Run:

```bash
go test ./litmus/internal/litmus ./litmus/cmd/litmus-eval -v
go test ./litmus/...
```

Expected: PASS.

### Task 2: Harden structured deterministic assertions

**Files:**
- Modify: `litmus/internal/litmus/runner.go`
- Modify: `litmus/internal/litmus/runner_test.go`
- Modify: `litmus/cases/researcher/valkey-glide-recommendation.json`
- Modify: `litmus/cases/research-validator/classify-findings.json`
- Modify: `litmus/cases/validator/missing-zero-check.json`
- Modify: `litmus/cases/validator/report-only.json`
- Add or modify: `litmus/replays/researcher/valkey-glide-recommendation.json`
- Add or modify: `litmus/replays/research-validator/classify-findings.json`

- [ ] **Step 1: Add failing JSON assertion tests**

Cover nested objects, missing fields, wrong values, malformed JSON, and
additional unexpected output. Keep the assertion deterministic:

```go
func TestEvaluateAssertionsRejectsWrongNestedJSONValue(t *testing.T) {
    output := `{"finding_1":{"classification":"CONFIRMED"}}`
    results := EvaluateAssertions(output, "", []Assertion{
        {Type: "json_path", Path: "finding_1.classification", Value: "UNVERIFIED"},
    })
    if results[0].Passed {
        t.Fatal("wrong nested classification passed")
    }
}
```

- [ ] **Step 2: Run the focused assertion tests**

Run:

```bash
go test ./litmus/internal/litmus -run 'TestEvaluateAssertions' -v
```

Expected: PASS after the test additions; no new assertion type is required
for the nested object shape already supported by the runner.

- [ ] **Step 3: Require structured output in research and validator cases**

Change the case tasks to require compact JSON objects with stable keys.
Examples:

```json
{
  "finding_1": {"classification": "UNVERIFIED"},
  "finding_2": {"classification": "CONTRADICTED"}
}
```

Assertions must check each classification independently. Do not use a
free-floating `contains: CONFIRMED` assertion.

For the researcher case, pin the expected recommendation criteria to a
versioned evidence snapshot or make the task explicitly compare alternatives
without assuming a permanently current vendor recommendation. Require:

- a primary recommendation field;
- an alternative field;
- at least one source URL;
- a short tradeoff field.

- [ ] **Step 4: Add adversarial replay outputs**

Add Go table tests and, where useful, replay cases where:

- the classifications are swapped;
- `CONFIRMED` appears only in prose but the JSON classification is wrong;
- the researcher mentions `valkey-glide` but recommends an unsupported option;
- the output includes a URL but no recommendation or tradeoff.

Every bad output must fail for a specific assertion.

- [ ] **Step 5: Run all replays for the affected cases**

Run:

```bash
go run ./litmus/cmd/litmus-eval replay researcher valkey-glide-recommendation
go run ./litmus/cmd/litmus-eval replay research-validator classify-findings
go run ./litmus/cmd/litmus-eval replay validator missing-zero-check
go run ./litmus/cmd/litmus-eval replay validator report-only
```

Expected: the checked-in good replays pass and the adversarial grader tests
fail as intended.

### Task 3: Add local validators for generated code

**Files:**
- Modify: `litmus/internal/litmus/runner.go`
- Add: `litmus/internal/litmus/validators.go`
- Add: `litmus/internal/litmus/validators_test.go`
- Modify: `litmus/cases/builder/parse-pair.json`
- Modify: `litmus/cases/tester/happy-error.json`
- Modify: `litmus/cases/valkey-glide-implementor/python-batch.json`
- Modify: `litmus/cases/valkey-glide-implementor/injection-resistance.json`
- Add or modify: `litmus/fixtures/parse-pair/*`
- Add or modify: `litmus/fixtures/tester/*`

- [ ] **Step 1: Add failing validator unit tests**

Test validators against:

```go
func TestValidatePythonSyntaxRejectsInvalidSnippet(t *testing.T) {}
func TestValidatePythonTestsRequiresExpectedBehavior(t *testing.T) {}
func TestValidateGeneratedBatchRejectsDeprecatedTransactionAPI(t *testing.T) {}
```

Validators must return a structured result with `pass`, `fail`, or
`validator_unavailable`; the case result maps `validator_unavailable` to
`grader_error` and must never convert it into a pass.

- [ ] **Step 2: Implement bounded local validation**

Implement only validators needed by the selected cases:

- Python syntax validation using `python3 -m py_compile` in a temporary
  workspace;
- tester validation by placing the generated test and target function in a
  temporary fixture, then running the narrow pytest command if pytest exists;
- GLIDE batch static validation for required imports/API calls, forbidden
  transaction APIs, and cleanup/error-handling markers.

Use `exec.CommandContext` with a short timeout, no network access, and a
temporary workspace. Capture stdout/stderr only in the case result.

- [ ] **Step 3: Wire validators into case assertions**

Add a validator assertion type or case-level validator configuration without
changing the existing substring/regex semantics. A validator failure must
produce `agent_failure`; a missing runtime must produce `grader_error` with
the reason `validator_unavailable`.

- [ ] **Step 4: Replace token-only case assertions**

Retain minimal content checks such as a code fence or function name, but make
local correctness checks required for:

- `builder/parse-pair`;
- `tester/happy-error`;
- `valkey-glide-implementor/python-batch`;
- `valkey-glide-implementor/injection-resistance`.

- [ ] **Step 5: Run validator tests and replay affected cases**

Run:

```bash
go test ./litmus/internal/litmus -run 'TestValidate|TestEvaluateAssertions' -v
go run ./litmus/cmd/litmus-eval replay builder parse-pair
go run ./litmus/cmd/litmus-eval replay tester happy-error
go run ./litmus/cmd/litmus-eval replay valkey-glide-implementor python-batch
go run ./litmus/cmd/litmus-eval replay valkey-glide-implementor injection-resistance
```

Expected: valid captured outputs pass; intentionally broken validator inputs
fail.

### Task 4: Tighten remaining catalog smoke tests

**Files:**
- Modify: `litmus/cases/code-reviewer/clean-code-approval.json`
- Modify: `litmus/cases/code-reviewer/eval-exec-injection.json`
- Modify: `litmus/cases/code-reviewer/injection-resistance.json`
- Modify: `litmus/cases/builder/ambiguity.json`
- Modify: `litmus/cases/builder/refuse-delegation.json`
- Modify: `litmus/cases/documenter/required-sections.json`
- Modify: `litmus/cases/context-curator/context-only.json`
- Modify: `litmus/cases/context-curator/research-boundary.json`
- Modify: corresponding files under `litmus/replays/`

- [ ] **Step 1: Add paired negative controls**

Preserve the current cases and use the existing catalog pairs where possible:

- safe review -> approve;
- dangerous review -> block;
- ambiguous builder task -> clarify;
- clear bounded builder task -> proceed (`builder/parse-pair`);
- clean security code -> empty findings;
- vulnerable security code -> concrete findings.

- [ ] **Step 2: Tighten assertions without requiring exact prose**

Use anchored verdict checks and required evidence fields. Do not require one
exact sentence or tool-call sequence. Keep valid alternative solutions
acceptable.

- [ ] **Step 3: Demote duplicate context cases**

Keep one wrapper-format smoke test. Keep the research-boundary case only as a
restraint test unless a seeded-memory fixture is added; document that it does
not measure memory relevance.

- [ ] **Step 4: Add a non-persisting catalog replay test**

Add `TestReplayCatalog` in `litmus/internal/litmus/runner_test.go`. It should
enumerate `litmus/cases`, load each case, call `Replay` directly, and fail if a
checked-in replay does not pass. It must not call `WriteRun`, so verification
does not create result directories.

- [ ] **Step 5: Run the complete catalog**

Run:

```bash
go test ./litmus/internal/litmus -run TestReplayCatalog -v
```

Expected: all checked-in good replays pass without invoking a model.

### Task 5: Add optional, separately budgeted model grading

**Files:**
- Add: `litmus/internal/litmus/model_grader.go`
- Add: `litmus/internal/litmus/model_grader_test.go`
- Modify: `litmus/internal/litmus/runner.go`
- Modify: `litmus/internal/litmus/results.go`
- Modify: `litmus/cmd/litmus-eval/main.go`
- Modify: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Define opt-in grader configuration**

Add optional case metadata:

```json
{
  "model_grader": {
    "enabled": false,
    "model": "claude-haiku-4.5",
    "rubric": "Return JSON with pass, score, and one-sentence reason.",
    "max_budget_usd": 0.03
  }
}
```

No case should invoke this grader unless it is explicitly enabled by both
case configuration and the command.

- [ ] **Step 2: Add cache-key and response-parsing tests**

Cache keys must include:

- captured output hash;
- task/rubric hash;
- grader model;
- grader implementation version.

Require a compact JSON response with no reasoning field. Reject malformed
grader responses as `grader_error`.

- [ ] **Step 3: Implement an explicit grading command**

Add a command such as:

```text
litmus-eval grade <run-directory> --budget <usd> [--jobs 1]
```

It reads existing case outputs, skips cases without opt-in grader metadata,
checks the cache, and invokes the grader only for uncached eligible cases.
Use a separate budget reservation from subject-agent probe budgets. Default
parallelism is `1` to avoid unnecessary concurrent spend.

- [ ] **Step 4: Enforce token and spend limits**

Use the smallest configured grader model, require concise JSON, pass only the
minimum task/rubric/output context, and stop scheduling when the grader budget
is exhausted. Record subject cost and grader cost separately.

- [ ] **Step 5: Test without live grader calls**

Use a fake executor for unit tests. Do not invoke a real model while
implementing or running the normal replay suite. A real grader calibration
run is a separate manual operation.

### Task 6: Documentation and final verification

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-07-16-litmus-cost-efficient-evals-design.md`
- Test: all Litmus Go tests and replay catalog

- [ ] **Step 1: Document the cost tiers**

Document:

- replay: no model call;
- probe: one subject-agent call;
- grade: optional call against captured output;
- separate budgets and cache behavior;
- no automatic retries or multi-trial execution.

- [ ] **Step 2: Run verification**

Run:

```bash
git diff --check
go test ./litmus/...
```

Then run the non-persisting full replay catalog from Task 4. Do not run live probes as part
of routine verification.

- [ ] **Step 3: Inspect final state**

Run:

```bash
git status --short --branch
```

Confirm only intended source, case, replay, and documentation changes exist.
