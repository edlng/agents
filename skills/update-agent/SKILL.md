---
name: update-agent
description: Use when the user wants to update or modify an existing agent definition for their AI agents
---

# Update Agent

## Rules

When updating an agent, you MUST:

1. Update the agent in ALL three locations:
   - `~/.kiro/agents/<agent-name>.md`
   - `~/.claude/agents/<agent-name>.md`
   - `~/agents/agents/<agent-name>.md`

2. Before writing, read the existing agent file in each target directory to understand current content and preserve proper formatting (frontmatter fields, section structure, markdown style).

3. Apply the user's changes to all three copies consistently — do not leave any location out of sync.

4. Preserve all content not targeted by the update. Do not remove or rewrite sections the user did not ask to change.

## Workflow

1. Parse the user's request to determine what changes to make and to which agent
2. Read the existing agent file from each of the three locations
3. Apply the requested changes while preserving formatting and unchanged content
4. Write the updated agent file to all three locations
5. Confirm completion with the paths updated and a summary of what changed
