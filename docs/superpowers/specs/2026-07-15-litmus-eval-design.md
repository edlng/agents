# Litmus Eval Design

## Status

This design defines the manual production evaluation path.

## Goal

Replace the manual Promptfoo path with `litmus-eval`, a small Go CLI for
relevant, deterministic agent evaluations with budget-aware live probes.

Litmus Eval runs only selected cases, invokes the existing production agent
prompt and model directly, and writes durable results suitable for later
visualization.

## Non-Goals

- Reimplement Promptfoo's full feature set.
- Add an LLM judge to normal evaluation runs.
- Change an agent's production prompt or model merely to lower evaluation
  spend.
- Run evaluations automatically or on a daily schedule.
- Add third-party Go modules in the initial implementation.

## Architecture

The implementation contains three Go source files under `litmus/`:

```text
litmus/cmd/litmus-eval/main.go       # Command parsing, help, and exit codes.
litmus/internal/litmus/runner.go     # Cases, direct invocation, budgets, and checks.
litmus/internal/litmus/results.go    # Result persistence, reading, comparison, and reports.
```

Git revision metadata is optional and is collected inside the runner. A
separate `git.go` module is not needed.

The CLI invokes `claude -p` directly, using the same agent prompt resolution,
model normalization, fixture injection, and JSON usage parsing as the current
evaluation provider. Live probes disable Claude tools and run in an isolated
temporary workspace, so the default path measures prompt behavior without
side effects or tool-loop spend. The runner does not invoke Promptfoo and does
not invoke an LLM grader.

All Go commands, including development and Make targets, set
`GOPROXY=direct`. The first version uses only the Go standard library.

## Repository Layout

```text
litmus/
  cmd/
    litmus-eval/
  internal/
    litmus/
  cases/
    <agent>/
      <case>.json
  fixtures/
    <fixture-name>/
  replays/
    <agent>/
      <case>.json
  results/
    <timestamp>-<revision>/
      summary.json
      report.md
      cases/
        <agent>--<case>.json
```

`cases`, `fixtures`, and `replays` are versioned test inputs.
`results` is also versioned and must not be added to `.gitignore`.

## Case and Fixture Model

Each pretty-printed JSON case defines:

- Stable case ID and agent name.
- Task supplied to the production agent.
- Maximum subject-agent budget.
- Explicit live eligibility. Cases are replay-only unless `live` is `true`.
- Optional fixture directory.
- Deterministic assertions.

The first assertion set supports substring, regular expression, JSON primitive
path, and fixture file-content checks. Cases that need a workspace execute in
a temporary copy of their fixture. Live probes do not receive Claude tools, so
the initial catalog is limited to response behavior and pre-existing fixture
state. External services are represented by fixture-local stubs.

## Commands

```shell
GOPROXY=direct go run ./litmus/cmd/litmus-eval list
GOPROXY=direct go run ./litmus/cmd/litmus-eval replay <agent> <case>
GOPROXY=direct go run ./litmus/cmd/litmus-eval probe <agent> <case> --budget 0.10
GOPROXY=direct go run ./litmus/cmd/litmus-eval batch <manifest> --budget 0.80 [--include-replay-only] [--jobs <count>]
GOPROXY=direct go run ./litmus/cmd/litmus-eval compare <baseline-run> <current-run>
```

`replay` evaluates a versioned captured output without a model call.
`probe` runs one fresh production invocation only for a live-enabled case.
`batch` schedules live-enabled cases from its manifest until the global budget
is exhausted. `--include-replay-only` explicitly promotes replay-only cases
for one batch invocation without changing their stored eligibility. Batches
run up to three cases concurrently by default; `--jobs 1` restores serial
execution.
`compare` reads two existing result directories and performs no model call.

## Cost Controls

Each batch reserves a case's lower selected-case limit or remaining budget
before starting it, and releases that reservation when the case completes.
The runner records actual provider cost immediately after each call and does
not schedule another case once the global limit is exhausted. Claude CLI's
dollar limit is advisory, so reservations and the configured amount are
scheduling targets rather than hard spend guarantees. Concurrent in-flight
probes can collectively exceed the recorded budget. Inspect recorded actual
cost after every live call.

The default operating tiers are:

| Tier | Operation | Target cost |
| --- | --- | --- |
| Replay | Verify checks against captured output | $0 |
| Probe | One fresh production case | $0.05-$0.10 |
| Batch | Selected, change-scoped production cases | $0.30-$0.80 |

The default path uses deterministic checks only. A model-based judgment may be
added later as an explicitly budgeted, separately recorded command.

The complete deterministic catalog is replay-only by default. The initial
live core contains only two observed low-cost code-reviewer checks. A case must
be explicitly promoted to `live: true` before `probe` or `batch` can invoke
the provider.

## Durable Results

Every fresh run creates one versioned result directory named with UTC timestamp
and the available short Git revision. Results are pretty-printed JSON.

`summary.json` contains stable data for later machine consumption:

- Run ID, timestamp, Git revision, and configured budget.
- Total actual cost, token counts, duration, and passed/failed counts.
- A compact entry for each executed case and its detailed result path.

Each case result contains:

- Agent, case ID, production model, and prompt-content hash.
- Raw agent output.
- Deterministic assertion results and final status.
- Input/output tokens, actual cost, duration, and provider error data.

`report.md` provides a concise human-readable view of the same run.

## Makefile Interface

The implementation adds wrappers to the existing Makefile:

```make
litmus-list:
	GOPROXY=direct go run ./litmus/cmd/litmus-eval list

litmus-replay:
	GOPROXY=direct go run ./litmus/cmd/litmus-eval replay "$(AGENT)" "$(CASE)"

litmus-probe:
	GOPROXY=direct go run ./litmus/cmd/litmus-eval probe "$(AGENT)" "$(CASE)" --budget "$(BUDGET)"

litmus-batch:
	GOPROXY=direct go run ./litmus/cmd/litmus-eval batch "$(MANIFEST)" --budget "$(BUDGET)" $(BATCH_ARGS)

litmus-compare:
	GOPROXY=direct go run ./litmus/cmd/litmus-eval compare "$(BASELINE)" "$(CURRENT)"
```

Each target validates its required variables and reports usage before starting
a model invocation. Defaults are defined in the Go CLI rather than hidden in
the Makefile.

## Migration

Litmus Eval replaces the previous Promptfoo path. Expand the catalog only when
a case is proven useful enough to spend a production-model call on it.
