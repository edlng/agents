---
name: update-agent
description: Use when the user wants to update or modify an existing agent definition for their AI agents
---

> **Codex runtime:** Use Codex-native agent dispatch, task plans, user-input requests, MCP capabilities, and skill loading. Resolve agents from `~/.codex/agents` or `.codex/agents`; resolve skills from `~/.agents/skills` or `.agents/skills`.
>
> Match work to catalog roles: low effort uses `context-curator`, `explore`, or `documenter`; medium uses `builder`, `code-reviewer`, `tester`, or `researcher`; high uses `validator` or `superhuman`.
>
> **Codex model contract:** Native agents are TOML, not Markdown. In the catalog, edit `agents/<name>/codex.toml`; installed agents live at `~/.codex/agents/<name>.toml` or `.codex/agents/<name>.toml`. Preserve the exact `model` and `model_reasoning_effort` pair from `platforms/model-policy.json`. When the active Codex provider requires qualified IDs, use the complete ID such as `openai.gpt-5.6-luna`; never shorten it to `gpt-5.6-luna`.

# Update Agent

Update an agent definition across all synced roots.

**Entity type:** Agent
**Sync convention:** Follow `_shared/five-root-sync.md` (paths, sync rules, and workflow pattern for entity type "Agent").

## Additional guidance

- Codex agent files are flat TOML files (no subdirectory structure). The sync still applies identically.
- Devin-cli does not use agent markdown files (it uses `--agent-config`), so skip Root 4 when syncing agents.
- Codex does not use agent Markdown files; update its native TOML separately from the Claude Markdown variant.
- If the agent prompt references a skill by name (e.g. "Use the `code-review-excellence` skill"), verify the referenced skill exists before committing the change.
- If multiple agents share identical phrasing (e.g. the `verification-before-completion` reference), consider whether that phrasing belongs in the agent definition or can be handled by the skill's own invocation. Avoid duplicating skill instructions inside agent prompts.
