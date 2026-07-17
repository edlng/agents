# Getting Started

## Prerequisites

Install Node.js and npm. Install Claude Code or Codex separately when using
live runtime checks. From the repository root:

```bash
npm install
node scripts/validate-catalog.mjs
```

## Install For a User

```bash
node scripts/install.mjs claude
node scripts/install.mjs codex
```

Claude receives agents in `~/.claude/agents` and skills in `~/.claude/skills`.
Codex receives agents in `~/.codex/agents` and skills in `~/.agents/skills`.

Use `--dry-run` to inspect the complete plan without writing:

```bash
node scripts/install.mjs codex --dry-run
```

If migrating from the previous shared-skill layout, materialize only the
known Claude skill symlinks with an explicit opt-in:

```bash
node scripts/install.mjs claude --migrate-legacy --dry-run
node scripts/install.mjs claude --migrate-legacy
```

The Codex destination keeps its collision protection. Review conflicts first,
then use `--force` only for unowned files you have confirmed should be replaced:

```bash
node scripts/install.mjs codex --dry-run
node scripts/install.mjs codex --force
```

## Install For a Project

Pass `--scope project` from the project directory. Use `--target` to select a
different root:

```bash
node scripts/install.mjs claude --scope project
node scripts/install.mjs codex --scope project --target /path/to/project
```

The installer records ownership in `.agents-catalog-install.json`, writes
atomically, preserves executable modes, and refuses unowned collisions unless
`--force` is supplied. It does not uninstall files; remove owned destinations
manually after reviewing the state file.

## First Invocation

After installation, invoke a skill by its name in the selected CLI. Start
with a focused role such as `builder`, `code-reviewer`, or `researcher`, then
use the platform-specific workflow skill when the task requires it.
