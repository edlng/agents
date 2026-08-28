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
- `pr:$RUNID:photon_client_consistency` — evidence-backed comparisons with other clients; populated only for `awslabs/photon`
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

### 1f. Build Photon Cross-Client Context (only `awslabs/photon`)
Normalize `<owner/repo>` to lowercase. Run this phase only when it equals `awslabs/photon`; skip it for every downstream repository.

Identify the changed client and each externally observable behavior affected by the diff. For every behavior:

1. Resolve the current `main` commit with `gh api repos/awslabs/photon/commits/main --jq .sha`. Inspect other client implementations on that commit with `gh api`. Record an exact source snippet, path, line or line range, observed behavior, and an immutable link of the form `https://github.com/awslabs/photon/blob/<main_sha>/<path>#L<start>-L<end>`.
2. List open PR metadata with `gh pr list --repo awslabs/photon --state open --json number,title,body,headRefName`. Shortlist only PRs whose stated purpose plausibly concerns the same behavior. For each plausible candidate, use `gh pr view <number> --repo awslabs/photon --json files,headRefOid` to check its changed paths. Fetch its diff only when the purpose and changed paths establish relevance. Do not inspect unrelated PR diffs, and do not treat a shared keyword alone as relevance.
3. Record one verdict per compared client and behavior: `ALIGNS`, `DIVERGES`, `MIXED`, or `INSUFFICIENT_EVIDENCE`.
4. Include a stable evidence ID, compared client, behavior or contract, source type (`main` or relevant PR), commit SHA or PR number and head SHA, path and lines, exact source snippet, immutable GitHub URL, observed behavior, verdict, and reasoning.

Absence of an implementation is not divergence unless the shared contract requires that client to implement it. Unavailable files, ambiguous behavior, contradictory evidence, or a lack of relevant clients produce `INSUFFICIENT_EVIDENCE`, not a guess. Write the records to `pr:$RUNID:photon_client_consistency`.

---

## Phase 2: Initial Review

**Size gate** (use `additions + deletions` from metadata):
- `<= 500`: spawn ONE `code-reviewer` subagent, all lenses in one pass.
- `> 500`: spawn **4 `code-reviewer` subagents in parallel** — one per lens (see below). Each re-reads the same Valkey cache keys, so only use this tier when a single agent would genuinely lose the thread across 500+ lines.

**Photon context for both size paths:** For an `awslabs/photon` review, every subagent that emits findings must also read `pr:$RUNID:photon_client_consistency`. After each finding's normal evidence and reasoning, append a `client_consistency` object with `verdict`, `reasoning`, and the exact comparison-record evidence IDs. Cross-client behavior is supporting context; it does not prove the underlying finding. Use `INSUFFICIENT_EVIDENCE` when the records do not establish alignment or divergence.

**Prompt for the single-agent path (`<= 500`):**
> Read `pr:$RUNID:diff`, `pr:$RUNID:requirements`, `pr:$RUNID:codebase_context`, and `pr:$RUNID:photon_client_consistency` when present from Valkey at `localhost:8888`. If you need a file beyond the cached context, use `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d`. Do not invent file contents.
>
> Use the `code-review-excellence` skill as your reasoning frame. Apply the four lenses in `_shared/review-findings-schema.md` in order — **Codebase Alignment first** (primary lens), then Correctness & Security, then Requirements, then Testability. Only flag codebase-alignment issues that conflict with patterns visible in `pr:$RUNID:codebase_context`. Write the findings JSON to `pr:$RUNID:findings_v1`.

**For `> 500` line diffs, spawn 4 parallel subagents:**

- **Subagent A — `code-reviewer` — Codebase Alignment & Software Principles (PRIMARY):** Does the code fit this codebase? Flag reimplemented utilities, naming/casing/error-handling deviations, layering violations, premature abstraction, duplication of adjacent code, violations of SOLID/DRY/YAGNI where the codebase visibly follows them. Only flag what conflicts with patterns visible in `pr:$RUNID:codebase_context` — not general preferences.
- **Subagent B — `code-reviewer` — Correctness & Requirements:** Logic bugs, off-by-ones, race conditions, unhandled errors, broken invariants, boundary/edge cases. Plus: does the implementation satisfy every acceptance criterion from `pr:$RUNID:requirements`? Quote each criterion and mark MET or MISSING. Skip requirements if `pr:$RUNID:requirements` is absent.
- **Subagent C — `security-reviewer` — Security:** Full CWE-anchored threat model per the security-reviewer agent definition. Receives `pr:$RUNID:diff` and `pr:$RUNID:codebase_context` only — not requirements.
- **Subagent D — `tester` — Testability:** New behavior without tests, untested error paths, tests that don't assert behavior, mocks hiding real bugs, flaky patterns, missing edge cases. Only flag code that is new in this diff.
- **Subagent E — `glide-code-reviewer` — Valkey GLIDE:** Client lifecycle, batch/pipeline usage, cluster awareness, connection management, error handling, resource leaks, and GLIDE anti-patterns. Self-gates if the diff contains no GLIDE code — no findings, no cost.

After all 5 subagents complete, merge their JSON arrays, deduplicate findings on the same file+line (keep highest severity), and write to `pr:$RUNID:findings_v1`.

---

## Phase 3: Validator (skeptic pass)

Spawn ONE `validator` subagent (Opus). Pass it the merged findings from `pr:$RUNID:findings_v1` directly — do not re-read the diff. This is a single pass; no loop.

Follow the self-challenge rubric in `_shared/validator-skeptic-pass.md`. The validator reads only the findings list and the codebase context (`pr:$RUNID:codebase_context`) to verify claims — it does not re-read the full diff. Write the validated findings to `pr:$RUNID:findings_v2`.

