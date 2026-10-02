---
name: write-pr-comments
description: Posts inline PR review comments from a review-pr findings file as a pending GitHub review. Filters out non-actionable findings, presents each remaining one for approval, humanizes the approved text, and posts a pending review visible only to the user until they submit it. Use when the user explicitly asks to post review findings as PR comments.
disable-model-invocation: true
---

# Write PR Comments

Post actionable review findings as inline GitHub PR comments from a findings file that `review-pr` saved.

`$ARGUMENTS` is a PR URL or `owner/repo#number`. If empty, ask the user for one.

---

## Phase 1: Load the Findings File

1. Parse `owner`, `repo`, and `number` from `$ARGUMENTS`.

2. Find the newest findings file:

   ```bash
   ls -t /tmp/pr-review-<repo>-<number>-*.json 2>/dev/null | head -1
   ```

   If none exists, tell the user to run `review-pr` on this PR first and stop.

3. Read the file. It has the shape `{"pr": {"owner", "repo", "number", "head_sha", "url"}, "generated_at", "findings": [...]}`. Each finding has:
   - `file`: path
   - `line_range`: e.g. `42-58`, or a single line such as `42`
   - `severity`: `important`, `nit`, or `pre-existing`
   - `claim`: what is wrong
   - `evidence`: a verbatim code quote
   - `suggested_fix`: the concrete change

4. Confirm `pr.owner`, `pr.repo`, and `pr.number` match the arguments. If they differ, ask the user to confirm.

---

## Phase 2: Filter Non-Actionable Findings

Remove any finding where `suggested_fix` contains any of these phrases (case-insensitive):
- "no change needed"
- "no action needed"
- "accept as consistent"
- "accept as matching"
- "no action required"
- "matches existing pattern"
- "matches flowise pattern"

Also remove findings where `suggested_fix` starts with "This is acknowledged" or "This is fine".

After filtering, if zero findings remain, tell the user "No actionable findings to post" and stop.

---

## Phase 3: Get the Head Commit

Fetch the current head commit. The findings file's `pr.head_sha` can be stale if the author pushed since the review:

```bash
gh pr view <number> --repo <owner>/<repo> --json headRefOid,state --jq '{sha: .headRefOid, state: .state}'
```

Use the fetched SHA as `commit_sha`. If it differs from `pr.head_sha`, warn the user that the review ran on an older commit. Phase 6 re-verifies every anchor against `commit_sha`, so stale line numbers are caught there.

---

## Phase 3b: Skip Already-Posted Comments

Before presenting findings, fetch existing review comments so the same point isn't posted twice:

```bash
gh api repos/<owner>/<repo>/pulls/<number>/comments --paginate \
  --jq '.[] | {path: .path, line: .line, body: .body}'
```

For each finding, check whether an existing comment is on the same `file` at (or near) the same line and covers the same point. If it does, drop the finding from the queue and tell the user it was skipped as a duplicate. Match on file, proximity of line, and overlapping subject. The existing comment may be phrased differently, so do not require an exact string match.

---

## Phase 4: Present Findings One-by-One

Present `important` findings first, then `nit`, then `pre-existing`. Queue `important` and `nit` findings for posting by default. Present `pre-existing` findings too, but default them to skip. The user must answer yes to queue one.

For each remaining finding, present it to the user in this format:

```
========================================
Finding [N/total], [severity]
File: [file], Lines: [line_range]
========================================

[claim]

Evidence: [evidence]

Suggested fix: [suggested_fix]

========================================
Post this comment? (yes / skip / edit / stop)  [default: skip for pre-existing, yes otherwise]
```

Wait for the user's response:
- **yes**: Queue this finding for posting.
- **skip**: Do not post this finding. Move to the next.
- **edit**: Ask the user for revised text, then queue the edited version.
- **stop**: Stop presenting findings. Post whatever has been queued so far.

---

## Phase 5: Humanize Comment Text

Before posting, run each approved comment's body through the `pr-comment-humanizer` skill. The goal is to make comments sound like the author's real code review voice: terse, imperative, no AI phrasing, no severity labels in the body (word choice and a lowercase `nit` prefix convey severity).

Apply `pr-comment-humanizer` to the combined `claim`, `evidence`, and `suggested_fix` text of each approved finding. Keep technical accuracy intact and change only tone and phrasing. The humanized text is the final comment body. Do not re-wrap it with severity headers or templated sections in Phase 7.

---

## Phase 6: Resolve and Verify Comment Anchors

The `line_range` in the findings file is written by a reviewer model and is routinely off by a few lines (a finding about a call on line 44 may be recorded as 48). Posting on the raw number lands the comment on the wrong code. Verify the anchor against the actual file content, then confirm it falls inside a diff hunk.

Anchor with `line` + `side: "RIGHT"` (file-relative), not the deprecated `position`. The line must be inside a diff hunk, or the API returns HTTP 422 ("line ... could not be resolved").

Resolve each anchor in three steps.

**Step 1. Verify the line against real file content (authoritative).**
Each finding's `evidence` field contains a verbatim code quote. Fetch the file at `commit_sha` and locate that quote to get the true line number. Do not trust `line_range` when the evidence can be found.

```bash
# Fetch the file content at the head commit, numbered
gh api "repos/<owner>/<repo>/contents/<path>?ref=<commit_sha>" --jq '.content' \
  | base64 -d | nl -ba
```

