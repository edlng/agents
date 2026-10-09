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

`evidence/runs/INDEX.md` has the run IDs. Every run was made on 2026-10-09
through the Messages API. The first four used the shipped source and prompts.
The guardrail stop used the reviewer prompt from before that day's fix. The
runs show five paths:

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
4. A rejection, on a second task (`archive-path-injection`). The report blocks
   the change and leaves a minor challenge to the documentation unresolved.
   The human signs `reject` with a note, and the run ends `REJECTED_HUMAN`.
5. A guardrail stop, on the same task. In round 1 the reviewer marked two
   artifacts UPHELD while attaching minor challenges, and in round 2 it left
   out the artifacts it upheld. The schema check rejected both submissions
   (`artifacts/adversarial-review/round-1.rejected-1.json` and
   `round-2.rejected-1.json`), and each `tool_result` event in `audit.jsonl`
   carries the `schema_violation` the coordinator received. The coordinator
   recorded a human-review request, and the harness refused the final review
   with `review_pending`. The run ended `FAILED_AUTOMATED` with no report. The
   reviewer prompt now states both rules (`ITERATION-LOG.md`, 10-09), and
   runs 1 to 4 used it.

## Rubric cards

| Card | Evidence |
|---|---|
| Dispatch-only coordinator | `source/delegation/coordinator/manifest.json` lists six tools, all `launch_*` or `invoke_*`. `TestShippedManifestIsDispatchOnly` fails on any other verb. `TestCoordinatorCannotReachWorkTools` fails if the coordinator package imports the tools, sub-agent, dispatch, or SDK packages, `os/exec`, or `net/http` (`source/delegation/internal/coordinator/imports_test.go`). |
| Sub-agents scoped to one workflow | Each `source/delegation/workflows/*/manifest.json` declares its tools, and a step may narrow them but not widen them (`TestRejectsStepToolOutsideWorkflow`). Tools are rooted at the repository and refuse path escapes (`source/delegation/internal/tools/tools_test.go`). The one adversarial reviewer covers every workflow by review assignment. |
| Stage 3 guarantees | Each workflow has a prompt (`workflows/<id>/prompt.md` plus step files), at least 3 named criteria in its manifest, and `evidence/evals/<id>.json` with per-trial outcomes. Code review is also scored against 17 human-reviewed pull-request rounds, with 7 held out (`splits` in `evidence/evals/code-review.json`, and `source/delegation/ITERATION-LOG.md`). |
| Adversarial review in its own context | `workflows/adversarial-review/manifest.json` sets `isolated_context: true` and gives the reviewer `read_file`, `list_files`, and `search`. Every review round is a fresh API call: each reviewer `dispatch` event in `audit.jsonl` records `initial_messages: 1` and `isolated_context: true`, and its `llm_call` events carry their own `response_id`. The reviewer's job is to find where an artifact is wrong (`workflows/adversarial-review/prompt.md`). It challenges claims only with counter-evidence at a file and line. It catches missed defects and findings that are not real. It detects PASS verdicts without evidence, cited lines that do not show what is claimed, and test results that do not match the code. A `critical` challenge blocks the run. The coordinator relaunches the challenged step (scored by `relaunches-challenged-step` in `evidence/evals/coordinator.json`), the harness refuses the final review until the revised artifact has its own review round, and an open critical challenge prints `UNRESOLVED` in the report. |
| Agents need no manual correction | No human edits agent output. The harness refuses an invalid submission and the coordinator relaunches the step, and the human only signs `approve`, `continue`, or `reject` on the finished report. None of the sample runs needed a human edit. Every pass rate in `evidence/evals/` is from unedited agent output. Code-review recall on the hard tuning split is 9/24 human-flagged defects, so the report can miss a defect. A miss lowers report quality but nothing ships unless a human signs the report. |
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
