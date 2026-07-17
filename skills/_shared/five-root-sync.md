# Shared: Catalog Authoring Convention

This reference is used by `create-skill`, `update-skill`, and `update-agent`.
The repository catalog is the source of truth. Platform-native files are
authored in their own format and then validated from the catalog.

## Catalog layout

| Entity | Source files |
|---|---|
| Agent | `agents/<name>/{manifest.json,claude.md,codex.toml,kiro.json,kiro-prompt.md}` |
| Universal skill | `skills/universal/<name>/` |
| Claude skill | `skills/claude/<name>/` |
| Codex skill | `skills/codex/<name>/` |
| Shared reference | `skills/_shared/<file>.md` |

## Authoring rules

1. Read the existing source and preserve content that is outside the requested
   change.
2. Keep universal skills free of provider-specific model IDs and tool names.
3. Keep Claude and Codex variants behaviorally equivalent, but use each
   platform's native agent, tool, model, and installation vocabulary.
4. Update the platform-native source directly. Do not copy files across home
   directories or treat an installed copy as canonical.
5. Run `node scripts/validate-catalog.mjs` after changing catalog files.

## Installation and verification

Preview the relevant platform install with:

```text
node scripts/install.mjs claude --dry-run
node scripts/install.mjs codex --dry-run
```

Claude installs agents under `~/.claude/agents` or `.claude/agents` and skills
under `~/.claude/skills` or `.claude/skills`. Codex installs agents under
`~/.codex/agents` or `.codex/agents` and skills under `~/.agents/skills` or
`.agents/skills`.
