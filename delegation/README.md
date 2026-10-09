# Delegation: change-review coordinator (Stage 5)

`delegate` reviews a client code change. A coordinator LLM holds only launch
tools. It dispatches scoped sub-agents, reads their validated artifacts,
sends challenged work back for rework, and hands everything to a final
review writer. A human approves or rejects every report.

```
coordinator (launch_* and invoke_* tools only)
 ├─ launch_code_reviewer        → code-review/{security,correctness}   read_file, list_files, search
 ├─ launch_spec_validator       → spec-validation/{tests,criteria}     read_file, list_files, search, run_tests
 ├─ launch_documenter           → documentation/write                  read_file, list_files, search, write_doc
 ├─ launch_adversarial_reviewer → fresh context per round              read_file, list_files, search
 ├─ launch_final_review_writer  → prose only; the harness builds headings
 └─ invoke_human_review      → records a request; cannot approve
```

## Run it

```sh
export ANTHROPIC_API_KEY=...            # the Improving key
go run ./delegation/cmd/delegate run --task delegation/fixtures/endpoint-allowlist --budget 3 \
  --record delegation/runs/endpoint.exchanges.jsonl
go run ./delegation/cmd/delegate status <correlation-id>
go run ./delegation/cmd/delegate decide <correlation-id> --decision approve --reviewer <name>
go run ./delegation/cmd/delegate verify <correlation-id>

# Substance gate demo: a catalog copy with documentation scored toy ends ELEVATED
cp -R delegation/workflows delegation/runs/toy-catalog
sed -i '' 's/"substance": "peripheral"/"substance": "toy"/' delegation/runs/toy-catalog/documentation/manifest.json
go run ./delegation/cmd/delegate run --task delegation/fixtures/endpoint-allowlist --workflows delegation/runs/toy-catalog

# No credentials: re-run the harness from a recorded run
go run ./delegation/cmd/delegate replay --task delegation/fixtures/endpoint-allowlist \
  --exchanges delegation/runs/endpoint.exchanges.jsonl

go test ./delegation/...                 # deterministic, no model calls
```

Each run writes `delegation/runs/<correlation-id>/`: `audit.jsonl`
(hash-chained, fsynced per event), `summary.json` (tokens and cost per
agent), `artifacts/` (every submission: rejected ones as `*.rejected-N.json`, and earlier versions of a relaunched step as `*.v1.json`, `*.v2.json`, ...),
`report.md`, `run.json` (the system record, never edited), and
`decisions.jsonl` (human decisions, append-only).

## How each requirement is met

| Requirement | Mechanism | Evidence |
|---|---|---|
| Dispatch-only coordinator | The coordinator package defines six tools, all `launch_` or `invoke_`, and reaches sub-agents only through its own `Dispatcher` interface. It cannot import tools, sub-agent, dispatch, the SDK client, `os/exec`, or `net/http`. | `coordinator/manifest.json`, `internal/coordinator/imports_test.go` |
| Sub-agents scoped to one workflow | Each workflow manifest declares its tools; a step may narrow them and cannot widen them. Tools are rooted at the repo, refuse path escapes, and never write to it: `run_tests` runs a fixed command on a copy, `write_doc` writes only `docs/*.md` to the run output. | `workflows/*/manifest.json`, `internal/catalog`, `internal/tools/tools_test.go` |
| Stage 3 guarantees per workflow | Every workflow (and the coordinator) has a prompt, at least 3 named criteria, and a committed `results.json` with per-trial outcomes. | `evals/*/results.json`, `evals/cmd/evalrun` |
| Real-world measurement | Code review is also scored against 17 human-reviewed rounds of `aws/developer-toolkit-elasticache`, stored as links with pinned commits and diff hashes, with each labeled defect linked to its review comment. 10 rounds were used for tuning and 7 held out. Through the API (runtime path), held out: verdict 12/14, recall of human-flagged defects 11/16; tuning: 11/16 and 9/24. | `corpus/`, `evals/code-review/results.json` (`splits`), `ITERATION-LOG.md` |
| Adversarial review in its own context | Every review round is a new message list (`initial_messages: 1` in the audit), with `read_file` to check claims. Manifests declare `isolated_context: true`. One reviewer covers all workflows by assignment. It challenges with counter-evidence, catches missed and false findings, detects unsupported PASS verdicts, and the harness refuses the final review until a reworked artifact is reviewed again. | `internal/subagent`, `workflows/adversarial-review/` |
| Final review writer | Gets every artifact, every review round, and the coordinator's dispositions. A validator rejects prose that contains a heading or cites a finding or challenge ID no agent raised. Headings and verdict labels come from artifacts in `internal/report`; an open critical challenge prints `UNRESOLVED`. | `internal/schema` (`final-review.v1`), `internal/report` |
| Deterministic guardrails between steps | A sub-agent submits through its `submit_*` tool. The harness checks the schema and the evidence (cited file and line exist, reported test exit code matches the recorded run, criteria IDs match the task, docs have the required sections) before the coordinator sees anything. A failure returns `{"ok":false,"error":{"code":"schema_violation",...}}`. | `internal/schema`, `internal/subagent` |
| Custom harness | The runtime calls the Messages API through the Go SDK. The Claude CLI is used only under `evals/` (eval runner and dev tools), and a test fails if `cmd/delegate` can reach it. | `internal/provider/anthropic`, `cmd/delegate/imports_test.go` |
| Punch-outs honored | Runs end `PENDING_HUMAN` (or `ELEVATED`). Only `delegate decide` changes that. Each decision names the reviewer, binds the report's SHA-256, and is GPG-signed with the reviewer's key (the key git signs commits with); unsigned decisions are refused and `delegate verify` checks every signature. The coordinator has no tool that writes a decision. | `internal/gate`, `internal/harness/harness_test.go` |
| Substance gate | Any `toy` workflow or two or more `peripheral` ones elevate the run; `continue` then `approve` are both required. Every workflow is scored, the reviewer and writer included: code-review, spec-validation, adversarial-review, and final-review are core, documentation is peripheral, so this catalog does not elevate. A test marks documentation `toy` and checks the gate. | `internal/gate`, `TestSubstanceGateElevates` |
| Audit trail | One correlation ID per run. Every model call records model, input/output/cache tokens, cost, duration, and response ID, with parent spans linking each sub-agent to the coordinator call that launched it. Decisions append to the same chain. `delegate verify` re-checks it from disk. | `internal/audit` |
| Launch limits | 3 launches per step, 14 per run, a USD budget, and no final review while any artifact's current version lacks a review. Refusals come back to the coordinator as structured results. | `internal/dispatch` |

## Layout

| Path | Contents |
|---|---|
| `cmd/delegate` | runtime CLI: run, replay, status, decide, verify, smoke |
| `coordinator/` | coordinator manifest and prompt |
| `workflows/` | workflow manifests, prompts, shared trust model |
| `fixtures/` | client changes: base tree, post-change repo, patch, task spec |
| `internal/` | provider, audit, tools, schema, subagent, catalog, task, dispatch, coordinator, gate, report, harness |
| `evals/` | eval runner, CLI dev provider, dev runners, `results.json` per workflow |
| `corpus/` | links to real review rounds with human labels, fetched on demand |
| `ITERATION-LOG.md` | what broke, what changed, and what each change measured |
