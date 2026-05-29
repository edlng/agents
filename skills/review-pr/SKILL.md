---
name: review-pr
description: Senior-grade PR review against the linked Jira ticket and existing codebase. Uses Valkey caching, a merged-lens reviewer (sonnet-4-6), and an Opus skeptic validator pass. Output is local-only — printed in chat and saved to Obsidian, never posted to GitHub.
---

# Review PR (senior-dev grade)

Review someone else's pull request against the linked Jira ticket and the existing codebase. Optimized for **signal over volume** — every finding that reaches the final report has been validated by an Opus skeptic pass, so false positives are rare.

`$ARGUMENTS` is a PR URL or `owner/repo#number`. If empty, use `mcp__github__list_pull_requests` (or `mcp__github__search_pull_requests` with `review-requested:@me`) and ask which to review.

**Output is local-only.** Print the review in chat. **DO NOT** post comments, reviews, approvals, or any write operation to GitHub. The user will copy and post manually after reviewing the output.

This workflow uses Valkey at `localhost:8888` as a shared cache. **All GitHub interactions go through the GitHub MCP server (`mcp__github__*` tools) only.** Do not use the `gh` CLI for anything — not reads, not writes. If a needed operation is not available via MCP, stop and tell the user instead of falling back to `gh`.

---

## Cache setup

Cache keys for this run (TTL 6h):
- `pr:$RUNID:diff` — full unified diff
- `pr:$RUNID:metadata` — PR title, body, branches, author, file list, additions/deletions
- `pr:$RUNID:requirements` — Requirements Document (Jira + PR description)
- `pr:$RUNID:codebase_context` — patterns/conventions of touched files
- `pr:$RUNID:findings_v<n>` — findings JSON, versioned per validator pass

`$RUNID` = `<owner>-<repo>-<pr-number>`. Subagents read via `valkey-cli -p 8888 GET <key>` or `valkey-glide`. Mark cached values as Anthropic prompt-cached prefixes when inlined.

---

## Phase 1: Context Gathering

**Model: sonnet-4-6** (single agent — do not spawn for this phase)

### 1a. Fetch PR metadata via GitHub MCP
Use `mcp__github__pull_request_read` with method `get` for the PR identified by `$ARGUMENTS`. Capture: title, body, head ref, base ref, author, additions, deletions, changed files.

If `additions + deletions > 1500`, warn the user that the review will be lossy and ask whether to proceed or scope it down.

Cache to `pr:$RUNID:metadata`.

### 1b. Identify the Jira issue
Extract the Jira issue key from the PR title, body, or branch name. If none is found, search `mcp__atlassian__searchJiraIssuesUsingJql` for tickets the PR likely references (use keywords from branch name and title). If no confident match, list top 3 candidates and ask the user. If the user says "no ticket", proceed with PR description as the only requirements source.

### 1c. Snapshot the diff
Use `mcp__github__pull_request_read` with method `get_diff` (or the equivalent diff-fetching method exposed by the MCP) to retrieve the full unified diff for the PR.

If the MCP server does not expose a diff method directly, use `mcp__github__pull_request_read` with `get_files` to get the per-file patches and concatenate them into a unified diff format.

Write to `pr:$RUNID:diff`.

### 1d. Build Requirements Document
Use `mcp__atlassian__getJiraIssue` for the linked issue (if any). Combine with the PR body. Extract:
1. What must be built
2. Explicit acceptance criteria (infer if absent)
3. Edge cases / constraints
4. Implicit constraints (security, perf, compatibility)

Write to `pr:$RUNID:requirements`.

### 1e. Build Codebase Context
For each touched file, fetch its current state from the PR's head ref using `mcp__github__get_file_contents` (specify the head ref so file contents reflect the PR's branch). Also fetch 1–2 callers/neighbors of the most non-trivial touched files.

**Do NOT check out the PR locally. Do NOT modify the user's working tree.** Reads are GitHub MCP only.

Capture:
- Language, runtime, package manager
- Naming conventions, error-handling patterns, logging style, type annotation usage
- Existing utilities/helpers the new code should call rather than re-implement
- Test framework, fixture patterns, assertion style
- If a key/value store is in use: `valkey-glide` is the standard — flag any introduction of `redis-py` or other clients

