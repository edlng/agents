# Manual Evaluation Design

## Goal

Provide useful, relevant agent-evaluation feedback with manual invocation only.
The normal run must stay below $1 total cost. A $1.50 run is an exception for
investigating a suspected regression.

## Scope

This design changes the evaluation workflow, not the agents being evaluated.
It keeps Promptfoo as the runner and the existing `claude -p` provider.

## Evaluation Tiers

### Quick Check

Use after a narrow prompt or agent edit.

- Select only the changed agent's two or three highest-signal deterministic
  tests.
- Use the production model and bypass Promptfoo's response cache.
- Run each selected test once.
- Limit each subject-agent call to $0.12.
- Target total cost: $0.30 to $0.50.

### Confidence Check

Use before sharing a meaningful prompt, tool-policy, or model change.

- Start with the Quick Check selection.
- Run every selected test twice.
- Add one relevant boundary or refusal test.
- Limit each subject-agent call to $0.12.
- Target total cost: $0.70 to $0.90.

### Failure Diagnosis

Use only after a surprising failure.

- Rerun the failed test two or three times.
- Compare with the same test against the merge-base prompt when the failure
  may be pre-existing.
- Do not rerun unrelated tests.
- Target incremental cost: $0.20 to $0.40.

## Test Selection and Design

Every retained evaluation must be traceable to one of:

1. A real previously observed failure.
2. A high-impact safety or role boundary.
3. A core agent job-to-be-done.

Each agent should have one successful core-workflow case and one boundary case.
Tests must use a small realistic fixture when workspace or tool use matters:
two or three files, controlled external dependencies, and deterministic
postconditions. Prefer assertions on changed files, parsed code, command
results, or recorded tool arguments over output-string matching.

LLM-as-judge assertions are excluded from Quick and Confidence checks. They
may be used only for a one-off manual adjudication after their provider cost is
both capped and recorded.

## Cost and Caching

- Prompt-dependent runs must bypass the Promptfoo response cache. Cached
  outputs are permitted only when the subject prompt and test are unchanged.
- The default suite must have a global $0.90 ceiling, not just per-call caps.
  It must stop before exceeding the ceiling and report the unrun tests.
- Subject-agent and grader usage must be recorded in one run record. Existing
  token metrics do not currently include grader usage.

## Manual Result History

Each Confidence Check records one entry per attempt containing:

- Git commit and merge base.
- Agent, test ID, production model, and prompt-content hash.
- Fixture version.
- Pass/fail result, deterministic assertion details, spend, and timestamp.

This history means "last manually verified," not a claim of statistical
stability. A test becomes trusted only after repeated successful Confidence
Checks across independent relevant changes.

## Command Interface

The implementation will expose explicit commands for:

- A cache-bypassing Quick Check for a selected agent.
- A cache-bypassing Confidence Check for a selected agent.
- Failure diagnosis for a selected test, optionally against merge base.
- Viewing the local manual history and cumulative spend.

Commands must fail clearly when the selected agent has no curated tests, the
provider cannot return cost data, or the configured budget is insufficient for
the requested selection.

## Non-Goals

- Daily or scheduled evaluation runs.
- A full replacement of Promptfoo.
- Broad model matrices on every manual run.
- Promoting a test to "always passing" from one-off results.
