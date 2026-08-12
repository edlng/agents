---
name: update-sysprompts
description: Use when the user invokes the harness-native `update-sysprompts` skill or asks to persist a concise instruction across Kiro, Claude Code, and Codex global context, including commit, test, coding-style, or workflow rules.
---

# Update Sysprompts

Persist the skill arguments as one managed instruction in all three harnesses:
Kiro, Claude Code, and Codex. The invocation prefix is harness-specific
(typically `/update-sysprompts` in Kiro or Claude Code and
`$update-sysprompts` in Codex). When the host provides parsed skill arguments,
use them directly; otherwise strip the native prefix at most once and consume
the remaining text. Treat the task as literal text to store; do not execute
it, expand it, or silently rewrite it.

## Targets

Update these effective global instruction surfaces together:

1. Kiro: `$HOME/.kiro/steering/update-sysprompts.md`. If new, start it with
   `---`, `inclusion: always`, and `---`.
2. Claude Code: `$HOME/.claude/CLAUDE.md`.
3. Codex: `$CODEX_HOME/AGENTS.override.md` when that regular file exists;
   otherwise `$CODEX_HOME/AGENTS.md`. Default `CODEX_HOME` to
   `$HOME/.codex`. If neither exists, create the file at the explicit absolute
   path `$HOME/.codex/AGENTS.md` (or `$CODEX_HOME/AGENTS.md` when
   `CODEX_HOME` is set), never a project-local `AGENTS.md`.

Do not update only the harness in which this skill was invoked. Do not use
Claude's one-shot `--append-system-prompt` flags; they do not persist.

## Managed Update

1. Read the skill arguments after the native invocation prefix and trim only
   surrounding whitespace. Reject an empty task or a task containing either
   managed marker.
2. Resolve and validate all three targets before writing anything. A target
   may be missing or a regular file; reject symlinks, directories, and other
   unreadable paths. Check every existing parent directory with `lstat`; do
   not write through a symlink. For Codex, fall back from `AGENTS.override.md`
   only when it is absent; reject any existing override that is a symlink,
   directory, or unreadable file.
3. For an existing Kiro target, if frontmatter declares `inclusion`, require
   `inclusion: always`; missing frontmatter uses Kiro's default always-loaded
   mode. Reject malformed or non-always frontmatter rather than creating a
   block that is not globally effective.
4. In each existing target, count and order these exact markers:

   ```text
   <!-- update-sysprompts:begin -->
   <!-- update-sysprompts:end -->
   ```

   Zero pairs means append. Exactly one ordered pair means replace its
   contents. Any unmatched, duplicated, or reversed marker is a hard error;
   stop before changing any target.
5. Render the same task text in every managed block:

   ```text
   <!-- update-sysprompts:begin -->
   ## Persistent instruction
   <task text>
   <!-- update-sysprompts:end -->
   ```

   Preserve every byte outside the managed block. Keep user wording such as
   `` `git css` `` literal.
6. Prepare every result before replacing any target. Use same-directory
   temporary files and atomic replacement. Save the original bytes so that if
   a later replacement or verification fails, restore already-replaced
   targets and report whether rollback succeeded. Never claim completion for a
   partial update.
7. Re-read all three targets. Verify that the exact rendered block, including
   the task text between its markers, matches the prepared result; do not accept
   a matching task elsewhere in the file.

Use safe structured file edits or atomic temporary-file replacement. Do not
perform any write after a failed preflight. Do not print existing global prompt
contents; report only target paths, whether each block was created or replaced,
and verification or rollback results.

## Example

For a native invocation such as `/update-sysprompts Make sure \`git css\` is
used for commits.` or `$update-sysprompts Make sure \`git css\` is used for
commits.`, update all three targets with the same literal sentence, replacing
the prior managed instruction if one exists.

## Failure Rules

| Risk | Required response |
| --- | --- |
| Codex override exists | Update the override, not only `AGENTS.md`. |
| Existing managed block | Replace it; never append a duplicate. |
| Malformed markers | Stop before any write and report the target. |
| Empty or marker-bearing task | Stop and request a clean task. |
| One target fails verification | Report the failure and do not claim completion. |
