# Stage 5 plan: change-review coordinator

## Goal

Build a coordinator that reviews a code change for a client. The input is a
repository path and a task specification. The coordinator dispatches scoped
sub-agents, reasons over their validated results, sends challenged artifacts
back for rework, and hands everything to a final review writer. A human
approves or rejects the result. The coordinator holds launch tools only.

## What changes from Stage 4

| Stage 4 | Stage 5 |
| --- | --- |
| `team-lead` ran inside the `claude` CLI (`runner.go:898`) | New Go harness calls the model API over HTTP. No CLI at runtime. |
| Deterministic controller sequenced the steps | An LLM coordinator decides the order and the rework loops |
| Validator re-ran as `review-challenge` with shared inputs | Separate adversarial reviewer, fresh API call per review, `read_file` access |
| Documenter output went to the human gate | Final review writer synthesizes everything, and the harness owns the headings |
| One workflow | Three scoped workflows plus reviewer and writer |

Stage 4 pieces that carry over are the guardrail validators
(`src/runtime.mjs`), the hash-chained audit and sentinel format
(`workflow_runner.go`, `workflow_evidence.go`), the human gate states
(`PENDING_HUMAN`, `REJECTED_HUMAN`, `COMPLETE`), and the agent prompts. The
documenter prompt now exists only in the Stage 4 zip
(`source/agents/documenter/claude.md`), so it gets copied back.

## Provider

The runtime cannot use the `claude` or `codex` CLI. Options:

1. Anthropic Messages API with the Improving key, through the official Go SDK
   (`anthropic-sdk-go`). The course bills this to your AI usage. Each response
   includes input, output, and cache token counts, and cost comes from the
   price table in `internal/provider/provider.go`.
2. Bedrock Converse through `aws-sdk-go-v2/service/bedrockruntime`. This
   machine already has `CLAUDE_CODE_USE_BEDROCK` and AWS profiles set, so it
   works without a new key. Check first whose account pays.
3. A replay provider that serves recorded responses. Unit tests and the
   deterministic e2e use it, so they cost nothing.

All three implement one interface:

```go
type Provider interface {
    Complete(ctx context.Context, req Request) (Response, error) // Response carries Usage{Model, InputTokens, OutputTokens, CostUSD, ResponseID, DurationMS}
}
```

The CLIs can still appear under `delegation/evals/` (for example as an LLM
judge for tone), because eval usage is exempt.

## Layout

All code lives in the existing Go module (`go.mod` at the repo root).

```
delegation/
  cmd/delegate/main.go          run | decide | replay
  internal/provider/            anthropic.go, replay.go (record and replay)
  internal/coordinator/         LLM loop, launch tool definitions, limits
  internal/subagent/            fresh-context tool loop, output validation
  internal/tools/               read_file, list_files, search, run_tests, write_doc
  internal/schema/              JSON schemas and validators per output type
  internal/audit/               JSONL writer, hash chain, correlation IDs
  internal/report/              deterministic heading assembly
  internal/gate/                substance gate, human decision log
  workflows/
    code-review/       manifest.json, prompt.md
    spec-validation/   manifest.json, prompt.md
    documentation/     manifest.json, prompt.md
    adversarial-review/manifest.json, prompt.md
    final-review/      manifest.json, prompt.md
  coordinator/manifest.json, prompt.md
  fixtures/                     seeded repos plus task specs
  evals/                        cases, runner, results.json per workflow
  evals/cliprovider/            `claude -p --safe-mode --tools ""` dev provider
runs/<correlation_id>/          audit.jsonl, artifacts/, report.md, decisions.jsonl
```

## Coordinator

Tools (the manifest declares exactly these):

| Tool | Arguments |
| --- | --- |
| `launch_code_reviewer` | `step`, `focus_files?`, `prior_challenges?` |
| `launch_spec_validator` | `step`, `criteria_ids?`, `prior_challenges?` |
| `launch_documenter` | `step`, `prior_challenges?` |
| `launch_adversarial_reviewer` | `artifact_ids[]` |
| `launch_final_review_writer` | `disposition` (per artifact: `accepted`, `revised`, `unresolved`, plus a reason) |
| `request_human_decision` | `reason` |

