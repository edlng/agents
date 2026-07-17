# Authoring

## Agent Roles

Each role has one directory:

```text
agents/<name>/
  manifest.json
  claude.md
  codex.toml
  kiro.json
  kiro-prompt.md
```

Keep the manifest name, description, category, profile, and platform list
consistent with the native files. Select models only through the policy in
`platforms/model-policy.json`.

## Skill Classification

Classify a skill as universal when no platform-specific behavior is needed.
Otherwise create a Claude and Codex variant with the same trigger and outcome,
then adapt dispatch, input, MCP, paths, and model-routing instructions to the
native runtime. Put reusable references in `skills/_shared/`.

Use the catalog convention in `skills/_shared/five-root-sync.md`; installed
home directories are outputs, not authoring sources.

## Validation

Run these before reviewing a change:

```bash
node scripts/validate-catalog.mjs
node scripts/install.mjs claude --dry-run
node scripts/install.mjs codex --dry-run
```
