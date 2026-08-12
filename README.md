# Agents

## What Is Here

This repository is a source catalog for native Claude Code and Codex agents
and skills. Kiro agent files are retained for compatibility. The catalog
contains:

- 17 agent roles with Claude, Codex, and Kiro definitions.
- 18 universal skills shared by Claude and Codex.
- 23 Claude skills and 23 Codex skills for workflows whose tools or models
  differ.
- A validated installer, site catalog, and Litmus checks.

Claude agent files use the unpinned Claude Code family aliases `haiku`,
`sonnet`, and `opus`. Codex agent files use the configured Codex model IDs and
reasoning effort from `platforms/model-policy.json`.

## Quick Start

Prerequisites: Node.js, npm, and the CLI for the platform you intend to use.

```bash
npm install
node scripts/validate-catalog.mjs
node scripts/install.mjs claude
node scripts/install.mjs codex
```

Use `--dry-run` to inspect an install. Use `--force` only when replacing an
unowned destination. Use `--migrate-legacy` only when moving from the previous
shared-skill symlink layout. See [Getting Started](docs/getting-started.md) for
project-local installs and first invocation.

## Repository Map

```text
agents/<role>/           Native agent family and manifest
skills/universal/        Skills shared by Claude and Codex
skills/claude/           Claude-only workflow variants
skills/codex/            Codex-only workflow variants
skills/_shared/          References shared by platform variants
platforms/               Model policy
scripts/                 Validation, installation, and runtime checks
site/                    Read-only catalog browser
litmus/                  Claude evaluation and replay harness
```

## Documentation

- [Getting Started](docs/getting-started.md)
- [Compatibility](docs/compatibility.md)
- [Claude platform guide](docs/platforms/claude.md)
- [Codex platform guide](docs/platforms/codex.md)
- [Authoring](docs/authoring.md)
- [Testing](docs/testing.md)

## Catalog Site

Run `cd site && npm install && npm run dev` to browse the catalog locally.
The site reads the same agent and skill hierarchy used by the validator and
installer.

## Credits

The orchestration and review conventions are informed by
[CLI Agent Orchestrator](https://github.com/awslabs/cli-agent-orchestrator)
from AWS Labs.
