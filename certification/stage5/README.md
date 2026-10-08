# Stage 5 submission: change-review coordinator

`delegate` reviews a client code change. A coordinator LLM holds only
dispatch tools. It launches scoped sub-agents, reads their validated
artifacts, sends challenged work back for rework, and hands everything to a
final review writer. A human approves or rejects every report.

All paths below are relative to the root of this zip.

| Path | Contents |
|---|---|
| `source/` | The full Go source: `source/delegation/` plus `go.mod` and `go.sum`. `source/delegation/README.md` explains how to run it. |
| `evidence/runs/` | Live runs through the Messages API, each with `audit.jsonl`, `summary.json`, `artifacts/`, `report.md`, `run.json`, and `decisions.jsonl`. `evidence/runs/INDEX.md` lists each run's status, review rounds, rework launches, and human decisions. |
| `evidence/evals/` | One `results.json` per workflow and for the coordinator, measured through the API. |
| `evidence/recordings/` | Recorded model exchanges. `delegate replay` re-runs the harness from them with no credentials. |
| `evidence/git-log.txt` | `git log --stat` for `delegation/`. |
| `evidence/go-test.txt` | `go test ./delegation/...` output from this package's source. |
| `evidence/reviewer-keys.asc` | Public GPG keys of the reviewers who signed decisions, so `delegate verify` can check the signatures. |

## Sample runs

`evidence/runs/INDEX.md` has the run IDs. All four runs predate the
`tool_result` audit event, which the harness added on 2026-10-08 after the
guardrail-stop run below. The runs show four paths:

1. A clean review. The adversarial reviewer upholds every artifact, the
   coordinator does no rework, and the run ends `PENDING_HUMAN`. A signed
   `approve` moves it to `COMPLETE`.
2. A rework loop with one planted mistake. Accurate sub-agents rarely draw a
   blocking challenge: in 7 API runs on 2026-10-07 the reviewer raised only
   minor ones, which the coordinator correctly left without rework. So
   `evals/cmd/faultrun` replaces the first `code-review/security` submission
   with a false APPROVE, without a model call. Its `llm_call` event shows
   model `fault-injected`, and `run.json` names the injection in `provider`.
   Every other call goes to the API. The reviewer challenges the false
   APPROVE, and the coordinator relaunches that step with the challenge
   attached (`prior_challenges` in the `tool_call` event in `audit.jsonl`).
   It then sends only that artifact to a second review round
   (`artifacts/adversarial-review/round-2.json`). The planted APPROVE is kept
   as `artifacts/code-review/security.v1.json`, and the re-run BLOCK is
   `security.json`.
3. An elevated run. The `Workflows` column shows it loaded a copy of the
   catalog with documentation scored `toy`, made with
   `sed 's/"substance": "peripheral"/"substance": "toy"/'` on
   `workflows/documentation/manifest.json`. The run ends `ELEVATED`. Signed
   `continue` and `approve` decisions are both required before it reaches
   `COMPLETE`.
4. A guardrail stop. In an earlier fault-injected run, the reviewer twice
   sent its round-2 `reviews` field as a JSON string with tool-call markup
   appended. The schema check rejected both submissions
   (`artifacts/adversarial-review/round-2.rejected-1.json` and
   `round-3.rejected-1.json`), so the revised security artifact never got a
   valid review. The coordinator recorded a human-review request (the `gate`
   event), and the harness refused the final review because that artifact had
   no current review. The run ended `FAILED_AUTOMATED` with no report. This run
   predates `tool_result` events, so the refusal shows in the coordinator's
   `final_text` in `run.json` rather than as its own audit event.

## Rubric cards