For `awslabs/photon`, the validator must also read `pr:$RUNID:photon_client_consistency`. Verify that every client-consistency statement follows from its cited path, lines, and source snippet; that cited open PRs concern the same behavior; that links use the recorded immutable commit SHA; and that one client's behavior is not generalized to all clients. Replace an unsupported consistency conclusion with `INSUFFICIENT_EVIDENCE`. Apply the normal reject/downgrade rules to the underlying finding independently.

---

## Phase 4: Final Report (local only)

Spawn a `documenter` subagent. Pass `$RUNID`, the final findings version, and whether `pr:$RUNID:photon_client_consistency` exists.

The documenter reads these cache keys:
- `pr:$RUNID:metadata`
- `pr:$RUNID:requirements`
- `pr:$RUNID:codebase_context`
- `pr:$RUNID:findings_v<final>`
- `pr:$RUNID:photon_client_consistency` when present

Drop every `verdict: REJECTED` finding. Use the post-downgrade severity for `DOWNGRADE` findings.

### 4a. Chat report
Print the review directly in chat, with findings first and ordered by severity:

```markdown
# PR Review: <PR title>

**Action:** <Approve | Request changes | Comment only>
**Author:** <author> | **Files:** <count> | **+<additions> / -<deletions>** | **Jira:** <key or none>

## Blocking (<N>)
For each blocking finding:
- **`<file>:<line_range>` — <claim>**
  - Evidence: <exact evidence>
  - Reasoning: <why the evidence proves the claim>
  - Suggested fix: <specific fix>
  - Photon consistency: <ALIGNS | DIVERGES | MIXED | INSUFFICIENT_EVIDENCE> — <reasoning after the finding reasoning>
  - Client evidence: <immutable main or relevant-PR links with client, path, and lines>

## Recommended (<N>)
<same complete format for severity=suggestion>

## Nits (<N>)
<one line per severity=nit finding; include Photon verdict and evidence link when applicable>

## What This PR Does
<concise explanation of the implementation and its structure>

## Summary
<overall state, residual risk, and test coverage>
```

For non-Photon repositories, omit the Photon consistency and client evidence lines. For Photon findings, state the normal evidence and reasoning before the consistency verdict. Never use another client's behavior as the sole proof of a finding. If there are no accepted findings, say so explicitly and still summarize the PR and residual test risk.

Choose the action consistently:
- Any blocking finding: `Request changes`
- No blocking findings but at least one suggestion: `Comment only`
- Only nits or no findings: `Approve`

Tone: direct, factual, and non-condescending. Avoid "simply", "just", and "obviously".

### 4b. Temporary HTML report
Create one complete HTML5 document at `/tmp/pr-review-<repo>-<number>-<timestamp>.html`, where `<repo>` is the lowercase repository basename with non-alphanumeric runs replaced by `-`, and `<timestamp>` is UTC `YYYYMMDD-HHMMSS`. Do not create companion files.

The report is an evidence dashboard, ordered as follows:

1. PR title, metadata, recommended action, and severity counts.
2. "What this PR does": intent, requirements, and implementation summary.
3. Change map grouped by subsystem or directory, including each group's role.
4. Validated findings ordered by severity.
5. Test coverage, untested behavior, and residual risk.
6. Photon client consistency matrix when the cache key exists.
7. Methodology and source links.

Every non-nit finding displays its source location, claim, exact evidence, reasoning, suggested fix, validator disposition, and Photon consistency verdict/evidence when applicable. The Photon matrix displays behavior, compared client, source type, verdict, reasoning, exact source snippet, and immutable `main` or relevant-PR link. A link without path-and-line evidence is not sufficient.

Use a restrained, high-contrast technical-report design:
- System font stack, compact type, and zero letter spacing.
- Neutral page background with white content surfaces; red, amber, green, and blue reserved for distinct semantic states rather than a one-hue palette.
- Maximum content width around 1280px, a stable two-column desktop summary, and a single-column layout below 760px.
- Borders and spacing for hierarchy; card radius no greater than 8px.
- No gradients, decorative blobs, nested cards, oversized hero text, external fonts, or decorative images.
- Long paths, snippets, titles, and URLs must wrap without overlapping adjacent content.
- Print styles must remove sticky positioning and preserve evidence text and links.

The file must be portable and safe:
- Put all CSS and any optional enhancement JavaScript inline. Use no remote fonts, scripts, stylesheets, images, or other runtime assets.
- The complete report must remain readable with JavaScript disabled and when opened directly from disk.
- HTML-escape `&`, `<`, `>`, `"`, and `'` in every value originating from GitHub, Jira, source code, cache records, or findings before interpolation.
- Allow only `https://` source-link destinations. Attribute-escape URLs and add `rel="noreferrer noopener"` to links opened in a new tab.
- Do not interpolate untrusted values into `<style>`, `<script>`, event-handler attributes, or raw HTML.

After writing, verify that the file is non-empty, starts with `<!doctype html>`, contains the PR title and all accepted finding IDs, and has no external asset tags. Open it in the default browser when the environment supports that operation. If browser launch is unavailable or fails, preserve the file and provide its absolute path.

### 4c. Failure behavior
- Photon evidence fetch failure: continue the review and use `INSUFFICIENT_EVIDENCE`.
- Relevant PR read failure: identify that PR and state that its behavior could not be verified.
- Browser launch failure: preserve the generated HTML file and report its path.
- HTML generation or validation failure: still print the complete chat report and state the local-output failure.
- Never fall back to another note system and never write to GitHub.

Finish by giving the user the HTML path and the recommended action.

---

## STRICT: No GitHub writes

This command is **read-only on GitHub**. Follow `_shared/no-github-writes.md` in full. `gh` CLI is used for reads only. The user will read the output in chat and post manually if desired.
