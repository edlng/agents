# Litmus Catalog Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate Litmus implementation and data under `litmus/`, restore the deterministic coverage lost in the Promptfoo migration, and prevent unapproved cases from issuing live provider calls.

**Architecture:** Keep the repository Go module at its current root, but move the Litmus command and internal package below `litmus/` alongside cases, replays, fixtures, manifests, and committed results. Add an explicit `live` case flag: only the two proven code-reviewer cases are live-enabled and referenced by `core.json`; all remaining migrated checks are replay-only.

**Tech Stack:** Go standard library, JSON cases/replays, Make, existing agent prompt files. All Go commands use `GOPROXY=direct`. No model invocation is part of this migration.

---

### Task 1: Establish replay-only live-call protection

**Files:**
- Modify: `internal/litmus/runner.go`
- Modify: `internal/litmus/runner_test.go`
- Modify: `litmus/cases/code-reviewer/eval-exec-injection.json`
- Modify: `litmus/cases/code-reviewer/clean-code-approval.json`

- [ ] **Step 1: Write the failing live-case gate test**

Add a test that passes a structurally valid `Case` with `Live: false` to a
fake executor and asserts that `Probe` returns an error containing
`"not enabled for live probes"` without invoking the executor.

- [ ] **Step 2: Run the focused test**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run '^TestProbeRejectsReplayOnlyCase$' -v
```

Expected: FAIL because the `Case` type does not have a `Live` field and `Probe`
does not gate live execution.

- [ ] **Step 3: Add the minimal gate**

Add `Live bool \`json:"live"\`` to `Case`, reject `!testCase.Live` near the
start of `Runner.Probe`, and set `"live": true` only on:

```text
code-reviewer/eval-exec-injection
code-reviewer/clean-code-approval
```

Set `Live: true` in unit-test cases that exercise successful provider calls.

- [ ] **Step 4: Verify the focused regression test**

Run:

```shell
GOPROXY=direct go test ./internal/litmus -run '^TestProbeRejectsReplayOnlyCase$' -v
```

Expected: PASS and the fake executor records no request.

### Task 2: Move runtime code under `litmus/`

**Files:**
- Move: `cmd/litmus-eval/` to `litmus/cmd/litmus-eval/`
- Move: `internal/litmus/` to `litmus/internal/litmus/`
- Modify: `litmus/cmd/litmus-eval/main.go`
- Modify: `litmus/internal/litmus/runner_test.go`
- Modify: `Makefile`

- [ ] **Step 1: Move command and package directories**

Move both Go directories without changing their package names. Update the
command import to:

```go
"github.com/edlng/agents/litmus-eval/litmus/internal/litmus"
```

Update the repository-root test helper to walk three parent directories from
`litmus/internal/litmus`.

- [ ] **Step 2: Update Make targets**

Change each Litmus target to invoke:

```make
GOPROXY=direct go run ./litmus/cmd/litmus-eval <subcommand>
```

- [ ] **Step 3: Verify the moved packages**

Run:

```shell
GOPROXY=direct go test ./litmus/cmd/litmus-eval ./litmus/internal/litmus -v
GOPROXY=direct go run ./litmus/cmd/litmus-eval list
```

Expected: both packages pass and `list` prints the current case IDs.

### Task 3: Port the full deterministic Promptfoo catalog

**Files:**
- Create: `litmus/cases/builder/injection-resistance.json`
- Create: `litmus/cases/builder/ambiguity.json`
- Create: `litmus/cases/code-reviewer/injection-resistance.json`
- Create: `litmus/cases/context-curator/research-boundary.json`
- Create: `litmus/cases/context-curator/context-only.json`
- Create: `litmus/cases/documenter/required-sections.json`
- Create: `litmus/cases/glide-code-reviewer/client-lifecycle.json`
- Create: `litmus/cases/researcher/valkey-glide-recommendation.json`
- Create: `litmus/cases/research-validator/classify-findings.json`
- Create: `litmus/cases/security-reviewer/sql-ssrf.json`
- Create: `litmus/cases/security-reviewer/clean-code.json`
- Create: `litmus/cases/tester/happy-error.json`
- Create: `litmus/cases/validator/missing-zero-check.json`
- Create: `litmus/cases/validator/report-only.json`
- Create: `litmus/cases/valkey-glide-implementor/python-batch.json`
- Create: `litmus/cases/valkey-glide-implementor/injection-resistance.json`
- Create: matching files under `litmus/replays/<agent>/`
- Modify: the four existing case JSON files to add `"live": false` where absent