The coordinator sees the task spec, the workflow catalog (names, purposes,
steps), and tool results. It never sees file contents unless a sub-agent quotes
them. When the adversarial reviewer raises a `critical` challenge, the prompt
tells the coordinator to re-launch the producing workflow at the challenged
step with the challenges attached, then re-review. The harness does not encode
that rule. The coordinator's reasoning does.

Harness limits. Each one returns a structured error the coordinator can
reason over:

- at most 3 launches per artifact and 12 launches per run
- a per-run budget in USD (`--budget`) checked before each launch
- `launch_final_review_writer` refused until every produced artifact has an
  adversarial review
- a final disposition refused while the substance gate is open

Dispatch-only proof:

- `coordinator/manifest.json` lists the tools above and nothing else.
- A Go test fails if any coordinator tool name lacks a `launch_` or
  `request_` prefix.
- A Go test runs `go list -deps ./delegation/internal/coordinator` and fails
  if the package imports `os/exec`, `net/http`, or `internal/tools`. The
  coordinator reaches the provider only through an interface injected by
  `main.go`.

## Sub-agents

Each launch builds a new message array from the workflow prompt and the task
input. No sub-agent receives another agent's messages. Each sub-agent gets
only the tools in its manifest, rooted at the target repo with path traversal
refused.

| Workflow | Agent source | Tools | Output schema | Substance |
| --- | --- | --- | --- | --- |
| code-review | `agents/code-reviewer` | read_file, list_files, search | `review.v1`: verdict APPROVE or BLOCK, findings with file, line, severity, evidence, remediation | core |
| spec-validation | `agents/validator` | read_file, list_files, run_tests (allowlisted command from the task spec, timeout, no network) | `validation.v1`: PASS or FAIL per acceptance criterion with evidence | core |
| documentation | Stage 4 documenter | read_file, list_files, write_doc (scratch copy, `docs/` only) | `documentation.v1`: Overview, Usage, Limitations, files written | peripheral |
| adversarial-review | new prompt | read_file, list_files, search | `challenge.v1`: per artifact verdict, challenges with severity, claim, counter-evidence, file, line | n/a (reviewer) |
| final-review | new prompt | none | non-empty markdown, one prose block per artifact, no headings | core |

The adversarial reviewer manifest sets `isolated_context: true`, and the
audit log records a distinct `response_id` and an empty prior-message count
for every review. One reviewer covers all three workflows. That is scoping by
review assignment, which the rubric allows.

Schema validation runs inside `subagent` before anything returns to the
coordinator. A bad output returns
`{"ok": false, "error": "schema_violation", "details": [...]}` and the raw
text goes to the audit log only.

## Final report

`internal/report` builds the structure from artifact verdicts:

```
# Change review: <task id>
## code-review: PASS        <- from review.v1 verdict, never from the LLM
<writer prose for code-review>
## spec-validation: FAIL
<writer prose>
...
## Disposition
<table from the coordinator's disposition argument>
## Human decision
PENDING_HUMAN | COMPLETE | REJECTED_HUMAN
```

A deterministic check rejects writer prose that contains a markdown heading or
names a finding ID that does not appear in the examiner or reviewer
artifacts. That check enforces "no new findings".

## Human checkpoints

- Every run ends `PENDING_HUMAN`. Only `delegate decide --correlation-id X
  --decision approve|reject --reviewer <name>` moves it, and it appends to
  `decisions.jsonl`. The system report stays verbatim.
- Substance gate: if any workflow is `toy` or two or more are `peripheral`,
  the harness blocks final disposition and the decision changes to
  `continue|reject`. The real catalog has 0 toy and 1 peripheral, so it does
  not elevate. A fixture catalog that marks documentation `toy` proves the
  gate fires.
- The coordinator has no tool that writes a decision. `request_human_decision`
  only records a request.

## Audit trail

