# Claude

## Native Paths

User installs use `~/.claude/agents` and `~/.claude/skills`. Project installs
use `.claude/agents` and `.claude/skills`. Source files are:

- Agents: `agents/<role>/claude.md`
- Skills: `skills/universal/<skill>/` plus `skills/claude/<skill>/`

## Agent Format

Claude agents are Markdown files with YAML frontmatter. The required identity
fields are `name`, `description`, `model`, and `effort`; the body is the
system prompt. The manifest beside it supplies the platform-neutral role and
profile.

| Profile | Model alias | Minimum effort |
|---|---|---|
| Haiku | `haiku` | `medium` |
| Sonnet | `sonnet` | `medium` |
| Opus | `opus` | `high` |

These are Claude Code family aliases, not pinned model revisions. Claude Code
resolves them to the provider's current Haiku, Sonnet, or Opus model. If a
deployment needs explicit routing, set `ANTHROPIC_DEFAULT_HAIKU_MODEL`,
`ANTHROPIC_DEFAULT_SONNET_MODEL`, or `ANTHROPIC_DEFAULT_OPUS_MODEL` outside
this catalog.

Validate the complete source catalog with:

```bash
node scripts/validate-catalog.mjs
node scripts/install.mjs claude --dry-run
```
