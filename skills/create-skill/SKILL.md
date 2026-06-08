---
name: create-skill
description: Use when the user wants to create a new skill for their AI agents
---

# Create Skill

## Rules

When creating a skill, you MUST:

1. Place the skill in ALL three locations:
   - `~/.kiro/skills/<skill-name>/SKILL.md`
   - `~/.claude/skills/<skill-name>/SKILL.md`
   - `~/agents/skills/<skill-name>/SKILL.md`

2. Before writing, read existing skills in each target directory to match their formatting conventions (frontmatter fields, section structure, description style).

3. Check for name conflicts across all three directories before creating. Abort if a conflict exists.

## Workflow

1. Parse the user's request to determine what the skill should do
2. Check for naming conflicts in all three target directories
3. Read an existing skill in each directory to match formatting
4. Write the SKILL.md to all three locations
5. Confirm completion with the paths created
