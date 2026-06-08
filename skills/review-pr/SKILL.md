---
name: review-pr
description: Senior-grade PR review against the linked Jira ticket and existing codebase. Uses Valkey caching, a merged-lens reviewer (latest Sonnet), and a skeptic validator pass (Opus for large diffs). Output is local-only — printed in chat and saved to Obsidian, never posted to GitHub.
---

# Review PR (senior-dev grade)

Review someone else's pull request against the linked Jira ticket and the existing codebase.

This is the **standard** PR review. The full workflow is defined in the shared base file so it can be reused by specialized variants (e.g. `review-cookbook-pr`).

## Run the base workflow

Follow `_shared/pr-review-base.md` end to end (relative to this skill's directory — i.e. `../_shared/pr-review-base.md`). It covers:

- Cache setup (per `_shared/valkey-cache-conventions.md`)
- Phase 1: Context gathering (PR metadata, Jira issue, diff, requirements, codebase context per `_shared/codebase-context-checklist.md`)
- Phase 2: Merged-lens review (findings schema and lenses per `_shared/review-findings-schema.md`)
- Phase 3: Validator skeptic pass (per `_shared/validator-skeptic-pass.md`)
- Phase 4: Local-only report, printed in chat and saved to Obsidian (read-only per `_shared/no-github-writes.md`)

There are no additional phases for the standard review. Execute the base workflow exactly as written.
