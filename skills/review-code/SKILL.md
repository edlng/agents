---
name: review-code
description: Self-review uncommitted or unpushed work before opening a PR. Multi-phase review with Jira requirements, Valkey caching, parallel subagent reviewers, Opus skeptic validator, and auto-fix offer. Pass a branch name to review vs main, a Jira key to use as requirements source, or leave empty to review staged+unstaged changes.
---

# Review My Local Changes

Self-review your own uncommitted or unpushed work before opening a PR. The goal is to catch your own mistakes — bugs, sloppy patterns, missed requirements — at the cheapest possible point. Optimized for speed and **fixing in place**: when a finding is unambiguous, this command will offer to fix it for you.

`$ARGUMENTS` is optional:
- empty → review staged + unstaged changes vs `HEAD`
- `branch` → review the full branch vs the merge-base with `main`/`master`
- a Jira issue key → use it as the requirements source (otherwise inferred)

This workflow uses Valkey at `localhost:8888` for caching across phases. **All GitHub interactions go through the GitHub MCP server (`mcp__github__*` tools) only.** Do not use the `gh` CLI for anything — not reads, not writes. Local `git` is fine for inspecting your working tree (diff, log, branch); `gh` is not.

**Output is local-only.** Print the review in chat. **DO NOT** post anything to GitHub or open/update PRs. The user will act on findings manually.

---

## Cache setup

Cache keys (TTL 6h):
- `local:$RUNID:diff`
- `local:$RUNID:requirements` (only if a Jira ticket is identified)
- `local:$RUNID:codebase_context`
- `local:$RUNID:findings_v<n>`

`$RUNID` = current branch name. Subagents read via `valkey-cli -p 8888 GET <key>` or `valkey-glide`. Mark cached values as Anthropic prompt-cached prefixes when inlined.

---

## Phase 1: Context Gathering

**Model: sonnet-4-6** (single agent, no spawn)

### 1a. Determine the diff scope
- If `$ARGUMENTS` is `branch`: `git merge-base HEAD origin/main` (or `master`), then `git diff <merge-base>...HEAD`.
- Otherwise (default): `git diff --cached HEAD; git diff HEAD` combined.

If the diff is empty, stop and say "no local changes to review."

If `additions + deletions > 1500`, warn — self-review at this size is lossy. Suggest scoping with `$ARGUMENTS=branch` or splitting before continuing. Continue only if the user confirms.

Write to `local:$RUNID:diff`.

### 1b. Identify requirements source (best-effort, do not block)
- If `$ARGUMENTS` is a Jira issue key: use it directly via `mcp__atlassian__getJiraIssue`.
- Else, search on Jira and try `mcp__atlassian__getJiraIssue` and see if there's a match.
- Else, scan the most recent commit messages for an issue key.
- If still nothing: **skip the requirements lens entirely** — local self-review is often pre-ticket exploration. Note this in the final report. Do NOT prompt the user to find a ticket.

If a ticket is found, write the Requirements Document (acceptance criteria, edge cases, implicit constraints) to `local:$RUNID:requirements`.

### 1c. Codebase Context
Read touched files + their callers/neighbors at the **current working tree state**. Capture: language/runtime, naming conventions, error-handling patterns, logging, existing utilities the new code should call (especially flag any case where the diff reimplements something nearby), test conventions, key/value client (must be `valkey-glide`).

Write to `local:$RUNID:codebase_context`.

---

## Phase 2: Self-Review (merged lenses)

**Model: sonnet-4-6**

Spawn ONE reviewer subagent. Apply lenses in a single pass. Use the `code-review-excellence` skill as the reasoning frame.

Prompt:
> "You are reviewing the author's own uncommitted work. The author wants to catch mistakes before pushing — be direct, no diplomatic softening. Read from Valkey at `localhost:8888`:
>   - `local:$RUNID:diff`
>   - `local:$RUNID:requirements` (may not exist — if missing, skip Lens 1)
>   - `local:$RUNID:codebase_context`
>
> Apply lenses below in one pass. Output strict JSON: a flat array of findings with fields:
>   - `id`: short slug
>   - `lens`: `requirements` | `correctness_security` | `design_fit` | `testability` | `unfinished`
>   - `file`, `line_range`
>   - `severity`: `must_fix` | `should_fix` | `consider`
>   - `claim`: one sentence
>   - `evidence`: literal lines from diff/context. **No evidence → no finding.**
>   - `suggested_fix`: concrete change
>   - `auto_fixable`: boolean — true only if the fix is small (≤10 lines), local to one file, and unambiguous
>
> ### Lens 1 — Requirements alignment (skip if no requirements cached)
> Does the diff cover every acceptance criterion? Anything in the diff that is scope creep beyond requirements?
>
> ### Lens 2 — Correctness & Security
> Real bugs only: off-by-ones, wrong nil/error handling, race conditions, broken invariants, unsafe input handling at trust boundaries (injection, traversal, deserialization), secrets, weak crypto. Generic 'add validation' findings are rejected by the validator — be concrete about the failing input.
>
> ### Lens 3 — Design fit
> Did the author reimplement something that already exists in the codebase context? Premature abstraction or duplication within the diff? Inconsistent error handling vs the codebase pattern? Layering violations?
>
> ### Lens 4 — Testability
> Does the diff include tests for new behavior and failure paths? Mocks where a real fixture would catch the bug? Tests that exercise but don't assert?
>
> ### Lens 5 — Unfinished work (LOCAL-ONLY lens)
> This is what self-review catches that PR review can't. Flag:
>   - `TODO`, `FIXME`, `XXX`, `HACK` left in the diff
>   - Commented-out code
>   - `console.log`, `print()`, `dbg!()`, `pp` debug statements
>   - Hardcoded test values (`localhost:8888` outside Valkey config, hardcoded user IDs, dummy keys)
>   - Empty catch/except blocks
>   - Stubbed functions that return placeholder values
>   - Missing imports / unused imports
>
> IMPORTANT: Report only gaps that affect correctness, security, or stated requirements. Do NOT file findings that are purely matters of taste or style unless they directly conflict with a codebase pattern visible in the codebase context.
>
> Severity rubric:
>   - `must_fix`: real bug, security issue, broken acceptance criterion, debug code left in
>   - `should_fix`: design issue, missing test, unfinished cleanup
>   - `consider`: stylistic, judgment-call refactors
>
> Write JSON to `local:$RUNID:findings_v1`."

