---
name: review-cookbook-pr
description: Senior-grade PR review for cookbook PRs. Same as review-pr but adds two cookbook-specific checks: (1) code snippets call project functions directly rather than simulating Valkey internals, and (2) valkey/valkey-bundle version tags are verified against Docker Hub.
---

# Review Cookbook PR (senior-dev grade)

Review a cookbook pull request against the linked Jira ticket and the existing codebase. Optimized for **signal over volume** — every finding that reaches the final report has been validated by an Opus skeptic pass, so false positives are rare.

`$ARGUMENTS` is a PR URL or `owner/repo#number`. If empty, use `mcp__github__list_pull_requests` (or `mcp__github__search_pull_requests` with `review-requested:@me`) and ask which to review.

**Output is local-only.** Print the review in chat. **DO NOT** post comments, reviews, approvals, or any write operation to GitHub. The user will copy and post manually after reviewing the output.

This workflow uses Valkey at `localhost:8888` as a shared cache. **GitHub reads use the `gh` CLI** (faster, fewer round-trips than MCP tool calls). Do not use `mcp__github__*` tools for reads — use `gh` directly. Do NOT use `gh` for writes (no `gh pr review`, `gh pr comment`, etc.). Output is read-only.

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

**Model: latest Sonnet** (single agent — do not spawn for this phase)

### 1a. Fetch PR metadata via gh CLI
Run: `gh pr view <number> --repo <owner/repo> --json title,body,headRefName,baseRefName,author,additions,deletions,changedFiles,files`

Capture: title, body, head ref, base ref, author, additions, deletions, changed files.

If `additions + deletions > 1500`, warn the user that the review will be lossy and ask whether to proceed or scope it down.

Cache to `pr:$RUNID:metadata`.

### 1b. Identify the Jira issue
Extract the Jira issue key from the PR title, body, or branch name. If none is found, search `mcp__atlassian__searchJiraIssuesUsingJql` for tickets the PR likely references (use keywords from branch name and title). If no confident match, list top 3 candidates and ask the user. If the user says "no ticket", proceed with PR description as the only requirements source.

### 1c. Snapshot the diff
Run: `gh pr diff <number> --repo <owner/repo>`

This returns the full unified diff in a single call.

Write to `pr:$RUNID:diff`.

### 1d. Build Requirements Document
Use `mcp__atlassian__getJiraIssue` for the linked issue (if any). Combine with the PR body. Extract:
1. What must be built
2. Explicit acceptance criteria (infer if absent)
3. Edge cases / constraints
4. Implicit constraints (security, perf, compatibility)

Write to `pr:$RUNID:requirements`.

### 1e. Build Codebase Context
For each touched file, fetch its current state from the PR's head ref using `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d` or `gh pr checkout <number> --detach` if bulk reads are needed. Also fetch 1-2 callers/neighbors of the most non-trivial touched files.

**Do NOT modify the user's working tree.** If using `gh pr checkout`, do so in a temp worktree or use the API approach above.

Capture:
- Language, runtime, package manager
- Naming conventions, error-handling patterns, logging style, type annotation usage
- Existing utilities/helpers the new code should call rather than re-implement
- Test framework, fixture patterns, assertion style
- If a key/value store is in use: `valkey-glide` is the standard — flag any introduction of `redis-py`, `ioredis`, or other clients unless there's a documented reason
- When using valkey-glide: prefer `Batch` (or `ClusterBatch`) for multi-command operations over external concurrency libs like `asyncio.gather`, `Promise.all` with individual commands, or pipeline wrappers — Batch is atomic, reduces round-trips, and is the idiomatic GLIDE pattern

Write to `pr:$RUNID:codebase_context`.

---

## Phase 2: Initial Review (merged lenses)

Spawn ONE `code-reviewer` subagent. The subagent applies all four lenses in one pass — diff loaded once, not four times. Use the `code-review-excellence` skill as the reasoning frame for lenses 2 and 3.

Prompt:
> "You are a senior reviewer. Read these from Valkey at `localhost:8888`:
>   - `pr:$RUNID:diff`
>   - `pr:$RUNID:requirements`
>   - `pr:$RUNID:codebase_context`
>
> If you need to inspect a file beyond what's in the codebase context cache, use `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d`. Do not invent file contents.
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

## Phase 2b: Cookbook-Specific Checks

**Run in parallel with Phase 2** (spawn a separate subagent for this phase). These checks are specific to cookbook PRs and must run against the diff from `pr:$RUNID:diff`.

Spawn ONE `code-reviewer` subagent for cookbook checks.