For each finding:
- Extract the most distinctive verbatim fragment from `evidence`. Prefer the code inside the first backtick pair (e.g. `get_query_embedding(query.query_str)`). Strip backticks.
- `grep -nF '<fragment>'` the numbered content. If exactly one line matches, that line is the verified anchor.
- If multiple lines match, pick the one closest to the start of `line_range`.
- If nothing matches (evidence paraphrased or reformatted), fall back to the start of `line_range` and tell the user the anchor is unverified.

**Step 2. Build the set of commentable lines from the diff.**

```bash
gh api repos/<owner>/<repo>/pulls/<number>/files --paginate \
  | python3 -c '
import sys, json, re
files = json.load(sys.stdin)
out = {}
for f in files:
    patch = f.get("patch")
    if not patch:
        continue
    valid, new_line = [], None
    for ln in patch.split("\n"):
        if ln.startswith("@@"):
            m = re.search(r"\+(\d+)", ln)   # new-side start line from @@ -a,b +c,d @@
            new_line = int(m.group(1)) if m else None
        elif new_line is None or ln == "" or ln.startswith("\\"):
            continue                         # skip preamble and "\ No newline" markers
        elif ln.startswith("-"):
            continue                         # removed line: not commentable on RIGHT, no advance
        else:                                # added (+) or context ( ): commentable, advance
            valid.append(new_line)
            new_line += 1
    out[f["filename"]] = valid    # files API returns "filename", not "path"
print(json.dumps(out, indent=2))
'
```

This prints a map of `path -> [commentable line numbers]` (new-side / RIGHT).

**Step 3. Anchor each comment.** Decide single-line or range first, then validate against the commentable set.

*Single line (default).* Use the Step 1 verified line:
- If the verified line is in the commentable set, anchor with `"line": <verified_line>, "side": "RIGHT"`.
- If it is not in the set, snap to the nearest value in that file's list and prefix the body with `(re: line <verified_line>)`.
- If the file has no entry (not in the diff: renamed-only, binary, or unchanged), skip the finding and warn the user.

*Multi-line range (only when both endpoints are verifiable).* Some findings are about a block, not one line (a batch-insert loop, a try/except, a multi-line config). Draw a range only when both ends can be located in the real file:
- If `line_range` is a range (`start-end`) and `evidence` quotes two distinct code fragments (e.g. `tasks.append(...)` and `await asyncio.gather(...)`), grep each in the numbered content from Step 1 to get `start_line` and the end `line`.
- Set `start_line` to the smaller number and `line` to the larger, with `side` and `start_side` both `"RIGHT"`. `start_line` must be strictly less than `line`.
- Both endpoints must be in the commentable set and within the same diff hunk. If either fails, do not draw the range. Fall back to a single-line anchor on the verified start line.
- Do not build a range from the `start-end` numbers in `line_range` alone (e.g. `191-210`). An unverified range highlights the wrong block. Treat `line_range` as a hint for where to look, not as the anchor.

Notes:
- For a single-line comment, omit `start_line` and `start_side`.
- Removed lines are only commentable on `side: "LEFT"`. This skill posts on new code, so always use `RIGHT`.

---

## Phase 7: Post Approved Comments

The review is posted as pending. It is visible only to the user in the GitHub UI with inline comments attached, and is not submitted publicly. The user finishes the review in the browser (Approve, Comment, or Request Changes).

Write a top-level review body as a draft summary for the user's reference:
- A brief note on what the PR does well
- A one-line summary of the comments left (e.g. "1 important issue re: credential handling, 2 nits")
- Keep it short. The user edits it before submitting if needed.

Create a single review with `gh api`. Each comment's `body` is the humanized text from Phase 5, anchored with `line` + `side`, never `position`:

```bash
cat <<'EOF' | gh api repos/<owner>/<repo>/pulls/<number>/reviews --method POST --input -
{
  "commit_id": "<commit_sha>",
  "body": "<review body written above>",
  "comments": [
    {
      "path": "<file>",
      "line": <line>,
      "side": "RIGHT",
      "body": "<humanized single-line comment body from Phase 5>"
    },
    {
      "path": "<file>",
      "start_line": <start_line>,
      "start_side": "RIGHT",
      "line": <end_line>,
      "side": "RIGHT",
      "body": "<humanized range comment body, only when both endpoints were verified>"
    }
  ]
}
EOF
```

If more than 20 comments are queued, batch them into multiple reviews (GitHub API limit).

After posting, print a confirmation with the review URL, the count of comments posted, and a reminder: "Review is pending. Open the PR in your browser to submit with your chosen action."

---

## Rules

- Use the `gh` CLI for all GitHub reads and for posting the review. This is the inverse of `review-pr`, which never writes to GitHub.
- Read findings only from the `/tmp/pr-review-<repo>-<number>-*.json` file. Do not modify it.
- Anchor inline comments with `"line"` + `"side": "RIGHT"` (file-relative), not the deprecated `"position"`. Verify the line against actual file content using the finding's evidence quote (Phase 6, Step 1), because `line_range` is model-generated and often off. Then confirm the line falls inside a diff hunk (Step 2), since an out-of-diff line returns HTTP 422.
- Run approved comment bodies through `pr-comment-humanizer` before posting (Phase 5).
- Skip findings already covered by an existing PR comment (Phase 3b).
- To create a pending review, omit the `"event"` field from the POST body. The API rejects `"event": "PENDING"` with HTTP 422, and the review is pending by default. The user submits it in the GitHub UI.
- If `gh auth status` fails, stop and tell the user to authenticate.
- If the PR has been merged or closed, warn the user and ask whether to proceed.
