---
name: update-skill
description: Creates or updates a skill or agent definition in the AI agents catalog, following the catalog layout and authoring rules. Use when the user wants to create a new skill or agent, or modify an existing one.
disable-model-invocation: true
---

# Update Skill

Create or update a skill or agent in the catalog. The repository catalog is the source of truth. Edit the catalog source, then validate it. Do not treat an installed copy as canonical.

## Catalog layout

| Entity | Source files |
|---|---|
| Agent | `agents/<name>/{manifest.json,claude.md,codex.toml,kiro.json,kiro-prompt.md}` |
| Universal skill (default) | `skills/universal/<name>/` |
| Claude-only skill (rare) | `skills/claude/<name>/` |
| Codex-only skill (rare) | `skills/codex/<name>/` |
| Shared reference | `skills/_shared/<file>.md` |

Platform-specific skills are rare. Write a universal skill unless the skill depends on a feature only one platform has. Agents also have Kiro files (`kiro.json`, `kiro-prompt.md`). The installer does not manage the Kiro files, so update them by hand.

## Authoring rules

1. Read the existing source and keep content outside the requested change.
2. Keep universal skills free of model names and provider-specific tool names. Name MCP tools neutrally (for example "the Jira MCP `getJiraIssue` tool") and refer to agents by catalog name (for example "spawn a `builder` agent").
3. Use the platform-native format for each agent file.
4. Run `node scripts/validate-catalog.mjs` after changing catalog files.

## Creating a skill or agent

- Check for a naming conflict across `agents/`, `skills/universal/`, `skills/claude/`, and `skills/codex/` before writing.
- Read 1-2 existing skills or agents to match formatting conventions (frontmatter fields, section structure, description style).
- Use kebab-case names that describe the capability and stay narrow in scope.
- If the new skill shares logic with an existing skill, move the shared part into `_shared/` rather than duplicating it.
- Write the `description` in third person: what the skill does, then "Use when ...". Include enough keywords for accurate invocation matching.

## Updating a skill

- If the skill references `_shared/` files, check whether the change belongs in the shared file instead. If so, update the shared file.
- If the change affects the frontmatter `description`, verify it still reflects when the skill should be invoked.
- If the skill has auxiliary files (for example `references/`, `agents/openai.yaml`, prompt templates), update those too.

## Updating an agent

- An agent has five source files: `manifest.json`, `claude.md`, `codex.toml`, `kiro.json`, and `kiro-prompt.md`. Apply the change to every file it affects.
- If the agent prompt references a skill by name (for example "Use the `code-review-excellence` skill"), verify the referenced skill exists.
- If several agents share identical instructions, consider whether they belong in a shared workflow instead of each agent prompt.

## Install and verify

Preview what each platform install would write, including the target directories, with:

```text
node scripts/install.mjs claude --dry-run
node scripts/install.mjs codex --dry-run
```

Each platform installs agents and skills into its own user or project directories. The dry run prints the exact paths. Run `node scripts/validate-catalog.mjs` before finishing.
