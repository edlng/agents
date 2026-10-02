---
name: review-code
description: Self-reviews uncommitted or unpushed work before opening a PR. Multi-phase review with Jira requirements, a single reviewer agent, a skeptic validator pass, and an auto-fix offer. Use when asked to review local changes, self-review a branch before a PR, or check your own diff. Pass a branch name to review against main, a Jira key as the requirements source, or nothing to review staged and unstaged changes.
---

# Review My Local Changes

Self-review your own uncommitted or unpushed work before opening a PR. The goal is to catch your own mistakes (bugs, sloppy patterns, missed requirements) at the cheapest point. The review is fast and fixes in place: when a finding is unambiguous, the skill offers to fix it.

`$ARGUMENTS` is optional:
- empty: review staged and unstaged changes vs `HEAD`
- `branch`: review the full branch vs the merge-base with `main`/`master`
- a Jira issue key: use it as the requirements source (otherwise inferred)

GitHub reads use the `gh` CLI. Local `git` is fine for inspecting your working tree (diff, log, branch). Output is local-only and GitHub writes are off limits; follow `../_shared/no-github-writes.md`. Auto-fix may modify local files with explicit user approval, but never touches GitHub.

---

## Run directory

Shared files live in `/tmp/agent-runs/<RUNID>/`, where `RUNID` is the current branch name with `/` replaced by `-`. Create it with `mkdir -p`. Subagents read these files with their file-reading tool. If a file is missing, regenerate it.

- `diff.patch`
- `requirements.md` (only if a Jira ticket is identified)
- `codebase_context.md`
- `findings_v<n>.json`

---

## Phase 1: Context Gathering

Run this phase in the main session. Do not spawn agents for it.

### 1a. Determine the diff scope
- If `$ARGUMENTS` is `branch`: `git merge-base HEAD origin/main` (or `master`), then `git diff <merge-base>...HEAD`.
- Otherwise (default): `git diff --cached HEAD; git diff HEAD` combined.

If the diff is empty, stop and say "no local changes to review." If `additions + deletions > 1500`, warn that self-review at this size is lossy and suggest scoping with `$ARGUMENTS=branch` or splitting. Continue only if the user confirms. Write to `diff.patch`.

### 1b. Identify requirements source (best-effort, do not block)
- If `$ARGUMENTS` is a Jira issue key: use it directly via the Jira MCP `getJiraIssue` tool.
- Otherwise, search Jira and try to match, or scan recent commit messages for an issue key.
- If still nothing, skip the requirements lens entirely. Local self-review is often pre-ticket exploration. Note this in the final report and do not prompt the user to find a ticket.

If a ticket is found, write the Requirements Document to `requirements.md`.

### 1c. Codebase Context
Build per `../_shared/codebase-context-checklist.md`, reading touched files at the current working-tree state. Write to `codebase_context.md`.

---

## Phase 2: Self-Review (merged lenses)

Spawn ONE `code-reviewer` agent. It applies the lenses in a single pass and uses the `code-review-excellence` skill as the reasoning frame.

Apply lenses in order. Codebase Alignment comes first (primary lens: a change that does not fit the codebase is a defect even if logically correct), then Correctness & Security, then Requirements (if a ticket was found), then Testability. Only flag codebase-alignment issues that conflict with visible patterns in `codebase_context.md`, not general preferences.

Use the findings schema and the four core lenses from `../_shared/review-findings-schema.md`, with these local-review additions:

- **Severity examples:** `important` covers a real bug, a security issue, a broken acceptance criterion, a missing test for new behavior, and debug code left in. `nit` covers unfinished cleanup and small refactors. `pre-existing` covers a real bug in code this diff did not introduce.
- **Extra field:** `auto_fixable`: boolean. True only if the fix is small (10 lines or fewer), local to one file, and unambiguous.
- **Extra Lens 5, Unfinished work (local-only lens).** Self-review catches what PR review cannot. Flag: `TODO`/`FIXME`/`XXX`/`HACK` left in the diff; commented-out code; `console.log`/`print()`/`dbg!()`/`pp` debug statements; hardcoded test values (hardcoded user IDs, dummy keys); empty catch/except blocks; stubbed functions returning placeholders; missing or unused imports.

Tell the agent: "You are reviewing the author's own uncommitted work. Be direct, no diplomatic softening. Read `/tmp/agent-runs/<RUNID>/diff.patch`, `requirements.md` (may not exist; if missing, skip the requirements lens), and `codebase_context.md`. Report only gaps that affect correctness, security, or stated requirements, not matters of taste unless they conflict with a visible codebase pattern. Write JSON to `findings_v1.json`."

---

## Phase 3: Validator (skeptic pass)

Spawn ONE `validator` agent. Pass it the path to `findings_v1.json` and do not have it re-read the full diff. Run a single pass. Do not loop or re-run the validator on its own output.

Follow the self-challenge rubric in `../_shared/validator-skeptic-pass.md`. The validator reads only the findings list and `codebase_context.md`. Apply these local-review specifics on top:
- Re-evaluate `auto_fixable`: true only if the fix is small, local, and unambiguous. If unsure, set false.
- Bias added findings toward Lens 5 (unfinished work). Add at most 2 new findings.

Write validated findings to `findings_v2.json`.

---

## Phase 4: Report + Auto-Fix Offer

Write the report yourself in the main session. Do not spawn an agent for it. Follow these instructions:
> "Read the final `findings_v<n>.json`. Drop `verdict: REJECTED`. Group by severity (post-downgrade). Output markdown:
>
> ```
> # Local Review: $RUNID
>
> ## Important (N)
> For each important finding:
> - **`<file>:<line>`**: <claim>
>   - `<evidence>`
>   - Fix: <suggested_fix>
>   - auto-fixable (if true)
>
> ## Nits (N)
> <one line per finding, omit evidence if claim is self-evident>
>
> ## Pre-existing (N)
> <same format as Important. These never affect the verdict.>
>
> ## Skipped
> <list any skipped lenses, e.g. 'requirements (no Jira ticket found)'>
>
> ## Verdict
> <Ready to commit | Fix N important findings before committing>
> ```"

Print the report.

### Auto-fix offer

Count `important` findings where `auto_fixable: true` and `verdict != REJECTED`. Nits that are auto-fixable are optional and listed separately. If either count is above 0, ask the user with numbered options, recommended option first:
1. Apply all N auto-fixable important findings
2. Pick which findings to apply (includes auto-fixable nits)
3. Apply none

- **Apply all:** apply each fix using Edit. Re-run any fast checks (linter, typecheck) the codebase context identified. Print a diff of applied changes.
- **Pick:** list the auto-fixable findings with numbers and ask which to apply.
- **None:** stop and leave fixes for the author.

Only auto-fix findings marked `auto_fixable: true` by Phase 2 and confirmed by the validator.

---

## Decision

- 0 `important`: ready to commit (modulo any nits the user wants to address)
- 1 or more `important`: stop. The author needs to address these before pushing.

---

## No GitHub writes

Follow `../_shared/no-github-writes.md`. This skill is read-only on GitHub, and reads use the `gh` CLI. Auto-fix may modify local files with explicit user approval, but never pushes, opens or updates PRs, or posts comments. The user commits and pushes manually after reviewing any auto-fixes.