- [ ] **Step 1: Encode the deterministic requirements**

Port the task text and deterministic assertions from the former
`promptfooconfig.yaml`. Use only Litmus-supported `contains`, `regex`, and
`not_regex` assertions; omit every `llm-rubric` assertion and score weight.
Set `max_budget_usd` to `0.10` and `"live": false` for every new case.

- [ ] **Step 2: Add replay samples**

For each case, create one concise representative output that satisfies its
assertions. These samples validate the deterministic case encoding at zero
cost; they are not substituted for fresh live-agent evidence.

- [ ] **Step 3: Add catalog coverage test**

Extend the manifest/catalog test to assert `listCases` returns exactly these
20 `(agent, case)` identities:

```text
builder/ambiguity
builder/injection-resistance
builder/parse-pair
builder/refuse-delegation
code-reviewer/clean-code-approval
code-reviewer/eval-exec-injection
code-reviewer/injection-resistance
context-curator/context-only
context-curator/research-boundary
documenter/required-sections
glide-code-reviewer/client-lifecycle
researcher/valkey-glide-recommendation
research-validator/classify-findings
security-reviewer/clean-code
security-reviewer/sql-ssrf
tester/happy-error
validator/missing-zero-check
validator/report-only
valkey-glide-implementor/injection-resistance
valkey-glide-implementor/python-batch
```

- [ ] **Step 4: Verify catalog and replays**

Run:

```shell
GOPROXY=direct go test ./litmus/cmd/litmus-eval ./litmus/internal/litmus -v
```

Then run `litmus-eval replay` once for each catalog identity. Expected: all
20 replays pass with `$0.00` cost.

### Task 4: Restrict the live core and update documentation

**Files:**
- Modify: `litmus/manifests/core.json`
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-07-15-litmus-eval-design.md`

- [ ] **Step 1: Restrict the core manifest**

Set `core.json` to only:

```json
{
  "cases": [
    {"agent": "code-reviewer", "case": "eval-exec-injection"},
    {"agent": "code-reviewer", "case": "clean-code-approval"}
  ]
}
```

- [ ] **Step 2: Document the safety model**

Document that the full catalog is replay-only by default, only cases with
`"live": true` can be probed or batched, and the provider dollar argument is
advisory. State that a native provider API with token caps is required before
claiming a strict spend ceiling.

- [ ] **Step 3: Verify the final interface**

Run:

```shell
GOPROXY=direct go test ./litmus/cmd/litmus-eval ./litmus/internal/litmus -v
make litmus-list
make litmus-replay AGENT=builder CASE=refuse-delegation
```

Expected: tests pass, the catalog lists 20 cases, and the builder boundary
replay passes at `$0.00`.

### Task 5: Remove obsolete evaluation files and validate workspace state

**Files:**
- Delete: `evals/`
- Delete: `promptfooconfig.yaml`
- Delete: `promptfooconfig.smoke.yaml`
- Delete: `run-eval.sh`
- Modify: `.gitignore`
- Modify: `package.json`
- Modify: `package-lock.json`

- [ ] **Step 1: Retain only Litmus evaluation assets**

Keep the existing deletion of Promptfoo provider, grader, scoring, and
configuration files. Keep `litmus/results/` tracked, including failed live
artifacts, because each represents a durable record of actual provider
behavior and cost.

- [ ] **Step 2: Confirm no Promptfoo reference remains**

Run:

```shell
git grep -n -i 'promptfoo' -- ':!docs/superpowers/plans/'
```

Expected: no output.

- [ ] **Step 3: Final verification**

Run:

```shell
git diff --check
GOPROXY=direct go test ./litmus/cmd/litmus-eval ./litmus/internal/litmus
```

Expected: both commands exit successfully. No commit is created without
explicit user approval.
