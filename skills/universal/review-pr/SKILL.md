---
name: review-pr
description: "Reviews a pull request against the linked Jira ticket and the existing codebase. Parallel reviewers per discipline, findings merged and checked by a skeptic validator pass. Output is local only: printed in chat, rendered as a temporary self-contained HTML report, and saved as a findings JSON file. Nothing is posted to GitHub. Use when asked to review someone else's PR by URL or owner/repo#number."
---

# Review PR (senior-dev grade)

Review a pull request against the linked Jira ticket and the existing codebase. The review favors signal over volume. A skeptic pass validates every finding that reaches the final report, so false positives are rare.

`$ARGUMENTS` is a PR URL or `owner/repo#number`. If empty, run `gh pr list --search "review-requested:@me" --json number,title,headRepository` (or `gh search prs`) and ask the user which PR to review.

GitHub reads use the `gh` CLI. The review is read-only on GitHub; follow `../_shared/no-github-writes.md`.

---

## Run directory

All phases share files in `/tmp/agent-runs/<RUNID>/`, where `RUNID` is `<owner>-<repo>-<pr-number>`. Create it with `mkdir -p`. Subagents read these files with their file-reading tool. If a file is missing, regenerate it.

- `diff.patch`: full unified diff
- `metadata.json`: PR title, body, branches, author, file list, additions and deletions
- `requirements.md`: Requirements Document (Jira plus PR description)
- `codebase_context.md`: patterns and conventions of the touched files
- `photon_client_consistency.json`: evidence-backed comparisons with other clients, written only for `awslabs/photon`
- `findings_v<n>.json`: findings, versioned per validator pass

---

## Phase 1: Context Gathering

Run this phase in the main session. Do not spawn agents for it.

### 1a. Fetch PR metadata via gh CLI
Run: `gh pr view <number> --repo <owner/repo> --json title,body,headRefName,headRefOid,baseRefName,author,additions,deletions,changedFiles,files,url`

Capture title, body, head ref, head SHA, base ref, author, additions, deletions, changed files. If `additions + deletions > 1500`, warn the user that the review will be lossy and ask whether to proceed or scope it down. Write to `metadata.json`.

### 1b. Identify the Jira issue
Extract the Jira issue key from the PR title, body, or branch name. If none is found, search with the Jira MCP `searchJiraIssuesUsingJql` tool for likely tickets (keywords from branch name and title). If no confident match, list the top 3 candidates and ask. If the user says "no ticket", proceed with the PR description as the only requirements source.

### 1c. Snapshot the diff
Run `gh pr diff <number> --repo <owner/repo>` and write the result to `diff.patch`.

### 1d. Build Requirements Document
Use the Jira MCP `getJiraIssue` tool for the linked issue (if any). Combine with the PR body. Extract: (1) what must be built, (2) explicit acceptance criteria (infer if absent), (3) edge cases and constraints, (4) implicit constraints (security, performance, compatibility). Write to `requirements.md`.

### 1e. Build Codebase Context
Follow `../_shared/codebase-context-checklist.md`. For each touched file, fetch its current state from the PR's head ref with `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d`. For bulk reads, use `gh pr checkout <number> --detach` in a temp worktree. Do not modify the user's working tree. Write to `codebase_context.md`.

### 1f. Build Photon Cross-Client Context (only `awslabs/photon`)
Normalize `<owner/repo>` to lowercase. Run this phase only when it equals `awslabs/photon`. Skip it for every downstream repository.

Identify the changed client and each externally observable behavior affected by the diff. For every behavior:

1. Resolve the current `main` commit with `gh api repos/awslabs/photon/commits/main --jq .sha`. Inspect other client implementations on that commit with `gh api`. Record an exact source snippet, path, line or line range, observed behavior, and an immutable link of the form `https://github.com/awslabs/photon/blob/<main_sha>/<path>#L<start>-L<end>`.
2. List open PR metadata with `gh pr list --repo awslabs/photon --state open --json number,title,body,headRefName`. Shortlist only PRs whose stated purpose plausibly concerns the same behavior. For each plausible candidate, use `gh pr view <number> --repo awslabs/photon --json files,headRefOid` to check its changed paths. Fetch its diff only when the purpose and changed paths establish relevance. Do not inspect unrelated PR diffs, and do not treat a shared keyword alone as relevance.
3. Record one verdict per compared client and behavior: `ALIGNS`, `DIVERGES`, `MIXED`, or `INSUFFICIENT_EVIDENCE`.
4. Include a stable evidence ID, compared client, behavior or contract, source type (`main` or relevant PR), commit SHA or PR number and head SHA, path and lines, exact source snippet, immutable GitHub URL, observed behavior, verdict, and reasoning.

Absence of an implementation is not divergence unless the shared contract requires that client to implement it. Unavailable files, ambiguous behavior, contradictory evidence, or a lack of relevant clients produce `INSUFFICIENT_EVIDENCE`, not a guess. Write the records to `photon_client_consistency.json`.

---

## Phase 2: Initial Review

**Size gate** (use `additions + deletions` from `metadata.json`):
- `<= 500`: spawn ONE `code-reviewer` agent, all lenses in one pass.
- `> 500`: spawn 4 agents in parallel, one per lens (see below). Each reads the same run-directory files. Use this tier only when a single agent would lose the thread across 500+ lines.

**Photon context for both size paths:** For an `awslabs/photon` review, every agent that emits findings also reads `photon_client_consistency.json`. After each finding's normal evidence and reasoning, append a `client_consistency` object with `verdict`, `reasoning`, and the exact comparison-record evidence IDs. Cross-client behavior is supporting context and does not prove the underlying finding. Use `INSUFFICIENT_EVIDENCE` when the records do not establish alignment or divergence.

**Blast radius for both size paths:** When the diff changes a shared contract (for `awslabs/photon`: `shared/`, RESP or wire encoding, auth, or the connection, session, or config-refresh lifecycle; elsewhere: a public API, serialized format, or schema), the agent that reviews correctness (the single agent, or Agent B) also follows steps 1-4 of the `blast-radius` skill against the PR head. Breakage it confirms goes into the findings JSON as normal findings. It writes `blast_radius.md` with the one fact the change is safe because of and the evidence level it reached (1 to 3; this review runs no code), plus the risks it checked and cleared. Skip steps 5 and 6. Skip the whole check when no shared contract changed.

**Retry and shared state for both size paths:** When the diff changes retry, reconnect, restart, or refresh logic, or state that more than one actor writes, the correctness reviewer also applies the `principle-make-operations-idempotent` and `principle-separate-before-serializing-shared-state` skills and reports violations as normal findings. When the PR claims a performance change, it checks the claimed numbers against the `benchmark-checklist` skill and reports missing run counts, spread, or limiter as a finding.

**Prompt for the single-agent path (`<= 500`):**
> Read `/tmp/agent-runs/<RUNID>/diff.patch`, `requirements.md`, `codebase_context.md`, and `photon_client_consistency.json` when present. If you need a file beyond the codebase context, use `gh api repos/<owner>/<repo>/contents/<path>?ref=<head_ref> --jq .content | base64 -d`. Do not invent file contents.
>
> Use the `code-review-excellence` skill as your reasoning frame. Apply the four lenses in `../_shared/review-findings-schema.md` in order: Codebase Alignment first (primary lens), then Correctness & Security, then Requirements, then Testability. Only flag codebase-alignment issues that conflict with patterns visible in `codebase_context.md`. Write the findings JSON to `findings_v1.json`.

**For `> 500` line diffs, spawn 4 parallel agents:**