| Card | Evidence |
|---|---|
| Dispatch-only coordinator | `source/delegation/coordinator/manifest.json` lists six tools, all `launch_*` or `invoke_*`. `TestShippedManifestIsDispatchOnly` fails on any other verb. `TestCoordinatorCannotReachWorkTools` fails if the coordinator package imports the tools, sub-agent, dispatch, or SDK packages, `os/exec`, or `net/http` (`source/delegation/internal/coordinator/imports_test.go`). |
| Sub-agents scoped to one workflow | Each `source/delegation/workflows/*/manifest.json` declares its tools, and a step may narrow them but not widen them (`TestRejectsStepToolOutsideWorkflow`). Tools are rooted at the repository and refuse path escapes (`source/delegation/internal/tools/tools_test.go`). The one adversarial reviewer covers every workflow by review assignment. |
| Stage 3 guarantees | Each workflow has a prompt (`workflows/<id>/prompt.md` plus step files), at least 3 named criteria in its manifest, and `evidence/evals/<id>.json` with per-trial outcomes. Code review is also scored against 17 human-reviewed pull-request rounds, with 7 held out (`splits` in `evidence/evals/code-review.json`, and `source/delegation/ITERATION-LOG.md`). |
| Adversarial review in its own context | `workflows/adversarial-review/manifest.json` sets `isolated_context: true` and gives the reviewer `read_file`, `list_files`, and `search`. Every review round is a fresh API call: each reviewer `dispatch` event in `audit.jsonl` records `initial_messages: 1` and `isolated_context: true`, and its `llm_call` events carry their own `response_id`. The prompt (`workflows/adversarial-review/review.md`) asks for challenges with counter-evidence from the repository. |
| Final review writer | `internal/report` writes every heading and PASS/FAIL label from the artifacts (`TestHeadingsComeFromArtifacts`). The `final-review.v1` validator rejects prose with a heading or with a finding or challenge ID no agent raised (`TestFinalReviewAddsNothingNew`). `evidence/evals/final-review.json` adds a judge for new defects and for tone. |
| Deterministic guardrails between steps | A sub-agent submits through its `submit_*` tool, and `internal/schema` checks the schema and the evidence before the coordinator sees it: cited files and lines exist, the reported test exit code matches the recorded run, and criteria IDs match the task. A failure returns `{"ok":false,"error":{"code":"schema_violation",...}}` (`TestInvalidSubmissionIsStructuredError`). No submission is overwritten. Rejected ones are kept as `artifacts/**/*.rejected-N.json`, and a relaunch moves the earlier valid artifact to `<step>.v1.json`, with the move recorded in the `validation` event (`TestRelaunchKeepsEarlierSubmissions`). |
| Version-controlled repository | `evidence/git-log.txt`. |
| Stage 4 continuity: custom harness | The runtime calls the Messages API through the Go SDK (`internal/provider/anthropic`). The Claude CLI appears only under `delegation/evals/`, and `TestRuntimeCannotReachCLIProvider` fails if `cmd/delegate` can reach it. `run.json` records `"provider": "anthropic-api"` for every sample run. |
| Stage 4 continuity: punch-outs | Runs end `PENDING_HUMAN` or `ELEVATED`. Only `delegate decide` changes that. Each decision names the reviewer, binds the report's SHA-256, is GPG-signed, and is appended to `decisions.jsonl` and the audit chain; `report.md` and `run.json` are never edited (`TestDecisionBindsReport`). The coordinator's only human tool, `invoke_human_review`, records a request and cannot approve. |
| Stage 4 continuity: substance gate | `run.json` scores every workflow. The shipped catalog has no toy and one peripheral workflow (documentation), so it does not elevate. The elevated sample run and `TestSubstanceGateElevates` show the gate firing. |
| Stage 4 continuity: audit trail | `audit.jsonl` is hash-chained and fsynced per event under one correlation ID. Every model call records model, input, output, and cache tokens, cost, duration, and response ID, with parent spans linking each sub-agent to the coordinator turn that launched it. `summary.json` totals tokens and cost per agent. `delegate verify <id>` re-checks the chain and the decision signatures from disk (`TestVerifyDetectsTampering`). |
| The counterfeit | The coordinator holds no file, HTTP, or shell tool, and the import test above proves its package cannot reach one. It sees the task and the workflow catalog, never the code, unless a sub-agent quotes it. |

## Rebuild and verify

```sh
gpg --import evidence/reviewer-keys.asc
cd source
go test ./delegation/...
go run ./delegation/cmd/delegate replay --task delegation/fixtures/endpoint-allowlist \
  --exchanges ../evidence/recordings/endpoint.exchanges.jsonl --runs /tmp/delegate-replay
go run ./delegation/cmd/delegate verify <run-id> --runs ../evidence/runs
```
