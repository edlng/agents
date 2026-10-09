# Iteration log

What broke, what changed, and what each change measured. Dates are 2026.
Costs up to 10-06 are model spend through the Claude CLI on the evaluation
path; from 10-07 every measurement goes through the Messages API, the same
path the runtime uses.

## Harness and guardrails

| Date | Observed | Change | Result |
|---|---|---|---|
| 10-05 | `claude-haiku-4-5` through the CLI silently ran Opus 5. | The CLI provider maps model IDs and fails if the reported model differs from the requested one. | Unit test `TestRejectsSilentModelFallback`. |
| 10-05 | `Resolve` rejected every file in a temp-dir repository on macOS (`/var` is a symlink to `/private/var`). | Resolve the repository root through symlinks before comparing. | Schema and sub-agent tests pass. |
| 10-05 | Live run: the criteria step cited files without line numbers, and the validator rejected it. | Criteria may cite a whole file (line 0); findings and challenges still need exact lines. | Next live run validated. |
| 10-05 | Live run: the reviewer challenged documentation it could not read; written docs live in the run output, not the repository. | Written docs are passed to the reviewer inline and may be cited by `docs/` path. | Next live run validated. |
| 10-05 | Live run 1: the reviewer wrote its verdict as text and never called `submit_challenges`; the audit trail did not record why a launch failed. | Failed launches log their error code and closing text. The CLI protocol states that harness tools are called only through `tool_calls`. | Run 2 passed the review step. |
| 10-05 | Live run 2: the final writer cited F1 (a code-review finding) in the documentation section; the validator allowed only IDs raised about the same artifact. | Any ID raised by an agent in the run may be cited; IDs no agent raised are still rejected. | Run 3 ended `PENDING_HUMAN` with a correct report, $0.43. |
| 10-05 | Harness test: after a rework, the writer could not mention the challenge that caused it; only the latest review round's IDs were known. | Keep every review round per artifact and give the writer the full history. | `TestReworkLoopEndsPendingHuman`. |
| 10-05 | Evals: under `claude -p`, agents called harness tools natively, got "No such tool available", and gave up. | The CLI protocol names StructuredOutput as the only native tool. | Reviewer, docs, and criteria cases went from 5/11 to 11/11 passing. |
| 10-06 | Re-check against the rubric: `request_` is not a listed dispatch verb; the reviewer skipped the substance assessment; approvals carried only a typed name. | Renamed to `invoke_human_review`; the reviewer is scored core; decisions are GPG-signed and verified. | Coordinator evals rerun: every criterion passed. |
| 10-06 | Final-review "no new findings" was enforced only by ID. | Added a judge that compares the prose against every raised finding, challenge, and remediation. | First judge run flagged a remediation it had not been given; after including remediations, 6/6. |
| 10-06 | Package dry run: nothing recorded which provider produced a run. | `run.json` records the provider; `package.sh` refuses sample runs not produced by the API runtime. | Old CLI run refused. |
| 10-07 | First API run: the gateway refused `claude-opus-5-5` (organization policy allows Sonnet and Haiku). | The coordinator runs on `claude-sonnet-5-5`. | Live API run ended `PENDING_HUMAN`, 17 calls, $0.18; coordinator evals through the API passed every criterion (12 trials, $0.50). |
| 10-07 | API evals: on the "clean" allowlist fixture, one correctness trial reported that `strings.ToLower` folds the Kelvin sign (U+212A) to `k`, so `https://\u212Aapi.example.com` matched `kapi.example.com`. A test confirmed the bypass. | The fixture rejects non-ASCII hosts before lowercasing, with a regression test. | approves-clean-change 6/6. The fixture label was wrong; the system was right. |
| 10-07 | API evals: reviewers sometimes submitted challenge IDs `""` or `"x"`, which failed validation. | The harness numbers findings (F1...) and challenges (C1...); the model no longer supplies IDs. | adversarial-review 6/6 on every criterion (was 5/6). |
| 10-07 | Tried strict tool use on submit tools to stop malformed submissions. | Reverted. Under strict mode the models filled required fields with placeholders (one empty criterion, `file: "x"`, blank evidence): 21 validation failures, spec-validation fails-unmet-criterion 0/9, held-out verdict 9/14. | Without strict: 9/9 and 12/14. |
| 10-07 | API evals: in one `fr-blocked` trial the final writer asked for a regression test for the look-alike host. No agent had asked for one; the novelty judge caught it (`adds-no-new-findings` 5/6). | The writer prompt forbids fixes, tests, or hardening the agents did not state. | Final-review evals rerun through the API; see `adds-no-new-findings` in `evals/final-review/results.json`. |
| 10-07 | Final-review rerun through the API: 5/6 again. The judge flagged the writer's "AC1, AC3, AC4 and AC5 pass", which was in the writer's input; the judge saw only the failed criteria. | The judge receives every criterion status. The writer prompt is unchanged. | Next API run: `adds-no-new-findings` 6/6, but one tone trial failed because the tone judge ended without calling its tool. |
| 10-07 | That tone failure scored a judge slip as a writer failure. | Each judge asks once more when it makes no tool call; a second miss is still scored as a failure. | API rerun: every final-review criterion 6/6, $0.05. |
| 10-07 | Sample runs: 6 API runs of `archive-path-injection` and one of `endpoint-allowlist` drew only minor challenges, so none reworked (the CLI drew one major challenge in 4 runs). | `evals/cmd/faultrun` plants a false APPROVE as the first security submission and runs everything else through the API, so the rework loop can be shown on demand. The sample run is labeled fault-injected. | See the rework sample run. |
| 10-08 | Report verification of the fault-injected run: `security.json` held only the re-run BLOCK. The relaunch had overwritten the planted APPROVE, so the challenged artifact was gone, despite the README's "every submission". | A relaunch moves the earlier valid artifact to `<step>.v<n>.json` and records the move in the audit event; repeated rejections get distinct names. | `TestRelaunchKeepsEarlierSubmissions`; fault run repeated. |
| 10-08 | A fault run ended `FAILED_AUTOMATED`: the reviewer twice sent `reviews` as a JSON string with tool-call markup, the schema check rejected both, and the harness refused the final review. The refusal was visible only in the coordinator's text. | The harness logs a `tool_result` event (ok, artifact ID, error code) for every coordinator tool call. The schema check stays strict. | Kept as a sample of the guardrail stopping a run; the fault run was repeated and reworked. |
| 10-09 | Sample run on `archive-path-injection` ended `FAILED_AUTOMATED`. Round 1: the reviewer marked two artifacts UPHELD with minor challenges attached, which the schema allows only under CHALLENGED. Round 2: it left out the two artifacts it upheld. The prompt stated neither rule. | The reviewer prompt says any challenge, minor included, makes a review CHALLENGED, and that upheld artifacts still get a review. The schema check is unchanged. | adversarial-review rerun through the API: 6/6 on every criterion, $0.12. Every sample run was redone on the new prompt. |

