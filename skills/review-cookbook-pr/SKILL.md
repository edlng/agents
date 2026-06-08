---
name: review-cookbook-pr
description: Senior-grade PR review for cookbook PRs. Same as review-pr but adds two cookbook-specific checks: (1) code snippets call project functions directly rather than simulating Valkey internals, and (2) valkey/valkey-bundle version tags are verified against Docker Hub.
---

# Review Cookbook PR (senior-dev grade)

Review a cookbook pull request. This is the standard PR review **plus** two cookbook-specific checks.

## Run the base workflow

Follow `_shared/pr-review-base.md` (relative path: `../_shared/pr-review-base.md`) for cache setup, Phase 1 (context), Phase 2 (merged-lens review), Phase 3 (validator), and Phase 4 (local-only report). The cookbook additions below slot into that workflow.

---

## Phase 2b: Cookbook-Specific Checks (ADDITION)

**Run in parallel with the base Phase 2** — spawn a separate `code-reviewer` subagent for this phase. These checks run against `pr:$RUNID:diff`.

Prompt:
> "You are reviewing a cookbook PR for two specific quality issues. Read `pr:$RUNID:diff` from Valkey at `localhost:8888`.
>
> ## Check 1: Code snippets use project functions, not Valkey simulations
>
> Cookbook code snippets must demonstrate how to USE the project's API — they should call project-level functions and methods directly (e.g. `agent.receive()`, `client.search()`, `store.set()`). They must NOT simulate or narrate Valkey internals inline in the snippet.
>
> **Prohibited pattern:** A code comment that maps a project function call to a raw Valkey/Redis command as if the snippet is an explanation of the implementation detail — e.g. `# this simulates ft.create(...)`, `# internally calls FT.SEARCH`, `# this is equivalent to ZADD ...`.
>
> **Allowed exception:** Explanatory prose OUTSIDE the code snippet (in markdown paragraphs) may describe how a function works under the hood, including Valkey commands it uses. Comments inside a code snippet that explain behavior (e.g. `# returns all results matching the filter`) are fine as long as they do NOT use raw Valkey command syntax to simulate what the function does.
>
> For each violating snippet, file a finding with: `id`: `cookbook-simulates-valkey-<slug>`, `lens`: `cookbook_api_usage`, `file`, `line_range`, `severity`: `blocking`, `claim` (quote the offending comment and explain why it should be removed or moved to prose), `evidence` (exact diff line(s)), `suggested_fix` (remove the comment, rewrite it without raw Valkey syntax, or move it to prose outside the snippet).
>
> ## Check 2: valkey/valkey-bundle version accuracy
>
> Scan the diff for references to `valkey/valkey-bundle` with a specific version tag. For each versioned reference:
> 1. If the tag is `latest`, no check is needed.
> 2. If the tag is a specific version (e.g. `8.1.1`), verify it exists on Docker Hub:
>    `curl -s "https://hub.docker.com/v2/repositories/valkey/valkey-bundle/tags/?name=<version>" | python3 -m json.tool`
>    Confirm the tag appears in the results.
> 3. If the PR claims a feature version included in the image (e.g. 'uses valkey-search 1.2'), cross-check against the Docker Hub tag's description/release notes. If unconfirmed, file a finding.
>
> For each version issue, file a finding with: `id`: `cookbook-valkey-bundle-version-<slug>`, `lens`: `cookbook_version_accuracy`, `file`, `line_range`, `severity`: `blocking` if the tag does not exist / `suggestion` if the feature-version claim is unverifiable, `claim` (claimed vs. what Docker Hub shows), `evidence` (exact diff line(s)), `suggested_fix` (use the correct tag, or remove/qualify the unverifiable claim).
>
> Output the findings as a JSON array (same schema as `_shared/review-findings-schema.md`). Write to `pr:$RUNID:findings_cookbook` in Valkey."

After both the base Phase 2 and Phase 2b complete, merge `pr:$RUNID:findings_cookbook` into `pr:$RUNID:findings_v1` (append cookbook findings to the main findings) before proceeding to Phase 3.

---

## Phase 3: Validator — cookbook addenda (ADDITION)

Run the base Phase 3 (`_shared/validator-skeptic-pass.md`) unchanged, but add these two self-challenge questions for cookbook-lens findings:

4. For `cookbook_api_usage`: Does the quoted comment actually use raw Valkey command syntax to simulate a function, or is it just explaining behavior in plain English? Reject if the latter.
5. For `cookbook_version_accuracy`: Did the reviewer actually run the Docker Hub curl check, or is this a guess? If the tag was not verified, mark it DOWNGRADE (suggestion) unless the tag clearly does not follow Docker Hub's known version format.

---

The base Phase 4 report and the read-only GitHub rule (`_shared/no-github-writes.md`) apply unchanged.
