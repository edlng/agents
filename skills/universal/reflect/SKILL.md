---
name: reflect
description: Review the active session transcript for durable learnings and route each one to a concrete edit on an existing catalog skill, a description fix, or a backlog item. Use when the user says reflect or /reflect.
disable-model-invocation: true
---

# Reflect

Mine the current conversation for durable learnings, then route them into skill edits.

Invoke when the user says "reflect" or "/reflect". Skip when the conversation is trivial, off-topic, or already covered by an existing skill the session followed correctly. One-offs are not learnings.

## 1. Locate the active transcript

Read this session's transcript if the platform stores one, and only this session's. Look only in the transcript directory for the current project. Never search across other projects' transcripts, because that reads unrelated private chats. To pick the right file among recent ones, read its first user message and match it against this conversation's opening prompt. If no file matches, write a tight digest of the session instead.

## 2. Spawn one reviewer

Spawn one read-only `explore` agent at its catalog model and effort. Pass `references/reviewer.md` verbatim, substituting the transcript path or the digest where marked. The prompt sets the output format, which replaces the agent's default survey report.

## 3. Synthesize in the main session

Apply `references/synthesis.md` to the reviewer's findings yourself. Read each target skill's catalog source before you accept a body edit. Spot-check any evidence the reviewer cites that you cannot see in the conversation. The result is an Accepted, Rejected, and Backlog list.

## 4. Structural enforcement check

Move any Accepted item that a lint rule, script, metadata flag, or runtime check would enforce more reliably to Backlog. In this catalog that usually means a new check in `scripts/catalog-lib.mjs`. See the **principle-encode-lessons-in-structure** skill.

## 5. Apply

Present the full Accepted, Rejected, and Backlog output, then wait for explicit approval before applying any Accepted edit. The user picks the subset and may redirect routings. Do not auto-apply. Skill changes affect every future session that loads the skill.

Edit the catalog source under `skills/` or `agents/`, never an installed copy. Apply each approved row through the `update-skill` skill, which covers the catalog layout, ported-skill rules, and validation. Follow each row's routing:

- Trivial edit to an existing skill (a one-line bullet, a tightened sentence, a stale fact corrected): make it directly, then run `node scripts/validate-catalog.mjs`.
- Substantive edit to an existing skill (a new section, a new pattern table, more than about 10 lines): run `update-skill`.
- `tune description: <skill path>`: run `update-skill` and rewrite only the `description`.
- `new skill: <kebab-name>`: run `update-skill` to create it. Do not invent the shape ad hoc.

List Backlog items for the user to file. Do not file, commit, push, or open a PR unless the user asks.

## 6. Summarize for the user

Short list, no preamble:

- Edits applied: `<skill path>`. What changed, one line each.
- New skills created: `<skill path>`. One line each (rare).
- Backlog: `<item title>`. The suggested mechanism, one line each.
- Dropped: one line per rejected finding, with the reason.
