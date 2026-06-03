## 1. Remove Generic Knowledge Tests

- [x] 1.1 Delete Tests 1–5 (capital of France, US government branches, sky color, Tokyo JSON, French hello) from `promptfooconfig.yaml`
- [x] 1.2 Delete Test 10 (async/await vs Promises explanation) from `promptfooconfig.yaml`
- [x] 1.3 Retain one short "cost calibration baseline" test — repurpose the "say hello in French" test into an explicit baseline labeled `cost_baseline`, with a word-count cost assertion targeting < 50 words

## 2. Standardize Grading and Cost Metrics

- [x] 2.1 Update `graders/judge-prompt.txt` to use a 1–5 scale with explicit per-score criteria (1 = completely wrong, 2 = partial/major gaps, 3 = acceptable, 4 = good/minor issues, 5 = excellent)
- [x] 2.2 Update all `llm-rubric` assertions in `promptfooconfig.yaml` to define 1–5 criteria rather than open-ended "score based on" language, and set `threshold` to the normalized equivalent (e.g. 0.8 = score 4/5)
- [x] 2.3 Ensure every test has a `latency` gate at 30000ms (hard fail only, not scored)
- [x] 2.4 Ensure every test has a `javascript` cost assertion that decays score as word count grows past a task-appropriate ceiling; update the ceiling per test type (50 words for one-liners, 300 for explanations)

## 3. Create Firecrawl Stub Provider

- [x] 3.1 Create `providers/stubs/` directory
- [x] 3.2 Write `providers/stubs/firecrawl_stub.sh` — outputs fixture JSON with at least one `results` entry containing `url` and `content` fields, exits 0 for any input
- [x] 3.3 Write `providers/stubs/researcher_stub.sh` — wraps `researcher.sh` with an env var or flag that points Firecrawl to the stub fixture

## 4. Add Researcher Agent Tests

- [x] 4.1 Add test: researcher answers a domain-specific technical question — assert with 1–5 rubric (4+ threshold): correct answer, source/citation present, concrete recommendation
- [x] 4.2 Add test: researcher on a simple lookup — assert under 100 words via `javascript`, correct answer via `icontains`; mark as cost baseline

## 5. Update Team-Lead Agent Tests

- [x] 5.1 Verify Test 11 (trivial rename → no delegation) is present; update its `llm-rubric` to use the 1–5 scale
- [x] 5.2 Verify Test 12 (Valkey batching program) explicitly asserts `valkey-glide` is used and rejects `ioredis`/`iovalkey`
- [x] 5.3 Add test: team-lead receives a multi-file feature request — assert it produces a plan referencing subagent/builder, does NOT inline the full implementation

## 6. Add review-code Skill Tests

- [x] 6.1 Add test: `review-code` on a snippet with a known correctness or security bug — assert with 1–5 rubric (4+ threshold): finding identified, evidence quoted, fix suggested
- [x] 6.2 Add test: `review-code` on clean well-written code — assert with 1–5 rubric (4+ threshold): no BLOCK/MUST_FIX verdict; verify `javascript` assertion that penalizes false-positive critical findings

## 7. Add review-pr Skill Tests

- [x] 7.1 Add test: `review-pr` on a PR reference with known issues — assert `## Blocking` section is present, each finding has file, line range, and fix
- [x] 7.2 Add test: `review-pr` output check — assert output does NOT contain `gh pr review`, `gh pr comment`, or GitHub write indicators (local-only enforcement)

## 8. Add write-pr Skill Tests

- [x] 8.1 Add test: `write-pr` on a diff — assert Summary, Changes, and Tests sections are non-empty; assert Changes section contains at least one filename from the diff via `javascript`
- [x] 8.2 Add test: `write-pr` output — assert with 1–5 humanizer rubric (4+ threshold) that the description does not contain AI vocabulary markers; also assert via `javascript` that `streamlines`, `robust`, `enhance`, `leverage`, `crucial`, `pivotal` are absent

## 9. Add multi-discipline-review Skill Test

- [x] 9.1 Add test: `multi-discipline-review` on the bank-transfer snippet (security + atomicity + validation issues) — assert via `javascript` that output contains findings from at least 3 discipline labels (security, correctness, design, performance, testing)
- [x] 9.2 Verify existing Test 9 (multi-discipline coverage on team-lead) is migrated to use the 1–5 rubric scale

## 10. Verify and Validate

- [x] 10.1 Run `npm run eval` and confirm no config parse errors or test setup failures
- [ ] 10.2 Run `npm run eval:view` and verify new tests appear with correct metric names and scores
- [ ] 10.3 Confirm stub tests complete under 10 seconds per the latency gate
- [x] 10.4 Confirm no generic knowledge test descriptions appear in output