---

## Phase 3: Validator (skeptic pass)

**Model: claude-opus-4-8**

Spawn one validator subagent.

Prompt:
> "Skeptical second pass. Read from Valkey at `localhost:8888`:
>   - `local:$RUNID:findings_v$n`
>   - `local:$RUNID:diff`
>   - `local:$RUNID:requirements` (may not exist)
>   - `local:$RUNID:codebase_context`
>
> For each finding, attach `verdict` (`CONFIRMED` | `DOWNGRADE` | `REJECTED`) and `verdict_reason`. Reject if:
>   - cited symbol/file/line is wrong or doesn't say what's claimed
>   - already handled elsewhere in the diff/codebase
>   - generic without a concrete failure scenario
>   - matter of taste, not a deviation from codebase patterns
>   - outside the diff and not load-bearing
>
> Re-evaluate `auto_fixable`: only true if the fix is small, local, and unambiguous. If the validator is unsure, set false.
>
> Add at most 2 high-confidence misses, particularly in Lens 5 (unfinished work) — that's the highest-value lens for local review.
>
> Write to `local:$RUNID:findings_v$(n+1)`. Output `reject_rate` and `added_count` for the loop controller."

### Loop control
If `reject_rate >= 0.30` OR `added_count >= 2`: re-run Phase 3. Cap at 2 validator passes — local review should be fast; if findings are still volatile after 2 passes, ship the current set with a note.

---

## Phase 4: Report + Auto-Fix Offer

**Model: haiku-4-5** (for report aggregation)

Spawn a haiku-4-5 summary subagent.

Prompt:
> "Read `local:$RUNID:findings_v<final>`. Drop `verdict: REJECTED`. Group by severity (post-downgrade). Output markdown:
>
> ```
> # Local Review: $RUNID
>
> ## Must Fix (N)
> For each must_fix:
> - **<file>:<line>** — <claim>
>   - Evidence: <evidence>
>   - Fix: <suggested_fix>
>   - [auto-fixable] if true
>
> ## Should Fix (N)
> <same format>
>
> ## Consider (N)
> <same format, terse>
>
> ## Skipped lenses
> <list any skipped, e.g. 'requirements (no Jira ticket found)'>
>
> ## Bottom line
> <one paragraph: ready to commit, ready with fixes, or needs more work>
> ```"

Print the report.

### Auto-fix offer

Count findings where `auto_fixable: true` AND `verdict != REJECTED`.

If count > 0, ask:
> "I can auto-fix N findings (the auto-fixable ones). Apply now? (yes / no / pick)"

- **yes**: apply each fix using Edit. Re-run any fast checks (linter / typecheck) the codebase context identified. Print a diff of applied changes.
- **pick**: list the auto-fixable findings with numbers, ask which to apply.
- **no**: stop, leave fixes for the author.

Do NOT auto-fix anything not marked `auto_fixable: true` by both Phase 2 and confirmed by the validator.

---

## Decision

- **0 must_fix**: ✓ ready to commit (modulo any "should fix" the user wants to address)
- **≥1 must_fix**: stop — author needs to address before pushing

---

## STRICT: No GitHub writes

This command is **read-only on GitHub** AND **MCP-only**. Auto-fix may modify local files (with explicit user approval), but never:
- Use the `gh` CLI for anything. All GitHub access is via `mcp__github__*` tools.
- Push to any branch (`mcp__github__push_files`, `mcp__github__create_or_update_file`)
- Open, update, or comment on PRs (`mcp__github__create_pull_request`, `mcp__github__update_pull_request`, `mcp__github__pull_request_review_write`, `mcp__github__add_comment_to_pending_review`)
- Post issue comments (`mcp__github__add_issue_comment`, `mcp__github__issue_write`)
- Any other write method (`*_write`, `create_*`, `update_*`, `merge_*`, `delete_*`)

The user will commit and push manually after they've reviewed any auto-fixes.
