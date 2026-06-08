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

**All GitHub interactions go through the GitHub MCP server (`mcp__github__*` tools) only.** Do not use the `gh` CLI for anything — not reads, not writes. Local `git` is fine for inspecting your working tree (diff, log, branch).

Caching follows `_shared/valkey-cache-conventions.md`. Output is local-only and read-only on GitHub — follow `_shared/no-github-writes.md` (note: auto-fix MAY modify local files with explicit user approval, but never touches GitHub).

---

## Cache setup

Cache keys (TTL 6h), where `$RUNID` = current branch name:
- `local:$RUNID:diff`
- `local:$RUNID:requirements` (only if a Jira ticket is identified)
- `local:$RUNID:codebase_context`
- `local:$RUNID:findings_v<n>`

---

## Phase 1: Context Gathering

**Model: latest Sonnet** (single agent, no spawn)

### 1a. Determine the diff scope
- If `$ARGUMENTS` is `branch`: `git merge-base HEAD origin/main` (or `master`), then `git diff <merge-base>...HEAD`.
- Otherwise (default): `git diff --cached HEAD; git diff HEAD` combined.

If the diff is empty, stop and say "no local changes to review." If `additions + deletions > 1500`, warn — self-review at this size is lossy. Suggest scoping with `$ARGUMENTS=branch` or splitting. Continue only if the user confirms. Write to `local:$RUNID:diff`.

### 1b. Identify requirements source (best-effort, do not block)
- If `$ARGUMENTS` is a Jira issue key: use it directly via `mcp__atlassian__getJiraIssue`.
- Else, search Jira and try to match; else scan recent commit messages for an issue key.
- If still nothing: **skip the requirements lens entirely** — local self-review is often pre-ticket exploration. Note this in the final report. Do NOT prompt the user to find a ticket.

If a ticket is found, write the Requirements Document to `local:$RUNID:requirements`.

### 1c. Codebase Context
Build per `_shared/codebase-context-checklist.md`, reading touched files at the **current working-tree state**. Write to `local:$RUNID:codebase_context`.

---

## Phase 2: Self-Review (merged lenses)

Spawn ONE `code-reviewer` subagent. Apply lenses in a single pass. Use the `code-review-excellence` skill as the reasoning frame.

Use the findings schema and the four core lenses from `_shared/review-findings-schema.md`, with these **local-review overrides**:

- **Severity vocabulary:** `must_fix` | `should_fix` | `consider` (instead of blocking/suggestion/nit).
  - `must_fix`: real bug, security issue, broken acceptance criterion, debug code left in.
  - `should_fix`: design issue, missing test, unfinished cleanup.
  - `consider`: stylistic, judgment-call refactors.
- **Extra field:** `auto_fixable`: boolean — true only if the fix is small (≤10 lines), local to one file, and unambiguous.
- **Extra Lens 5 — Unfinished work (LOCAL-ONLY lens):** this is what self-review catches that PR review can't. Flag: `TODO`/`FIXME`/`XXX`/`HACK` left in the diff; commented-out code; `console.log`/`print()`/`dbg!()`/`pp` debug statements; hardcoded test values (hardcoded user IDs, dummy keys, `localhost:8888` outside Valkey config); empty catch/except blocks; stubbed functions returning placeholders; missing or unused imports.

Tell the subagent: "You are reviewing the author's own uncommitted work. Be direct, no diplomatic softening. Read `local:$RUNID:diff`, `local:$RUNID:requirements` (may not exist — if missing, skip Lens 1), and `local:$RUNID:codebase_context` from Valkey at `localhost:8888`. Report only gaps that affect correctness, security, or stated requirements — not matters of taste unless they conflict with a visible codebase pattern. Write JSON to `local:$RUNID:findings_v1`."

---

## Phase 3: Validator (skeptic pass)

Follow `_shared/validator-skeptic-pass.md` with a `validator` subagent. Apply these local-review specifics on top of the shared template:
- Re-evaluate `auto_fixable`: only true if the fix is small, local, and unambiguous. If unsure, set false.
- Bias added findings toward Lens 5 (unfinished work) — that is the highest-value lens for local review. Add at most 2.
- **Cap at 2 validator passes** — local review should be fast. If findings are still volatile after 2 passes, ship the current set with a note.

---

## Phase 4: Report + Auto-Fix Offer

Spawn a `documenter` subagent for report aggregation.

Prompt:
> "Read `local:$RUNID:findings_v<final>`. Drop `verdict: REJECTED`. Group by severity (post-downgrade). Output markdown:
>
> ```
> # 🔍 Local Review: $RUNID
>
> ## 🔴 Must Fix (N)
> For each must_fix:
> - **`<file>:<line>`** — <claim>
>   - `<evidence>`
>   - → <suggested_fix>
>   - ⚡ *auto-fixable* (if true)
>
> ## 🟡 Should Fix (N)
> <same format>
>
> ## 💭 Consider (N)
> <same format, one line per finding — omit evidence if claim is self-evident>
>
> ## ℹ️ Skipped
> <list any skipped lenses, e.g. 'requirements (no Jira ticket found)'>
>
> ## Verdict
> <✅ Ready to commit | ⚠️ Fix N items before committing | 🚫 Needs significant work>
> ```"

Print the report.

### Auto-fix offer

Count findings where `auto_fixable: true` AND `verdict != REJECTED`. If count > 0, ask:
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

Follow `_shared/no-github-writes.md`. This command is read-only on GitHub AND **MCP-only** (no `gh` CLI for anything). Auto-fix may modify local files with explicit user approval, but never pushes, opens/updates/comments on PRs, or posts issue comments. The user commits and pushes manually after reviewing any auto-fixes.
