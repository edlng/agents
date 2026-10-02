---
name: update-skill
description: Use when the user wants to create a new skill or agent, or update or modify an existing skill or agent definition, in their AI agents catalog
---

> **Codex runtime:** Use Codex-native agent dispatch, task plans, user-input requests, MCP capabilities, and skill loading. Resolve agents from `~/.codex/agents` or `.codex/agents`; resolve skills from `~/.agents/skills` or `.agents/skills`.
>
> Match work to catalog roles: low effort uses `explore`; medium uses `builder`, `code-reviewer`, `tester`, or `researcher`; high uses `validator`.
>
> **Codex model contract:** Skills do not select models directly. When a skill dispatches an agent, resolve that role through its native TOML and verify the exact provider-qualified `model` plus `model_reasoning_effort` pair from `platforms/model-policy.json`. Do not hardcode shortened aliases such as `gpt-5.6-luna` when the active provider requires `openai.gpt-5.6-luna`.

# Update Skill

Create or update a skill or agent in the catalog.

**Sync convention:** Follow `_shared/five-root-sync.md` (catalog layout, authoring rules, installation and verification).

## Creating a skill or agent

- Check for a naming conflict across `agents/`, `skills/universal/`, `skills/claude/`, and `skills/codex/` before writing.
- Read 1-2 existing skills or agents to match formatting conventions (frontmatter fields, section structure, description style).
- Names must be kebab-case, descriptive of the capability, and narrow in scope.
- If the new skill shares logic with an existing skill, move the shared part into `_shared/` rather than duplicating it.
- Verify the `description` field contains enough keywords for accurate invocation matching.

## Updating a skill

- If the skill references `_shared/` files, check whether the change belongs in the shared file instead. If so, update the shared file.
- If the change affects the frontmatter `description`, verify it still reflects when the skill should be invoked.
- If the skill has auxiliary files (e.g. `references/`, prompt templates), update those too.

## Updating an agent

- An agent has five source files: `manifest.json`, `claude.md`, `codex.toml`, `kiro.json`, and `kiro-prompt.md`. Apply the change to every platform file it affects.
- If the agent prompt references a skill by name (e.g. "Use the `code-review-excellence` skill"), verify the referenced skill exists.
- If several agents share identical instructions, consider whether they belong in a shared workflow instead of each agent prompt.