`runs/<correlation_id>/audit.jsonl` is written with fsync after each event and
hash-chained like Stage 4. Each event has `correlation_id`, `span_id`,
`parent_span_id`, `agent`, `workflow`, `step`, `model`, `input_tokens`,
`output_tokens`, `cost_usd`, `duration_ms`, `response_id`, and
`schema_result`. A `summary.json` rolls up cost and tokens per agent.
`delegate replay <correlation_id>` re-verifies the chain from disk.

## Evals (Stage 3 guarantees per workflow)

Each workflow keeps its prompt, at least 3 named criteria, and a committed
`evals/<workflow>/results.json` from a live run.

| Workflow | Criteria |
| --- | --- |
| code-review | blocks seeded injection (`eval-exec-injection`), approves clean code (`clean-code-approval`), ignores instructions in comments (`injection-resistance`), every finding cites a real file and line |
| spec-validation | fails a missing acceptance criterion (`missing-zero-check`), passes a compliant change, never writes files (`report-only`) |
| documentation | has the required sections, every documented API exists in the code, writes only under `docs/` |
| adversarial-review | challenges a planted false claim, raises no challenge on an accurate draft, cites counter-evidence that exists in the repo |
| final-review | no headings and no new finding IDs, covers every challenge and disposition, tone passes a judge rubric (judge may use the CLI under evals/) |
| coordinator | re-launches the challenged workflow at the challenged step, stops at the launch cap, does not finalize while the substance gate is open, ends `PENDING_HUMAN` |

Case material comes from the Stage 3 cases and the Stage 4 live corpus
fixture (`evidence/live/corpus/fixture`: go, node, python, docs).

## Milestones

Each milestone ends with `go test ./delegation/...` green and a commit.

1. Provider interface, replay provider, Anthropic provider, CLI dev
   provider, audit writer. One live smoke run recorded with tokens and cost.
   (Done 2026-10-05: CLI smoke on Sonnet 5.5, 2 calls, $0.0081.)
2. Sandboxed tools and the sub-agent loop with schema validation. Run
   code-review alone against one fixture. (Done: security step BLOCKed the
   seeded SSRF on the first CLI run, $0.03.)
3. Port the code-review, spec-validation, and documentation prompts and
   manifests. (Done: steps are code-review/security, code-review/correctness,
   spec-validation/tests, spec-validation/criteria, documentation/write.)
4. Adversarial reviewer, final review writer, report assembly, and the
   no-new-findings check. (Done.)
5. Coordinator loop, limits, and the dispatch-only tests. (Done: the
   provider SDK moved to provider/anthropic so the coordinator's import
   graph excludes net/http.)
6. Substance gate, `delegate decide`, decision log. (Done: ELEVATED needs
   continue, then approve; decisions bind the report hash and append to the
   audit chain.)
7. Evals for all six components, then live runs and committed `results.json`.
   (Done: 23 cases x 3 trials, every criterion passed, $3.15 via the CLI.
   Full live CLI run 20261005T164931Z-c2cf6f0e ended PENDING_HUMAN for $0.43.)
8. Package `certification/stage5/`: README mapping each rubric card to a
   file, a sample run directory with the full audit, `git log` export, and a
   replay e2e that needs no credentials.

## Rubric coverage

| Card | Evidence |
| --- | --- |
| Dispatch-only coordinator | `coordinator/manifest.json`, import and prefix tests |
| Sub-agents scoped to one workflow | `workflows/*/manifest.json`, tool root tests, `evals/*/results.json` |
| Adversarial review in its own context | `isolated_context: true`, per-review `response_id` in audit, file-access tools |
| Version-controlled repository | commit per milestone, `git log` export |
| Stage 4 continuity | API harness, `PENDING_HUMAN` default, substance gate, `audit.jsonl` per correlation ID |
| The counterfeit | the import test proves the coordinator cannot reach tools |

## Open items

- Pick the provider (Improving key or Bedrock) before milestone 1.
- Pick the coordinator and sub-agent models and set a total eval budget.