Write to `pr:$RUNID:codebase_context`.

---

## Phase 2: Initial Review (merged lenses)

**Model: sonnet-4-6**

Spawn ONE reviewer subagent with model `sonnet-4-6`. The subagent applies all four lenses in one pass — diff loaded once, not four times. Use the `code-review-excellence` skill as the reasoning frame for lenses 2 and 3.

Prompt:
> "You are a senior reviewer. Read these from Valkey at `localhost:8888`:
>   - `pr:$RUNID:diff`
>   - `pr:$RUNID:requirements`
>   - `pr:$RUNID:codebase_context`
>
> If you need to inspect a file beyond what's in the codebase context cache, use `mcp__github__get_file_contents` against the PR head ref. Do not invent file contents.
>
> Apply four lenses to the diff in a single pass. Output strict JSON: a flat array of findings. Each finding has these fields:
>   - `id`: short slug, e.g. `auth-missing-rate-limit`
>   - `lens`: one of `requirements` | `correctness_security` | `design_fit` | `testability`
>   - `file`: path
>   - `line_range`: e.g. `42-58`
>   - `severity`: `blocking` | `suggestion` | `nit`
>   - `claim`: one sentence — what is wrong
>   - `evidence`: one or two lines from the diff or codebase context that prove the claim. **If you cannot quote evidence, do not file the finding.**
>   - `suggested_fix`: concrete change
>
> ### Lens 1 — Requirements alignment
> Does the implementation satisfy every acceptance criterion? Cite the requirement item by quoting it. Flag missing or partial coverage. Flag scope creep (changes unrelated to any requirement).
>
> ### Lens 2 — Correctness & Security
> Logic bugs, off-by-ones, race conditions, unhandled errors, dropped exceptions, broken invariants. Security: injection (SQL, command, template), authn/authz gaps, secrets in code, unsafe deserialization, SSRF, path traversal, weak crypto, missing input validation at trust boundaries. Be concrete about the threat model — generic 'add validation' findings are rejected by the validator.
>
> ### Lens 3 — Design fit
> Does the code match the codebase context? Reused existing utilities? Premature abstraction? Duplication of nearby code? Naming and error-handling consistent? Layering respected (e.g. data access not bleeding into HTTP handlers)? Do not flag stylistic differences that are only matters of taste — only flag what conflicts with patterns visible in the codebase context.
>
> ### Lens 4 — Testability
> Do tests cover every acceptance criterion and the edge cases listed in requirements? Are tests asserting behavior or just exercising code paths? Mocked dependencies that should be real (e.g. mocked DB when an integration test would catch the bug)? Missing tests for new failure paths?
>
> IMPORTANT: Report only gaps that affect correctness, security, or stated requirements. Do NOT file findings that are purely matters of taste or style unless they directly conflict with a codebase pattern visible in pr:$RUNID:codebase_context.
>
> Rules for filing a finding:
>   - Only flag what is in the diff, except where context is essential to prove a diff issue (e.g. a caller breaks because of a signature change in the diff).
>   - Cite a real symbol, file, line. Do NOT invent function names or files.
>   - Severity rubric:
>     - `blocking`: violates a requirement, introduces a real security/correctness bug, breaks an interface, or causes test failures.
>     - `suggestion`: real improvement (perf, clarity, robustness) that does not block merge.
>     - `nit`: pure style/naming.
>   - If you are not sure something is wrong, omit it. The validator pass will not rescue you — it will downgrade or reject it.
>
> Write the JSON to `pr:$RUNID:findings_v1` in Valkey."

---

## Phase 3: Validator (skeptic pass)

**Model: claude-opus-4-8**

This is where Opus earns its cost — it kills false positives that would otherwise reach the user.

Spawn one validator subagent with model `claude-opus-4-8`. Pass `$RUNID` and `n` (current findings version).

