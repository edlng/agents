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

Make a skill universal by default. Write it without model names, provider
tool names, or install paths, and let each agent's own definition pick the
model. Create Claude and Codex variants only when a workflow cannot be
expressed neutrally. Put reusable references in `skills/_shared/`.

The `update-skill` skill describes the catalog layout and authoring rules.
Installed home directories are outputs, not authoring sources.

## Validation

Run these before reviewing a change:

```bash
node scripts/validate-catalog.mjs
node scripts/install.mjs claude --dry-run
node scripts/install.mjs codex --dry-run
```
