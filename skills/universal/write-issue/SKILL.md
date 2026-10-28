---
name: write-issue
description: Draft a brief GitHub issue (bug, feature, or task) that a maintainer can read in under a minute. Searches for duplicates, checks claims against the code, runs the repro when one exists, drafts from the repository issue template (or a short default), cuts it to fit, verifies accuracy, then applies the technical-writing and unslop skills. Prints the title and body, then offers to post with `gh issue create`. Use when asked to write, draft, file, or open an issue or bug report.
---

# Write issue

Write an issue that a maintainer can read in under a minute and still know what is wrong or wanted, how to see it, and where in the code to look. Every sentence answers a question the maintainer would ask. Anything else goes.

## Input

`$ARGUMENTS` is optional and may contain:
1. A **repository** (`owner/repo` or a GitHub URL). Default: the `origin` remote of the current directory.
2. A **description** of the problem or request, or a pointer to one: failing test output, a log file, a review finding, or the current conversation.

If the input does not say what the issue is about, ask the user before you start.

## Phase 1: Search for duplicates

Pick two or three key terms (the symbol, the error message, the area) and search open and closed issues:

```bash
gh issue list --repo <owner/repo> --state all --search "<terms>" --limit 10 \
  --json number,title,state,url
```

If a result looks like the same problem, show it to the user and ask whether to continue, comment there instead, or stop. Do not comment on it yourself.

## Phase 2: Gather evidence

Check the claims before you write them down:

- Read the code the issue is about. Note the `file:line` of the relevant symbols.
- If there is a repro command or test, run it and capture the real output. Trim the output to the lines that show the problem.
- Record versions and environment facts you can read directly: the package version, the language runtime, the OS, and the commit SHA.
- For a regression, find the last good version or commit and the first bad one if `git log` or the tags make that cheap.

If you cannot reproduce the problem, say so in the draft instead of implying that you did.

## Phase 3: Pick the type and template

1. If the repository has `.github/ISSUE_TEMPLATE/`, pick the template that matches the type and keep its required fields. For a form template (`.yml`), write one section per field, using the field `label` as the heading. Follow its `title:` pattern.
2. Otherwise, pick one of the default shapes below.

### Title

Write `area: symptom`, in lowercase after the colon unless a name requires capitals. The area is the package, module, or language client (`java`, `node`, `session`). State the problem, not the fix: "java: header names lowercase with the default locale", not "java: use Locale.ROOT".

### Bug

```markdown
## Summary
<!-- 1-3 sentences: what breaks, for whom, and how badly. -->

## Steps to reproduce
<!-- Numbered steps, or one command. The smallest case that shows it. -->

## Expected and actual
| | Behavior |
|---|---|
| Expected | ... |
| Actual | ... |

## Environment
<!-- Only the facts that matter: version, runtime, OS, commit. -->

## Logs
<!-- Trimmed output in a code block. Wrap long output in <details>. Omit when none. -->

## Possible cause
<!-- Optional. Only when the code points at one. Cite file:line. -->
```

### Feature

```markdown
## Summary
<!-- 1-3 sentences: what to add and who needs it. -->

## Use case
<!-- The concrete situation that needs it, and what users do today instead. -->

## Proposal
<!-- The shape of the change: API, flag, or behavior. A short code example if it pins the contract. -->

## Alternatives
<!-- Optional. Other options and why they fall short. -->
```

### Task

Use this shape for cleanup, triage, alignment, and tracking work.

```markdown
## Summary
<!-- 1-3 sentences: what needs doing and why now. -->

## Scope
<!-- A table when the work splits into items with a count, location, or status. -->
| Item | Where | Notes |
|---|---|---|

## Checklist
- [ ] ...

## Open questions
<!-- Optional. Decisions that block or shape the work. -->
```

## Phase 4: Draft

### Length budget

| Section | Budget |
|---|---|
| Summary | 1-3 sentences. |
| Steps, checklist | One line per step or item. |
| Expected and actual, scope | One short clause or sentence per cell. Split a crowded cell into more rows. |
| Logs | Only the lines that show the problem. |
| Possible cause, alternatives, open questions | One or two sentences per item. Omit the section when empty. |

Aim for under 200 words of prose, not counting logs and code. A design-heavy feature or a multi-part task can earn more.

### Show it when it reads faster

Prefer a visual form over prose when the reader can scan it faster:

- A table for expected and actual behavior, for a scope with several items, or for any comparison across the same few attributes.
- A numbered list for repro steps, and a checklist for task items.
- A code block for commands, logs, and short code examples.

Keep each table narrow and each cell short. A table with paragraphs in its cells is harder to read than prose.

### Rules

- Use real file, function, flag, error, and command names. Link related issues and PRs by number.
- Separate what you observed from what you suspect. Put guesses in "Possible cause" or "Open questions", never in "Actual".
- Do not invent a repro, a version, a log line, or an impact number.
- Do not paste a full log or stack trace. Trim it, or wrap it in `<details>`.
- Do not add labels, assignees, or milestones unless the user asks.

## Phase 5: Cut

Reread the draft as the maintainer and delete the following:

- Sentences that restate the title or another section.
- Background the maintainer already knows from the codebase.
- Qualifiers and hedges that do not change what the maintainer does.
- Story about how you found the problem, unless the path is the repro.

## Phase 6: Polish

Apply the `technical-writing` skill, which includes the `unslop` skill. Those skills own the sentence and anti-AI rules, so do not restate them here. Keep the facts and the section structure.

## Phase 7: Verify

Check the draft against the code and the output from Phase 2:

- Every claim is supported by the code, a command you ran, or the user's input.
- Each `file:line`, symbol, and version is correct.
- "Actual" describes observed behavior, and the repro steps produce it.
- The draft states plainly when something was not reproduced.
- The draft does not overstate impact or scope.

Fix any problem before you continue.

## Phase 8: Output and offer to post

Print the title and body in one fenced markdown code block so the user can copy it. Put the title on the first line as `Title: <title>`, then a blank line, then the body.

~~~markdown
```markdown
Title: <title>

<body>
```
~~~

Then ask whether to post it. Post only after the user says yes, and only to the repository from the input:

```bash
gh issue create --repo <owner/repo> --title "<title>" --body-file <file>
```

Write the body to a temporary file first so that quoting does not change it. Print the URL that `gh` returns. Do not post comments, edit other issues, or add labels.