## Code review on real pull requests

The corpus is 17 review rounds of `aws/developer-toolkit-elasticache`
(`delegation/corpus/`): 10 used for tuning and 7 held out, labeled before any
results were seen. Labels are the production-code defects human reviewers
flagged, each linked to its review comment.

| Date | Prompt state | Tuning verdict | Tuning recall | Notes |
|---|---|---|---|---|
| 10-06 | Original Stage 3 prompts | 7/10 | 4/12 | Reviewers judged from the patch alone: one or two model calls per lens, almost no file reads. All four human-approved heads agreed; three of six blocked rounds were approved with zero findings. |
| 10-06 | Read changed files in full; correctness lens works through check-then-act, completion, freshness, scheduling, callback, and consistency questions; security lens traces secrets into exception causes, logs, and callbacks | 8/10 | 5/12 | One trial. The reviewers also reported races on the human-approved heads of PR 13 and PR 17. The human reviewer could not confirm or reject them; both are recorded as disputed and excluded from verdict scoring. |
| 10-06 | Same prompts, effort `high` instead of `medium` | 6/8 | 5/12 | Excluding the two disputed cases (the previous row is also 6/8 on that basis). Found different defects, not more, at 47% higher cost. Reverted. |
| 10-06 | Prompts frozen, effort `medium`, 2 trials per case | 11/16 | 11/24 | Final CLI measurement, $8.92. |
| 10-07 | Same frozen prompts, through the API | 11/16 | 9/24 | Final measurement, the numbers in `evals/code-review/results.json`. Two fewer defects recalled than the CLI run; see below. |

Final measurement on the held-out split, which no prompt change saw, through
the API: verdict 12/14 (86%) and recall 11/16 (69%). The CLI run of the same
prompts recalled 13/16 held out and 11/24 in tuning. Both splits have only
two trials per case, so this log reports the API numbers without claiming
the provider changed recall. The
held-out rounds are easier than the tuning rounds (CI workflow defects,
smaller diffs), so the tuning numbers are the better guide to hard
concurrency reviews. Both held-out verdict misses are the human-approved PR 16
head, where both trials reported that `--require-npm=11.5.1` and
`--require-npm ""` still skip the npm version check; that extends the human
reviewer's own round-1 comment and is a candidate for adjudication. It is
scored as measured.

A tuning run that overlapped a manifest-hashing run was discarded: both used
the same cache paths, and the hashing run deleted checkouts mid-review. Each
eval process now uses its own cache directory.

Defects still missed in tuning, for the next iteration: serving through the
raw expiry instead of the effective lifetime, a callback that runs before a
refresh completes, inconsistent error types across waiters, callbacks on the
shared `commonPool`, and the PR 17 completion race.
