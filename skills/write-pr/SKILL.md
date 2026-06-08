---
name: write-pr
description: Generate a polished, human-sounding PR description from the current git changes. Optionally accepts a commit range (e.g. HEAD~3) and/or a PR template path. Reads the diff, drafts from the repo's PR template (or a default), verifies accuracy, then humanizes the output.
---

# Write PR Description

Generate a polished, human-sounding PR description for the current changes. Follow each phase in order. Do not skip phases.

## Input

`$ARGUMENTS` is optional and may contain:
1. A **commit range** (e.g. `HEAD~3`, `main..HEAD`). Default: the last commit plus any staged/unstaged changes.
2. A **PR template path** (e.g. `.github/pull_request_template.md`). Default: auto-detect from `.github/pull_request_template.md` or `.github/PULL_REQUEST_TEMPLATE/` in the repo. If none found, use the built-in default template below.

Parse `$ARGUMENTS` to separate these two inputs. A commit range looks like a git ref or range; a file path contains `/` or `.md`.

---

## Phase 1: Read Changes

Gather the full diff and commit messages for the target range:

```bash
# If a commit range was given (e.g. HEAD~3):
git log --oneline <range>
git diff <range>

# If no range given, use last commit + working changes:
git log -1 --oneline
git diff HEAD
git diff --cached
```

Read through the diff carefully. Understand:
- What files changed and why
- The intent behind each logical change
- Any new dependencies, config changes, or migrations

---

## Phase 2: Draft PR Description

### Template Selection
1. If the user provided a template path, read that file.
2. Otherwise, check for `.github/pull_request_template.md` or files in `.github/PULL_REQUEST_TEMPLATE/` in the repo root.
3. If no template exists, use this default:

```markdown
## Summary
<!-- 2-3 sentences: what this PR does and why. -->

## Issue
<!-- Link or reference to the Jira/GitHub issue, e.g. AEA-123. Write "N/A" if none. -->

## Changes
<!-- For each file (or logical group of files), one bullet explaining what changed and why. -->
- `path/to/file.py` — reason for change

## Tests
<!-- What tests were added or modified? How can a reviewer verify correctness? -->

## Blockers / Future work
<!-- Optional. Known limitations, follow-up tasks, or things intentionally left out of this PR. Remove this section if not applicable. -->

## Additional context
<!-- Optional. Screenshots, benchmarks, migration notes, links to design docs, or anything else a reviewer should know. Remove this section if not applicable. -->
```

### Drafting
Fill in every section of the chosen template based on the actual changes. Be specific and accurate:
- Reference concrete file names, functions, and behaviors — not vague generalities
- Explain **why**, not just **what**
- If the template has checkboxes or optional sections, fill in only the ones that apply
- Keep it concise — a reviewer should be able to skim it in under a minute

---

## Phase 3: Verify Accuracy

Review the draft against the actual diff. Check:
- Every claim in the description is supported by the diff (no hallucinated changes)
- No significant changes are omitted from the description
- File names, function names, and behaviors mentioned are accurate
- The description doesn't overstate or understate the scope

If anything is inaccurate or missing, fix it before proceeding.

---

## Phase 4: Humanize

Apply the `humanizer` skill to the PR description. The goal is to make it read like a real developer wrote it, not an AI. The humanizer skill contains the comprehensive rule set — do not re-specify individual patterns here.

---

## Phase 5: Final Output

Print the final PR description inside a single fenced markdown code block so the user can copy and paste it directly:

~~~markdown
```markdown
<final PR description here>
```
~~~

Do not print anything after the code block.
