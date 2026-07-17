# Codex

## Native Paths

User installs use `~/.codex/agents` for custom agents and `~/.agents/skills`
for skills. Project installs use `.codex/agents` and `.agents/skills`.
Source files are:

- Agents: `agents/<role>/codex.toml`
- Skills: `skills/universal/<skill>/` plus `skills/codex/<skill>/`

## Agent Format

Codex agents are TOML files. The required fields are `name`, `description`,
`model`, `model_reasoning_effort`, and the appropriate `sandbox_mode`; the
`developer_instructions` value is the system prompt.

| Profile | Model | Effort |
|---|---|---|
| Haiku | `openai.gpt-5.6-luna` | `xhigh` |
| Sonnet | `openai.gpt-5.6-luna` | `xhigh` |
| Opus | `openai.gpt-5.6-sol` | `high` |

Validate and preview an install with:

```bash
node scripts/validate-catalog.mjs
node scripts/install.mjs codex --dry-run
```