- **Agent A, `code-reviewer`, Codebase Alignment & Software Principles (primary):** Does the code fit this codebase? Flag reimplemented utilities, naming/casing/error-handling deviations, layering violations, premature abstraction, duplication of adjacent code, and violations of SOLID/DRY/YAGNI where the codebase visibly follows them. Only flag what conflicts with patterns visible in `codebase_context.md`, not general preferences.
- **Agent B, `code-reviewer`, Correctness & Requirements:** Logic bugs, off-by-ones, race conditions, unhandled errors, broken invariants, boundary and edge cases. Also check whether the implementation satisfies every acceptance criterion in `requirements.md`. Quote each criterion and mark MET or MISSING. Skip requirements if `requirements.md` is absent.
- **Agent C, `code-reviewer`, Security:** CWE-anchored threat model covering injection, broken access control, secrets and credential logging, crypto misuse, SSRF, path traversal, unsafe deserialization, and trust-boundary violations. Receives `diff.patch` and `codebase_context.md` only, not requirements.
- **Agent D, `tester`, Testability:** New behavior without tests, untested error paths, tests that do not assert behavior, mocks hiding real bugs, flaky patterns, missing edge cases. Only flag code that is new in this diff.

After all 4 agents complete, merge their JSON arrays, deduplicate findings on the same file and line (keep the highest severity: important, then nit, then pre-existing), and write to `findings_v1.json`.

---

## Phase 3: Validator (skeptic pass)

Spawn ONE `validator` agent. Pass it the path to `findings_v1.json` and do not have it re-read the diff. This is a single pass. Do not loop or re-run the validator on its own output.

Follow the self-challenge rubric in `../_shared/validator-skeptic-pass.md`. The validator reads only the findings list and `codebase_context.md` to verify claims. It does not re-read the full diff. Write the validated findings to `findings_v2.json`.

For `awslabs/photon`, the validator also reads `photon_client_consistency.json`. It verifies that every client-consistency statement follows from its cited path, lines, and source snippet; that cited open PRs concern the same behavior; that links use the recorded immutable commit SHA; and that one client's behavior is not generalized to all clients. It replaces an unsupported consistency conclusion with `INSUFFICIENT_EVIDENCE`. The normal reject and downgrade rules apply to the underlying finding independently.

When `blast_radius.md` exists, the validator also checks that its cited lines support the safety fact and lowers the stated evidence level when they don't.

---

## Phase 4: Final Report (local only)

Write the report yourself in the main session. Do not spawn an agent for it. Read these run-directory files:
- `metadata.json`
- `requirements.md`
- `codebase_context.md`
- the final `findings_v<n>.json`
- `photon_client_consistency.json` when present
- `blast_radius.md` when present

Drop every `verdict: REJECTED` finding. Use the post-downgrade severity for `DOWNGRADE` findings.

### 4a. Chat report
Print the review directly in chat, with findings first and ordered by severity:

```markdown
# PR Review: <PR title>

**Action:** <Approve | Request changes>
**Author:** <author> | **Files:** <count> | **+<additions> / -<deletions>** | **Jira:** <key or none>

## Important (<N>)
For each important finding:
- **`<file>:<line_range>`: <claim>**
  - Evidence: <exact evidence>
  - Reasoning: <why the evidence proves the claim>
  - Suggested fix: <specific fix>
  - Photon consistency: <ALIGNS | DIVERGES | MIXED | INSUFFICIENT_EVIDENCE>, <reasoning after the finding reasoning>
  - Client evidence: <immutable main or relevant-PR links with client, path, and lines>

## Nits (<N>)
<one line per severity=nit finding; include Photon verdict and evidence link when applicable>

## Pre-existing (<N>)
<same complete format for severity=pre-existing. These never affect the action.>

## What This PR Does
<concise explanation of the implementation and its structure>

## Blast radius
<only when `blast_radius.md` exists: the safety fact, its evidence level, and the cleared risks in one line each>

## Summary
<overall state, residual risk, and test coverage>
```

For non-Photon repositories, omit the Photon consistency and client evidence lines. For Photon findings, state the normal evidence and reasoning before the consistency verdict. Do not use another client's behavior as the sole proof of a finding. If there are no accepted findings, say so explicitly and still summarize the PR and residual test risk.

Choose the action consistently:
- Any `important` finding: `Request changes`
- Otherwise: `Approve`. List the nits and pre-existing items in the report.

Tone: direct, factual, and non-condescending. Avoid "simply", "just", and "obviously".

