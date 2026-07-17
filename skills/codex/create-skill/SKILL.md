---
name: create-skill
description: Use when the user wants to create a new skill for their AI agents
---

> **Codex runtime:** Use Codex-native agent dispatch, task plans, user-input requests, MCP capabilities, and skill loading. Resolve agents from `~/.codex/agents` or `.codex/agents`; resolve skills from `~/.agents/skills` or `.agents/skills`.
>
> Match work to catalog roles: low effort uses `context-curator`, `explore`, or `documenter`; medium uses `builder`, `code-reviewer`, `tester`, or `researcher`; high uses `validator` or `superhuman`.

# Create Skill

Create a new skill across all synced roots.

**Entity type:** Skill
**Sync convention:** Follow `_shared/five-root-sync.md` (paths, sync rules, and workflow pattern for entity type "Skill"). For create operations, also check for naming conflicts across all five directories before writing.

## Additional guidance

- Before writing, read 1-2 existing skills to match formatting conventions (frontmatter fields, section structure, description style).
- Skill names must be kebab-case, descriptive of the capability, and narrow in scope.
- If the new skill shares logic with an existing skill, extract the commonality into `_shared/` rather than duplicating it. Sync the shared file per the "Shared ref" row in the sync convention.
- After creation, verify the skill's `description` field contains enough keywords for accurate invocation matching.
