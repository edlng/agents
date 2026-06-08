---
name: update-skill
description: Use when the user wants to update or modify an existing skill for their AI agents
---

# Update Skill

## Rules

When updating a skill, you MUST:

1. Update the skill in ALL three locations:
   - `~/.kiro/skills/<skill-name>/SKILL.md`
   - `~/.claude/skills/<skill-name>/SKILL.md`
   - `~/agents/skills/<skill-name>/SKILL.md`

2. Before writing, read the existing skill in each target directory to understand current content and preserve proper formatting (frontmatter fields, section structure, markdown style).

3. Apply the user's changes to all three copies consistently — do not leave any location out of sync.

4. Preserve all content not targeted by the update. Do not remove or rewrite sections the user did not ask to change.

## Workflow

1. Parse the user's request to determine what changes to make and to which skill
2. Read the existing SKILL.md from each of the three locations
3. Apply the requested changes while preserving formatting and unchanged content
4. Write the updated SKILL.md to all three locations
5. Confirm completion with the paths updated and a summary of what changed
