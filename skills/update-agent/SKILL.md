---
name: update-agent
description: Use when the user wants to update or modify an existing agent definition for their AI agents
---

# Update Agent

Update an agent definition across all synced roots.

**Entity type:** Agent
**Sync convention:** Follow `_shared/four-root-sync.md` (paths, sync rules, and workflow pattern for entity type "Agent").

## Additional guidance

- Agent files are flat markdown (no subdirectory structure). The sync still applies identically.
- Devin-cli does not use agent markdown files (it uses `--agent-config`), so skip Root 4 when syncing agents.
- If the agent prompt references a skill by name (e.g. "Use the `code-review-excellence` skill"), verify the referenced skill exists before committing the change.
- If multiple agents share identical phrasing (e.g. the `verification-before-completion` reference), consider whether that phrasing belongs in the agent definition or can be handled by the skill's own invocation. Avoid duplicating skill instructions inside agent prompts.
