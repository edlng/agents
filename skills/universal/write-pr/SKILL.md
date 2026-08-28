---
name: write-pr
description: Generate a concise, human-sounding PR description from current git changes in a reviewer-first format. Optionally accepts a commit range (e.g. HEAD~3) and/or a PR template path. Reads the diff, drafts from the repository PR template (or a structured default), verifies accuracy, then humanizes the output.
---

# Write PR Description

Generate a concise, technically detailed PR description for the current changes. Use the format of a strong engineering handoff: explain the problem, map the implementation, call out non-obvious decisions, show testing evidence, and state what is deliberately out of scope. Follow each phase in order.

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

## Phase 2: Draft the PR Description

### Template Selection
1. If the user provided a template path, read that file.
2. Otherwise, check for `.github/pull_request_template.md` or files in `.github/PULL_REQUEST_TEMPLATE/` in the repo root.
3. If no template exists, use this default shape:

```markdown
## Summary
<!-- Explain the problem, its impact, and the approach in 1-2 short paragraphs. -->

## What's in it
<!-- Group related changes by module or area. Do not list every file separately. -->
| Area | Change |
|---|---|
| `path/to/module` | ... |

## Design decisions
<!-- Include only decisions a reviewer may question. Omit when there are none. -->

## Tests
<!-- State commands/results, coverage when available, and meaningful edge cases. -->

## Not in this PR
<!-- State explicit follow-ups or intentional limits. Omit when there are none. -->
```

If the repository template has required sections, preserve them. Use the structure above within the template where it fits. Do not add an `Issue` section with `N/A` unless the template requires it.

### Drafting

Organize the body around the questions a reviewer will have:

- **Summary:** Start with the behavior or problem that motivated the change. Explain the impact, then say what the implementation does. When behavior changes, add a small `Before`/`After` table. Keep it concrete.
- **What's in it:** Group files into logical modules or areas and explain each role. Mention new files, changed interfaces, compatibility behavior, migrations, and paired changes where they matter. A short JSON, protocol, or API example is useful when it makes a contract unambiguous.
- **Design decisions:** Include only non-obvious choices, tradeoffs, failure behavior, rollout constraints, or compatibility rules. Use short subsections when there are multiple decisions. Do not restate the implementation line by line.
- **Testing:** Report the actual commands, results, counts, coverage, and important edge cases. Include failure-path or regression tests when they are part of the change. Never invent a test result, coverage number, benchmark, or manual verification step.
- **Not in this PR:** Name deferred work, known limitations, or required follow-ups when they affect how the change should be reviewed or shipped. Omit the section when there is nothing meaningful to say.

Use concrete file names, functions, interfaces, and behaviors. Explain why a change exists, not just what moved. Prefer grouped tables and short paragraphs over a long file-by-file checklist. Include an issue, design document, rollout note, or external dependency only when it is present in the repository context or the supplied template. Keep the body as short as the scope allows; expand for a real protocol, migration, or compatibility contract rather than padding the summary.

Do not generate a commit title or branch name unless the user explicitly asks for them. The normal output is the PR body only.

## Phase 3: Humanize

Apply the `humanizer` skill to the completed draft. The `humanizer` skill owns the humanization and anti-AI rules; do not duplicate or override those rules here. Preserve the draft's technical facts, structure, and reviewer-oriented detail. If humanization changes a factual claim, resolve it against the diff and test output during Phase 4.

## Phase 4: Verify Accuracy

Review the draft against the actual diff and test output. Check:

- Every claim is supported by the diff, commit history, or observed command output
- No significant logical change, compatibility constraint, or test result is omitted
- File names, function names, interfaces, and behaviors are accurate
- Before/after tables describe real behavior rather than intended behavior
- The description does not overstate coverage, performance, rollout safety, or scope
- Optional sections are removed when they contain no useful information

Fix anything inaccurate, vague, repetitive, or missing before proceeding.

## Phase 5: Final Output

Print the final PR description inside a single fenced markdown code block so the user can copy and paste it directly:

~~~markdown
```markdown
<final PR description here>
```
~~~

Do not print a preamble, commit metadata, or anything after the code block.
