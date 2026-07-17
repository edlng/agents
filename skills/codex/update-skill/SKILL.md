---
name: update-skill
description: Use when the user wants to update or modify an existing skill for their AI agents
---

> **Codex runtime:** Use Codex-native agent dispatch, task plans, user-input requests, MCP capabilities, and skill loading. Resolve agents from `~/.codex/agents` or `.codex/agents`; resolve skills from `~/.agents/skills` or `.agents/skills`.
>
> Match work to catalog roles: low effort uses `context-curator`, `explore`, or `documenter`; medium uses `builder`, `code-reviewer`, `tester`, or `researcher`; high uses `validator` or `superhuman`.

# Update Skill

Update a skill across all synced roots.

**Entity type:** Skill
**Sync convention:** Follow `_shared/five-root-sync.md` (paths, sync rules, and workflow pattern for entity type "Skill").

## Additional guidance

- If the skill references `_shared/` files, check whether the change belongs in the shared file instead (DRY). If so, update the shared file and sync it per the "Shared ref" row in the sync convention.
- If the change affects the skill's frontmatter `description`, verify it still accurately reflects when the skill should be invoked.
- If the skill has auxiliary files (e.g. `references/`, prompt templates), sync those too using the same author-once-replicate pattern.
