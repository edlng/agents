# Iteration log

What broke, what changed, and what each change measured. Dates are 2026.
Costs are model spend through the Claude CLI on the evaluation path.

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
| 10-06 | Prompts frozen, effort `medium`, 2 trials per case | 11/16 | 11/24 | Final measurement, $8.92. |

Final measurement on the held-out split, which no prompt change saw: verdict
12/14 (86%) and recall 13/16 (81%). The held-out rounds are easier than the
tuning rounds (CI workflow defects, smaller diffs), so the tuning numbers are
the better guide to hard concurrency reviews. Both held-out verdict misses
are the human-approved PR 16 head, where both trials reported that
`--require-npm=11.5.1` and `--require-npm ""` still skip the npm version
check; that extends the human reviewer's own round-1 comment and is a
candidate for adjudication. It is scored as measured.

A tuning run that overlapped a manifest-hashing run was discarded: both used
the same cache paths, and the hashing run deleted checkouts mid-review. Each
eval process now uses its own cache directory.

Defects still missed in tuning, for the next iteration: serving through the
raw expiry instead of the effective lifetime, a callback that runs before a
refresh completes, inconsistent error types across waiters, callbacks on the
shared `commonPool`, and the PR 17 completion race.