### 4b. Temporary HTML report
Create one complete HTML5 document at `/tmp/pr-review-<repo>-<number>-<timestamp>.html`, where `<repo>` is the lowercase repository basename with non-alphanumeric runs replaced by `-`, and `<timestamp>` is UTC `YYYYMMDD-HHMMSS`. The only companion file is the findings JSON in 4c.

The report is an evidence dashboard, ordered as follows:

1. PR title, metadata, recommended action, and severity counts.
2. "What this PR does": intent, requirements, and implementation summary.
3. Change map grouped by subsystem or directory, including each group's role.
4. Validated findings grouped as important, nits, and pre-existing.
5. Test coverage, untested behavior, and residual risk.
6. Photon client consistency matrix when `photon_client_consistency.json` exists.
7. Methodology and source links.

Every `important` and `pre-existing` finding displays its source location, claim, exact evidence, reasoning, suggested fix, validator disposition, and Photon consistency verdict and evidence when applicable. The Photon matrix displays behavior, compared client, source type, verdict, reasoning, exact source snippet, and immutable `main` or relevant-PR link. A link without path-and-line evidence is not sufficient.

Use a restrained, high-contrast technical-report design:
- System font stack, compact type, and zero letter spacing.
- Neutral page background with white content surfaces. Reserve red, amber, green, and blue for distinct semantic states rather than a one-hue palette.
- Maximum content width around 1280px, a stable two-column desktop summary, and a single-column layout below 760px.
- Borders and spacing for hierarchy; card radius no greater than 8px.
- No gradients, decorative blobs, nested cards, oversized hero text, external fonts, or decorative images.
- Long paths, snippets, titles, and URLs wrap without overlapping adjacent content.
- Print styles remove sticky positioning and preserve evidence text and links.

The file must be portable and safe:
- Put all CSS and any optional enhancement JavaScript inline. Use no remote fonts, scripts, stylesheets, images, or other runtime assets.
- The complete report stays readable with JavaScript disabled and when opened directly from disk.
- HTML-escape `&`, `<`, `>`, `"`, and `'` in every value from GitHub, Jira, source code, run-directory records, or findings before interpolation.
- Allow only `https://` source-link destinations. Attribute-escape URLs and add `rel="noreferrer noopener"` to links opened in a new tab.
- Do not interpolate untrusted values into `<style>`, `<script>`, event-handler attributes, or raw HTML.

After writing, verify that the file is non-empty, starts with `<!doctype html>`, contains the PR title and all accepted finding IDs, and has no external asset tags. Open it in the default browser when the environment supports that operation. If browser launch is unavailable or fails, keep the file and provide its absolute path.

### 4c. Findings JSON
Write the findings file next to the HTML report, with the same basename: `/tmp/pr-review-<repo>-<number>-<timestamp>.json`. It contains:

```json
{"pr": {"owner": "", "repo": "", "number": 0, "head_sha": "", "url": ""}, "generated_at": "<UTC ISO 8601>", "findings": []}
```

`findings` holds the final accepted findings: `REJECTED` dropped, post-downgrade severity, using the schema in `../_shared/review-findings-schema.md` (`file`, `line_range`, `severity`, `claim`, `evidence`, `suggested_fix`, and `client_consistency` when present). Take `head_sha` from `headRefOid` in `metadata.json`. Verify the file parses with `python3 -m json.tool <file>` (or `jq empty <file>`). `write-pr-comments` reads this file.

### 4d. Failure behavior
- Photon evidence fetch failure: continue the review and use `INSUFFICIENT_EVIDENCE`.
- Relevant PR read failure: identify that PR and state that its behavior could not be verified.
- Browser launch failure: keep the generated HTML file and report its path.
- HTML generation or validation failure: still print the complete chat report and state the local-output failure.
- Findings JSON write or parse failure: fix and rewrite it; if that fails, state the failure.
- Do not fall back to another note system and do not write to GitHub.

Finish by giving the user the HTML path, the findings JSON path, and the recommended action.

---

## No GitHub writes

This skill is read-only on GitHub. Follow `../_shared/no-github-writes.md` in full. The `gh` CLI is used for reads only. The user reads the output in chat and posts manually if desired.
