---
name: review-addressed-comments
description: Given a PR link, reviews edlng's review comments, verifies each was addressed in the latest code, and provides a final verdict (APPROVE, REQUEST CHANGES, or COMMENT) with code evidence.
---

# Review Addressed Comments

Verify that all of edlng's review comments on a PR have been addressed in the current code, providing the new code snippets as evidence, and give a final recommendation.

## When to Use

- When asked to check whether edlng's review feedback has been addressed on a PR
- Before approving a PR that edlng previously reviewed
- When asked to verify review comments were resolved

## Workflow

1. **Fetch PR review threads** — use `pull_request_read` with `get_review_comments` to retrieve all review threads on the PR.

2. **Filter to edlng's comments** — identify every thread where edlng authored a comment. Record the file path, line number, and the substance of each comment.

3. **Check for author responses** — in each thread, look for the PR author's reply describing how they addressed the feedback.

4. **Fetch current file contents** — for each file edlng commented on, use `get_file_contents` on the PR's head branch to retrieve the latest code.

5. **Verify with evidence** — for each comment, find the specific code snippet or documentation change that addresses it. Quote the relevant lines as evidence.

6. **Produce the report** — for each comment, output:
   - edlng's original concern (one-line summary)
   - File and location
   - Status: ✅ Addressed, ⚠️ Partially addressed, or ❌ Not addressed
   - Code evidence (the new snippet proving the fix)

7. **Final verdict** — based on the results:
   - All addressed → recommend **APPROVE**
   - Some partially addressed or minor gaps → recommend **COMMENT** with notes
   - Any not addressed → recommend **REQUEST CHANGES** with specifics

## Output Format

```
## edlng's Comments — Status

### 1. [One-line summary of concern] ([file]:[line])
**Status**: ✅ Addressed
**Evidence**:
\```language
<relevant code snippet>
\```

... (repeat for each comment)

---

## Verdict: APPROVE | COMMENT | REQUEST CHANGES

[Brief justification]
```

## Notes

- Only evaluate edlng's comments, not other reviewers
- If a comment was marked as a "nit", still verify it but weight it lower in the final verdict
- If code evidence cannot be found (file deleted, lines moved significantly), note this and check if the concern was addressed differently
