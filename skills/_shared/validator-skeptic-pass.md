# Shared: Validator (Skeptic Pass)

> Shared reference used by `review-pr`, `review-cookbook-pr`, `review-code`, and `multi-discipline-review`. Not a standalone skill. Single source of truth for the adversarial validator pass that maximizes signal by killing false positives. Consuming skills choose the model/subagent role and may add skill-specific self-challenge questions.

The validator earns its cost by confirming what is real, downgrading what is overstated, rejecting what is false, and adding only high-confidence misses. This is where false positives that would otherwise reach the user get killed.

## Validator prompt template

> "You are a skeptical senior engineer doing a second pass on another reviewer's findings. Your job is to maximize signal.
>
> **Effort budget: 10-20 tool calls max. Read only the files needed to verify claims — do not re-review the entire diff.**
>
> Read the findings, diff, requirements (may not exist), and codebase context from the cache keys the controller gave you. If you need to verify a claim against a file, read the source directly using the consuming skill's stated read mechanism. Do NOT trust a finding's evidence blindly — re-read the source if anything looks off.
>
> For each finding, apply this self-challenge before deciding your verdict:
> 1. Can I point to the exact line in the diff that proves this claim?
> 2. Did I verify the issue isn't already handled elsewhere in the diff or codebase?
> 3. Would a concrete input/scenario actually trigger this failure?
>
> Then attach `verdict` (`CONFIRMED` | `DOWNGRADE` | `REJECTED`), `verdict_reason` (one sentence), and if `DOWNGRADE` also a new `severity`. Reject the finding if any of:
>   - The cited symbol/file/line does not exist or does not say what the finding claims (hallucinated evidence).
>   - The 'bug' is already handled elsewhere in the diff or in the codebase context.
>   - The finding is generic ('add error handling', 'add validation') without a concrete failure scenario.
>   - The finding is a matter of taste, not a deviation from the codebase context.
>   - The finding is outside the diff and not load-bearing for a diff change.
>
> Downgrade if the issue is real but the severity is overstated relative to the skill's rubric.
>
> Then independently scan the diff for HIGH-CONFIDENCE misses. Add at most 3 new findings (or the cap the consuming skill specifies), only if you have direct evidence and the issue is at least `suggestion` severity. Do NOT pad. If you have nothing to add, add nothing.
>
> Output: full updated findings array (original + verdict fields, plus any added findings with `lens: 'validator_added'`). Write it to the next findings version key.
>
> Also output two top-level numbers used by the loop controller:
>   - `reject_rate`: rejected_count / total_input_count
>   - `added_count`: number of new findings you added"

## Loop control

Read `reject_rate` and `added_count` from the validator output.
- If `reject_rate >= 0.30` OR `added_count >= 2`: re-run the validator on the latest findings version.
- Else: converged — proceed to the report phase.

Cap total validator passes per the consuming skill (PR review caps at 3; local review caps at 2). In practice 1 pass is enough; 2 is the worst common case.

## Model / role selection

The consuming skill selects the subagent role and model. Typical pattern, gated by diff size (`additions + deletions`):
- `<= 300`: skip the validator entirely — the initial reviewer is sufficient for small diffs.
- `<= 800`: use a `code-reviewer` subagent as the validator (cheaper, still catches hallucinations).
- `> 800`: use a `validator` subagent (earns its cost on large/complex diffs).