Prompt:
> "You are a skeptical senior engineer doing a second pass on another reviewer's findings. Your job is to maximize signal: confirm what is real, downgrade what is overstated, reject what is false, and add only high-confidence misses.
>
> Read from Valkey at `localhost:8888`:
>   - `pr:$RUNID:findings_v$n`
>   - `pr:$RUNID:diff`
>   - `pr:$RUNID:requirements`
>   - `pr:$RUNID:codebase_context`
>
> If you need to verify a claim against a file, use `mcp__github__get_file_contents` against the PR head ref. Do NOT trust the finding's evidence blindly — re-read the source if anything looks off.
>
> For each finding in v$n, attach `verdict` (`CONFIRMED` | `DOWNGRADE` | `REJECTED`), `verdict_reason` (one sentence), and if `DOWNGRADE` also a new `severity`. Reject the finding if any of:
>   - The cited symbol/file/line does not exist or does not say what the finding claims (hallucinated evidence).
>   - The 'bug' is already handled elsewhere in the diff or in the codebase context.
>   - The finding is generic ('add error handling', 'add validation') without a concrete failure scenario.
>   - The finding is a matter of taste, not a deviation from the codebase context.
>   - The finding is outside the diff and not load-bearing for a diff change.
>
> Downgrade if the issue is real but the severity is overstated relative to the rubric in Phase 2.
>
> Then independently scan the diff for HIGH-CONFIDENCE misses. Add at most 3 new findings, only if you have direct evidence and the issue is at least `suggestion` severity. Do NOT pad. If you have nothing to add, add nothing.
>
> Output: full updated findings array (original + verdict fields, plus any added findings with `lens: 'validator_added'`). Write to `pr:$RUNID:findings_v$(n+1)`.
>
> Also output two top-level numbers used by the loop controller:
>   - `reject_rate`: rejected_count / total_v$n_count
>   - `added_count`: number of new findings you added"

### Loop control
Read `reject_rate` and `added_count` from the validator output.
- If `reject_rate >= 0.30` OR `added_count >= 2`: re-run **Phase 3** on the latest version.
- Else: converged — proceed to Phase 4.

Cap at **3 validator passes total**. In practice 1 pass is enough; 2 is the worst common case.

---

## Phase 4: Final Report (local only)

**Model: haiku-4-5**

Spawn a summary subagent with model `haiku-4-5`. Pass `$RUNID` and the final findings version.

Prompt:
> "Read `pr:$RUNID:findings_v<final>` from Valkey. Drop all `verdict: REJECTED` findings entirely. Group remaining findings by severity (use the post-downgrade severity if `DOWNGRADE`). Produce markdown:
>
> ```
> # Review: $RUNID  (<PR title>)
>
> Author: <author>  |  Files: <count>  |  +<additions> / -<deletions>
> Linked Jira: <key or 'none'>
>
> ## Blocking
> For each blocking finding:
> - **<file>:<line_range>** — <claim>
>   - Evidence: <evidence>
>   - Fix: <suggested_fix>
>
> ## Recommended
> <same format, severity=suggestion>
>
> ## Nits
> <same format, severity=nit — keep these terse>
>
> ## Summary
> <one paragraph — overall state of the PR>
>
> ## Suggested PR action
> <one of: approve / request changes / comment only — based on findings>
> ```
>
> Tone: this is for the user to decide whether to post. Be direct but not condescending. State facts, not judgments. Avoid 'simply', 'just', 'obviously'. No emojis."

Print the final markdown directly in chat.

After printing, save the same markdown as an Obsidian note using `mcp__obsidian__write_note`:
- Path: `PRs/<repo>-<PR title>-<author>.md` (sanitize: lowercase, replace spaces and `/`, `\`, `:`, `*`, `?`, `"`, `<`, `>`, `|` with `-`, collapse consecutive `-` into one)
- Content: the full markdown output

Confirm to the user that the note was saved, including the path used.

---

## STRICT: No GitHub writes

This command is **read-only on GitHub** AND **MCP-only**. Do NOT:
- Use the `gh` CLI for anything (no `gh pr view`, `gh pr diff`, `gh pr review`, `gh pr comment`, etc.). All GitHub access is via `mcp__github__*` tools.
- Post a review (`mcp__github__pull_request_review_write`)
- Add a comment (`mcp__github__add_issue_comment`, `mcp__github__add_comment_to_pending_review`)
- Approve or request changes
- Update PR title/body (`mcp__github__update_pull_request`)
- Push to any branch (`mcp__github__push_files`, `mcp__github__create_or_update_file`)
- Any other write method (`*_write`, `create_*`, `update_*`, `merge_*`, `delete_*`)

The user will read the output in chat and post manually if desired.
