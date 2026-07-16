# Cost-Efficient Litmus Evaluation Design

## Status

Approved for implementation.

## Goal

Improve Litmus signal quality without increasing routine model spend. Replay
checks and local validators remain the default path; live model calls remain
explicit, single-shot probes for selected cases.

## Non-Goals

- Automatic multi-trial or repeated live execution.
- Expanding the default live core.
- Running an LLM judge for every replay or probe.
- Rebuilding Litmus as a general-purpose evaluation framework.
- Enabling unrestricted agent tools for the whole catalog.

## Design

### 1. Result classification

Separate evaluation outcomes into:

- `pass`: the provider returned a valid response and all graders passed.
- `agent_failure`: the provider returned a valid response and one or more
  graders failed.
- `infra_error`: the provider process or response format failed.
- `grader_error`: a case assertion or local validator was invalid.

Infrastructure and grader errors must not be counted as model capability
failures, although actual provider cost remains recorded.

### 2. Deterministic graders

Keep substring and regular-expression assertions for cheap smoke checks, but
add stronger structured checks where the task has a defined result:

- per-finding classifications for research validation;
- explicit recommendation fields and source requirements for research;
- exact verdict checks for review and validator agents;
- local syntax, test, or static checks for generated code where feasible.

Each strengthened case gets at least one deliberately invalid replay or grader
unit test proving that a plausible bad answer fails.

### 3. Selective model grading

Some outputs, especially research and documentation, cannot be judged fully
with deterministic checks. For those cases, an optional model grader may run
against an already captured output. It must not invoke the subject agent again.

Model graders are cost-controlled by:

- running only for explicitly opted-in cases or manual commands;
- using a separate total grader budget;
- requesting concise structured JSON with no reasoning transcript;
- using the smallest suitable grader model;
- caching results by output hash, rubric version, and grader model;
- skipping the grader when deterministic checks already establish failure.

The default replay and probe commands do not invoke model graders. A separate
calibration command can run a small labeled sample when a rubric changes.
The explicit command is:

```text
litmus-eval grade <run-directory> --budget <usd> [--jobs 1]
```

Only cases with an enabled `model_grader` block are eligible. The block names
the grader model, rubric, per-case budget, and optional output-token limit.
Grading reads the captured output from the run and writes `grader.json` and
`grader.md` beside the original run artifacts. Subject-agent cost and grader
cost are reported separately. The default is serial execution; malformed
grader JSON, provider failures, and output-token violations are
`grader_error`, and are never cached as valid judgments.

### 4. Local outcome validation

The harness will validate generated artifacts locally without another model
call:

- generated Python tests should be executable against the supplied function;
- generated Python snippets should pass syntax validation;
- GLIDE batch cases should demonstrate the required API and reject deprecated
  patterns;
- file and output checks should inspect actual artifacts when a case provides
  them.

Tool-enabled agent workflows are deferred. They may be added later as
explicitly opted-in cases with isolated fixtures.

### 5. Catalog changes

- Strengthen researcher and research-validator cases first.
- Strengthen builder `parse-pair`, tester `happy-error`, and GLIDE batch
  cases with local validation.
- Retain security, code-review, lifecycle, ambiguity, and validator cases as
  targeted checks, tightening broad assertions where practical.
- Treat the duplicate context-curator cases as smoke tests unless they gain
  seeded-memory fixtures and relevance checks.

### 6. Cost controls

- Deterministic replay and grader unit tests must cost `$0`.
- Optional model grading must be separately budgeted and reported.
- Live probes remain opt-in and single-shot.
- No automatic retries for model variability.
- No automatic multi-trial runs.
- Existing live manifests and budget behavior remain unchanged.

## Acceptance Criteria

- A malformed provider response is reported separately from an agent failure.
- The researcher case cannot pass merely by mentioning `valkey-glide`.
- The research-validator case verifies classifications per finding.
- At least the builder, tester, and GLIDE code cases have a local correctness
  check beyond keyword presence.
- Every changed case has a passing replay and a failing grader test or replay.
- Any model grader is opt-in, token-limited, cached, and charged against a
  separate visible budget.
- `go test ./litmus/...` passes.
- The full replay catalog passes without a live model call.
- The worktree contains no generated or unintended changes after verification.
