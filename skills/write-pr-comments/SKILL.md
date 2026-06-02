---
name: write-pr-comments
description: Post inline PR review comments from an Obsidian review note. Filters out non-actionable findings, presents each remaining one for approval, then posts approved comments as inline code comments via gh api.
---

# Write PR Comments

Post actionable review findings as inline GitHub PR comments from a previously saved Obsidian review note.

`$ARGUMENTS` is a PR URL, `owner/repo#number`, or empty. If empty, list recent notes in `PRs/` via `mcp__obsidian__list_directory` and ask which review to post.

---

## Phase 1: Load and Parse the Review Note

1. Derive the Obsidian note path from `$ARGUMENTS`:
   - If a PR URL or `owner/repo#number` is given, search `mcp__obsidian__search_notes` in the `PRs/` folder using the repo name and PR number.
   - If empty, use `mcp__obsidian__list_directory` on `PRs/` and ask the user which note to use.

2. Read the note with `mcp__obsidian__read_note`.

3. Parse the markdown into structured findings. Each finding has:
   - `severity`: from which `##` section it appears under (`Blocking`, `Recommended`, `Nits`)
   - `file`: extracted from the bold `**file:line**` pattern
   - `line`: the line number (or start of range)
   - `claim`: the text after the `—` on the finding line
   - `evidence`: text after `Evidence:`
   - `fix`: text after `Fix:`

---

## Phase 2: Filter Non-Actionable Findings

Remove any finding where the `fix` field contains any of these phrases (case-insensitive):
- "no change needed"
- "no action needed"
- "accept as consistent"
- "accept as matching"
- "no action required"
- "matches existing pattern"
- "matches flowise pattern"

Also remove findings where `fix` starts with "This is acknowledged" or "This is fine".

After filtering, if zero findings remain, tell the user "No actionable findings to post" and stop.

---

## Phase 3: Extract PR Metadata

From the review note header, extract:
- `owner/repo` and `PR number` (from the `# Review:` line — format is `owner-repo-number`)
- `commit_sha`: fetch via `gh pr view <number> --repo <owner/repo> --json headRefOid --jq .headRefOid`

If the PR identifier is ambiguous, ask the user to confirm.

---

## Phase 4: Present Findings One-by-One

For each remaining finding, present it to the user in this format:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Finding [N/total] — [severity]
File: [file], Line: [line]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[claim]

Evidence: [evidence]

Suggested fix: [fix]

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Post this comment? (yes / skip / edit / stop)
```

Wait for the user's response:
- **yes**: Queue this finding for posting.
- **skip**: Do not post this finding. Move to next.
- **edit**: Ask the user for revised text, then queue the edited version.
- **stop**: Stop presenting findings. Post whatever has been queued so far.

---

## Phase 5: Humanize Comment Text

Before posting, run each approved comment's body through the `/humanizer` skill. The goal is to make comments sound like a real engineer wrote them, not an AI tool.

Apply humanizer to the combined `claim + evidence + fix` text of each approved finding. Keep technical accuracy intact — only adjust tone, phrasing, and AI-isms.

---

## Phase 6: Post Approved Comments

Construct a single review via `gh api`:

```bash
cat <<'EOF' | gh api repos/<owner>/<repo>/pulls/<number>/reviews --method POST --input -
{
  "commit_id": "<commit_sha>",
  "event": "COMMENT",
  "body": "Review: <count> comments posted.",
  "comments": [
    {
      "path": "<file>",
      "line": <line>,
      "body": "<formatted comment body>"
    }
  ]
}
EOF
```

Format each comment body as:
```
**[severity]:** [claim]

[evidence — if short enough, include inline]

**Suggested fix:** [fix]
```

If more than 20 comments are queued, batch into multiple reviews (GitHub API limit).

After posting, print a confirmation with the review URL and the count of comments posted.

---

## Rules

- Use `gh` CLI for all GitHub writes (posting review comments). This is the inverse of review-pr which is read-only.
- Use `mcp__obsidian__*` tools for reading review notes.
- NEVER modify the Obsidian note.
- NEVER approve or request changes — always use `event: "COMMENT"`.
- If `gh auth status` fails, stop and tell the user to authenticate.
- If the PR has been merged or closed, warn the user and ask whether to proceed.
