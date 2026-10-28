---
name: write-pr
description: Generate a brief PR description from current git changes that a reviewer can read in under a minute. Optionally accepts a commit range (e.g. HEAD~3) and/or a PR template path. Reads the diff, drafts from the repository PR template (or a short default), cuts it to fit, verifies accuracy, then applies the technical-writing and unslop skills. Use when asked to write, draft, or tighten a PR description.
---

# Write PR description

Write a PR body that a reviewer can read in under a minute and still know what changed, why, what to look at closely, and how it was tested. Every sentence answers a question the reviewer would ask. Anything else goes.

## Input

`$ARGUMENTS` is optional and may contain:
1. A **commit range** (e.g. `HEAD~3`, `main..HEAD`). Default: the last commit plus any staged and unstaged changes.
2. A **PR template path** (e.g. `.github/pull_request_template.md`). Default: auto-detect `.github/pull_request_template.md` or `.github/PULL_REQUEST_TEMPLATE/`. If none exists, use the default template below.

A commit range looks like a git ref or range. A file path contains `/` or `.md`.

## Phase 1: Read changes

```bash
# With a range:
git log --oneline <range>
git diff <range>

# Without a range:
git log -1 --oneline
git diff HEAD
git diff --cached
```

Work out the intent behind each logical change, plus any new dependencies, config changes, migrations, or interface changes.

## Phase 2: Draft

### Template

1. Use the template path the user gave, if any.
2. Otherwise use the repository template if one exists. Keep its required sections and drop optional sections that would be empty.
3. Otherwise use this default:

```markdown
## Summary
<!-- 1-3 sentences: the problem, then what this PR does about it. -->

## Changes
<!-- One row per logical change, grouped by area. One short clause or sentence per cell. -->
| Area | Change |
|---|---|
| `path/or/area` | What changed and why |

## Review notes
<!-- Optional. Decisions a reviewer may question, risks, compatibility, rollout, and follow-ups left out of this PR. One or two sentences per item. -->

## Testing
<!-- Commands run and their results. -->
```

### Length budget

| Section | Budget |
|---|---|
| Summary | 1-3 sentences. No implementation detail the Changes list repeats. |
| Changes | One row per area, with one short clause or sentence per cell. Split a crowded cell into more rows instead of packing it. Never list every file. |
| Review notes | Only items a reviewer would flag or must know before merging. One or two sentences each. Omit the section when empty. |
| Testing | The commands and results, one line each. Name the edge cases the tests cover, without explaining them. |

Aim for under 200 words of prose. A real protocol, migration, or compatibility contract can earn more.

### Show it when it reads faster

Prefer a visual form over prose when the reader can scan it faster:

- A table for the Changes list, for `Before`/`After` behavior, or for any comparison across the same few attributes.
- A bulleted list for parallel items such as review notes or test commands.
- A short code or JSON example when it pins down a contract that prose would leave ambiguous.

Keep each table narrow and each cell short. A table with paragraphs in its cells is harder to read than prose.

### Rules

- Lead with why. The diff already shows what moved.
- Use real file, function, flag, and command names.
- Do not narrate the diff ("updated X to call Y, then changed Z"). Say the effect.
- Do not paste agent logs, SHA lists, full test output, or metric tables. Summarize the result and link the full output if the user has a link.
- Do not invent a test result, coverage number, benchmark, or manual check. If nothing was tested, say so in one line.
- Do not add an `Issue` section with `N/A`, a commit title, or a branch name unless the template or the user asks.

## Phase 3: Cut

Reread the draft as the reviewer and delete the following:

- Sentences that restate the title, the Summary, or another section.
- Background the reviewer already knows from the linked issue or the codebase.
- Qualifiers and hedges that do not change what the reviewer does.
- Review notes that describe an obvious choice.

## Phase 4: Polish

Apply the `technical-writing` skill's rules for PR descriptions, which include the `unslop` skill. Those skills own the sentence and anti-AI rules, so do not restate them here. Keep the facts and the section structure.

## Phase 5: Verify

Check the draft against the diff and the command output:

- Every claim is supported by the diff, the commit history, or observed output.
- No significant change, compatibility constraint, or test result is missing. Brevity never drops one of these.
- Names of files, functions, and interfaces are correct.
- `Before`/`After` rows describe real behavior, not intended behavior.
- The draft does not overstate coverage, performance, rollout safety, or scope.

Fix any problem before you continue.

## Phase 6: Output

Print the final PR description in one fenced markdown code block so the user can copy it:

~~~markdown
```markdown
<final PR description here>
```
~~~

Print nothing before or after the code block.
