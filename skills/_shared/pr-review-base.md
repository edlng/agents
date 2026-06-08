# Shared: PR Review Base Workflow

> Shared reference used by `review-pr` and `review-cookbook-pr`. Not a standalone skill. Single source of truth for the senior-grade PR review workflow (context gathering, merged-lens review, validator pass, local report). Consuming skills invoke this base and add their own specialized phases (e.g. cookbook-specific checks).

Review a pull request against the linked Jira ticket and the existing codebase. Optimized for **signal over volume** — every finding that reaches the final report has been validated by a skeptic pass, so false positives are rare.

`$ARGUMENTS` is a PR URL or `owner/repo#number`. If empty, use `mcp__github__list_pull_requests` (or `mcp__github__search_pull_requests` with `review-requested:@me`) and ask which to review.

**GitHub reads use the `gh` CLI** (faster, fewer round-trips than MCP tool calls). Do not use `mcp__github__*` tools for reads — use `gh` directly. Output is read-only: follow `_shared/no-github-writes.md`.

Caching follows `_shared/valkey-cache-conventions.md`.

---

## Cache setup

Cache keys for this run (TTL 6h), where `$RUNID` = `<owner>-<repo>-<pr-number>`:
- `pr:$RUNID:diff` — full unified diff
- `pr:$RUNID:metadata` — PR title, body, branches, author, file list, additions/deletions
- `pr:$RUNID:requirements` — Requirements Document (Jira + PR description)
- `pr:$RUNID:codebase_context` — patterns/conventions of touched files
- `pr:$RUNID:findings_v<n>` — findings JSON, versioned per validator pass

---

## Phase 1: Context Gathering

**Model: latest Sonnet** (single agent — do not spawn for this phase)

### 1a. Fetch PR metadata via gh CLI
Run: `gh pr view <number> --repo <owner/repo> --json title,body,headRefName,baseRefName,author,additions,deletions,changedFiles,files`

Capture title, body, head ref, base ref, author, additions, deletions, changed files. If `additions + deletions > 1500`, warn the user that the review will be lossy and ask whether to proceed or scope it down. Cache to `pr:$RUNID:metadata`.

### 1b. Identify the Jira issue
Extract the Jira issue key from the PR title, body, or branch name. If none is found, search `mcp__atlassian__searchJiraIssuesUsingJql` for likely tickets (keywords from branch name and title). If no confident match, list top 3 candidates and ask. If the user says "no ticket", proceed with the PR description as the only requirements source.

### 1c. Snapshot the diff
Run: `gh pr diff <number> --repo <owner/repo>` and write the result to `pr:$RUNID:diff`.

### 1d. Build Requirements Document
Use `mcp__atlassian__getJiraIssue` for the linked issue (if any). Combine with the PR body. Extract: (1) what must be built, (2) explicit acceptance criteria (infer if absent), (3) edge cases / constraints, (4) implicit constraints (security, perf, compatibility). Write to `pr:$RUNID:requirements`.

### 1e. Build Codebase Context
Follow `_shared/codebase-context-checklist.md`. For each touched file, fetch its current state from the PR's head ref using `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d` (or `gh pr checkout <number> --detach` in a temp worktree if bulk reads are needed — do NOT modify the user's working tree). Write to `pr:$RUNID:codebase_context`.

---

## Phase 2: Initial Review (merged lenses)

Spawn ONE `code-reviewer` subagent. It applies all lenses in one pass — diff loaded once, not once per lens. Use the `code-review-excellence` skill as the reasoning frame for the correctness/security and design lenses.

Prompt the subagent to:
> Read `pr:$RUNID:diff`, `pr:$RUNID:requirements`, and `pr:$RUNID:codebase_context` from Valkey at `localhost:8888`. If you need a file beyond the cached context, use `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d`. Do not invent file contents.
>
> Apply the four lenses and the findings schema defined in `_shared/review-findings-schema.md`. Write the JSON to `pr:$RUNID:findings_v1`.

---

## Phase 3: Validator (skeptic pass)

Follow `_shared/validator-skeptic-pass.md`. Use its diff-size model-selection rubric (skip at `<= 300`; `code-reviewer` at `<= 800`; `validator` at `> 800`). Pass `$RUNID` and `n` (current findings version); the validator writes `pr:$RUNID:findings_v$(n+1)`. Apply the loop control from that file; cap at **3 validator passes total**.

---

## Phase 4: Final Report (local only)

Spawn a `documenter` subagent. Pass `$RUNID` and the final findings version.

Prompt:
> "Read `pr:$RUNID:findings_v<final>` from Valkey. Drop all `verdict: REJECTED` findings entirely. Group remaining findings by severity (use the post-downgrade severity if `DOWNGRADE`). Produce markdown:
>
> ```
> # PR Review: <PR title>
>
> **Author:** <author> | **Files:** <count> | **+<additions> / -<deletions>** | **Jira:** <key or none>
>
> ---
>
> ## Blocking (<N>)
> For each blocking finding:
> - **`<file>:<line_range>`** — <claim>
>   - `<evidence>`
>   - <suggested_fix>
>
> ## Recommended (<N>)
> <same format, severity=suggestion>
>
> ## Nits (<N>)
> <same format, severity=nit — one line per finding, omit evidence>
>
> ---
>
> ## Summary
> <one paragraph — overall state of the PR>
>
> ## Action
> <Approve | Request changes | Comment only>
> ```
>
> Tone: this is for the user to decide whether to post. Be direct but not condescending. State facts, not judgments. Avoid 'simply', 'just', 'obviously'."

Print the final markdown directly in chat.

After printing, save the same markdown as an Obsidian note using `mcp__obsidian__write_note`:
- Path: `PRs/<repo>-<PR title>-<author>.md` (sanitize: lowercase, replace spaces and `/`, `\`, `:`, `*`, `?`, `"`, `<`, `>`, `|` with `-`, collapse consecutive `-` into one)
- Content: the full markdown output

Confirm to the user that the note was saved, including the path used.

---

## STRICT: No GitHub writes

This command is **read-only on GitHub**. Follow `_shared/no-github-writes.md` in full. `gh` CLI is used for reads only. The user will read the output in chat and post manually if desired.
