---
name: update-skill
description: Use when the user wants to update or modify an existing skill for their AI agents
---

# Update Skill

Update a skill across all synced roots.

**Entity type:** Skill
**Sync convention:** Follow `_shared/four-root-sync.md` (paths, sync rules, and workflow pattern for entity type "Skill").

## Additional guidance

- If the skill references `_shared/` files, check whether the change belongs in the shared file instead (DRY). If so, update the shared file and sync it per the "Shared ref" row in the sync convention.
- If the change affects the skill's frontmatter `description`, verify it still accurately reflects when the skill should be invoked.
- If the skill has auxiliary files (e.g. `references/`, prompt templates), sync those too using the same author-once-replicate pattern.