Prompt:
> "You are reviewing a cookbook PR for two specific quality issues. Read `pr:$RUNID:diff` from Valkey at `localhost:8888`.
>
> ## Check 1: Code snippets use project functions, not Valkey simulations
>
> Cookbook code snippets must demonstrate how to USE the project's API — they should call project-level functions and methods directly (e.g. `agent.receive()`, `client.search()`, `store.set()`). They must NOT simulate or narrate Valkey internals inline in the snippet.
>
> **Prohibited pattern:** A code comment that says something like `# this simulates ft.create(...)`, `# internally calls FT.SEARCH`, `# this is equivalent to ZADD ...`, or any comment that maps a project function call to a raw Valkey/Redis command as if the snippet is an explanation of the implementation detail.
>
> **Allowed exception:** Explanatory prose OUTSIDE the code snippet (in markdown paragraphs) may describe how a function works under the hood, including Valkey commands it uses. Comments inside a code snippet that explain behavior (e.g. `# returns all results matching the filter`) are fine as long as they do NOT use raw Valkey command syntax to simulate what the function does.
>
> For each code snippet in the diff that violates this, file a finding with:
>   - `id`: `cookbook-simulates-valkey-<slug>`
>   - `lens`: `cookbook_api_usage`
>   - `file`, `line_range`, `severity`: `blocking`
>   - `claim`: Quote the offending comment and explain why it should be removed or moved to prose.
>   - `evidence`: The exact line(s) from the diff.
>   - `suggested_fix`: Either remove the comment, rewrite it to describe behavior without using raw Valkey syntax, or move it to a prose paragraph outside the code snippet.
>
> ## Check 2: valkey/valkey-bundle version accuracy
>
> Scan the diff for any references to `valkey/valkey-bundle` with a specific version tag (e.g. `valkey/valkey-bundle:8.1.1`, `valkey/valkey-bundle:latest`). For each versioned reference found:
>
> 1. If the tag is `latest`, no Docker Hub check is needed — `latest` is always valid.
> 2. If the tag is a specific version number (e.g. `8.1.1`), verify it exists on Docker Hub by running:
>    `curl -s "https://hub.docker.com/v2/repositories/valkey/valkey-bundle/tags/?name=<version>" | python3 -m json.tool`
>    or: `curl -s "https://hub.docker.com/v2/repositories/valkey/valkey-bundle/tags/" | python3 -c "import sys,json; tags=[t['name'] for t in json.load(sys.stdin)['results']]; print(tags)"`
>    Confirm the tag appears in the results.
> 3. If the PR mentions a specific feature version included in the image (e.g. 'uses valkey-search 1.2'), cross-check that claim against the Docker Hub tag's listed description or release notes. If the Docker Hub page does not confirm the feature version, file a finding.
>
> For each version accuracy issue found, file a finding with:
>   - `id`: `cookbook-valkey-bundle-version-<slug>`
>   - `lens`: `cookbook_version_accuracy`
>   - `file`, `line_range`, `severity`: `blocking` if the tag does not exist; `suggestion` if the feature version claim is unverifiable.
>   - `claim`: What was claimed vs. what Docker Hub shows.
>   - `evidence`: The exact line(s) from the diff.
>   - `suggested_fix`: Use the correct tag, or remove/qualify the unverifiable feature version claim.
>
> Output the findings as a JSON array (same schema as the main review). Write to `pr:$RUNID:findings_cookbook` in Valkey."

After both Phase 2 and Phase 2b complete, merge the two findings arrays into `pr:$RUNID:findings_v1` (appending cookbook findings to the main findings) before proceeding to Phase 3.

---

## Phase 3: Validator (skeptic pass)

**Model selection by diff size:**
- If `additions + deletions <= 300`: **skip Phase 3 entirely** — the code-reviewer is sufficient for small diffs. Proceed directly to Phase 4 using `findings_v1`.
- If `additions + deletions <= 800`: spawn a `code-reviewer` subagent as the validator (cheaper, still catches hallucinations).
- If `additions + deletions > 800`: spawn a `validator` subagent (earns its cost on large/complex diffs).

This is where the validator earns its cost — it kills false positives that would otherwise reach the user.

Spawn one subagent with the role selected above. Pass `$RUNID` and `n` (current findings version).

Prompt:
> "You are a skeptical senior engineer doing a second pass on another reviewer's findings. Your job is to maximize signal: confirm what is real, downgrade what is overstated, reject what is false, and add only high-confidence misses.
>
> **Effort budget: 10-20 tool calls max. Read only the files needed to verify claims — do not re-review the entire diff.**
>
> Read from Valkey at `localhost:8888`:
>   - `pr:$RUNID:findings_v$n`
>   - `pr:$RUNID:diff`
>   - `pr:$RUNID:requirements`
>   - `pr:$RUNID:codebase_context`
>
> If you need to verify a claim against a file, use `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d`. Do NOT trust the finding's evidence blindly — re-read the source if anything looks off.
>
> For each finding, apply this self-challenge before deciding your verdict:
> 1. Can I point to the exact line in the diff that proves this claim?
> 2. Did I verify the issue isn't already handled elsewhere in the diff or codebase?
> 3. Would a concrete input/scenario actually trigger this failure?
>
> For cookbook-specific findings (lens `cookbook_api_usage` or `cookbook_version_accuracy`), also apply:
> 4. For `cookbook_api_usage`: Does the quoted comment actually use raw Valkey command syntax to simulate a function, or is it just explaining behavior in plain English?
> 5. For `cookbook_version_accuracy`: Did the reviewer actually run the Docker Hub curl check, or is this a guess? If the tag was not verified, mark it DOWNGRADE (suggestion) unless the tag clearly does not follow Docker Hub's known version format.
>
> Then attach `verdict` (`CONFIRMED` | `DOWNGRADE` | `REJECTED`), `verdict_reason` (one sentence), and if `DOWNGRADE` also a new `severity`. Reject the finding if any of:
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

This command is **read-only on GitHub**. `gh` CLI is used for reads (view, diff, API GET). Do NOT:
- Post a review (`gh pr review`, `mcp__github__pull_request_review_write`)
- Add a comment (`gh pr comment`, `mcp__github__add_issue_comment`)
- Approve or request changes
- Update PR title/body
- Push to any branch
- Any other write method

The user will read the output in chat and post manually if desired.
