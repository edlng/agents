# Cross-Platform Agent and Skill Catalog Design

Date: 2026-07-17

## Context

This repository currently stores Kiro-style JSON agent definitions beside
prompt files and copies the same skill tree into several tool-specific home
directories. The root README presents the repository as a flat inventory and
mixes catalog, installation, evaluation, and maintainer documentation.

Claude Code and Codex do not share an agent-definition format:

- Claude Code loads Markdown agents with YAML frontmatter.
- Codex loads TOML custom agents.
- Both load Agent Skills directories containing `SKILL.md`.

The existing flat sync therefore overstates compatibility. Sixteen agent
configs hardcode Claude model IDs, one agent inherits an unspecified model, and
several orchestration skills mention Claude model families directly.

A disposable live probe confirmed the target architecture:

- The same byte-identical `SKILL.md` loaded from Claude's
  `.claude/skills/` and Codex's `.agents/skills/`.
- A Claude agent loaded from `.claude/agents/` and used
  `claude-sonnet-5`.
- A Codex agent loaded from `.codex/agents/` and overrode its parent with
  `openai.gpt-5.5`, low reasoning, and a read-only sandbox.
- Generic Claude model aliases are not reliable in the current ASBX
  environment: `sonnet` routed to Opus, while `claude-sonnet-5` selected the
  intended model.

## Goals

1. Make Claude Code and Codex first-class, accurately represented platforms.
2. Keep one copy of a skill only when the same instructions genuinely work on
   both platforms.
3. Keep agent definitions native and independently tunable per platform.
4. Install either platform with one command at user or project scope.
5. Make model selection explicit, testable, and independent of prose in skills.
6. Present a concise README and a platform-aware catalog site.
7. Preserve Kiro definitions as secondary compatibility during migration.

## Non-Goals

- Treating Claude and Codex models as equivalent.
- Automatically translating arbitrary prompts between platforms.
- Making Kiro or Devin first-class installation targets in this iteration.
- Converting the repository into a Codex or Claude plugin marketplace.
- Redesigning the Litmus evaluation engine beyond path and model assertions
  required by the new layout.

## Repository Structure

```text
agents/
  builder/
    manifest.json
    claude.md
    codex.toml
    kiro.json
    kiro-prompt.md
  code-reviewer/
    ...

skills/
  universal/
    systematic-debugging/
      SKILL.md
      references/
      scripts/
  claude/
    <platform-specific-skill>/
      SKILL.md
  codex/
    <platform-specific-skill>/
      SKILL.md
  _shared/
    ...

platforms/
  model-policy.json

scripts/
  install.mjs
  validate-catalog.mjs

docs/
  getting-started.md
  compatibility.md
  platforms/
    claude.md
    codex.md
  authoring.md
  testing.md

site/
litmus/
```

The repository hierarchy is a source catalog, not a native discovery tree.
The installer materializes selected files into native platform locations.

## Agent Families

Each `agents/<name>/` directory represents one conceptual role.

`manifest.json` contains catalog metadata:

```json
{
  "name": "builder",
  "category": "implementation",
  "profile": "sonnet",
  "platforms": ["claude", "codex", "kiro"]
}
```

The profile names describe the role's relative cost and capability within the
catalog. They do not claim cross-provider model equivalence.

### Claude Variant

`claude.md` is a complete native Claude Code agent:

```markdown
---
name: builder
description: Implements one scoped task.
model: claude-sonnet-5
effort: medium
tools: Read, Write, Edit, Bash
---

Platform-tuned instructions...
```

### Codex Variant

`codex.toml` is a complete native Codex custom agent:

```toml
name = "builder"
description = "Implements one scoped task."
model = "openai.gpt-5.6-luna"
model_reasoning_effort = "xhigh"
sandbox_mode = "workspace-write"

developer_instructions = """
Platform-tuned instructions...
"""
```

The tested Codex runtime accepts `name`, `description`, model, effort, sandbox,
and developer instructions in this single native file. The installer copies it
unchanged to the platform's agent directory.

Agent instructions may share concepts, but each native file is authored and
reviewed independently. There is no prompt generator that assumes tool or model
equivalence.

The existing Kiro JSON and prompt are retained inside the role directory as
secondary compatibility. Kiro files do not determine Claude or Codex behavior.

## Model Policy

`platforms/model-policy.json` is the validation source of truth:

| Profile | Claude model | Claude effort | Codex model | Codex effort |
|---|---|---:|---|---:|
| `haiku` | `claude-haiku-4.5` | `medium` | `openai.gpt-5.6-luna` | `xhigh` |
| `sonnet` | `claude-sonnet-5` | `medium` | `openai.gpt-5.6-luna` | `xhigh` |
| `opus` | `claude-opus-4.8` | `high` | `openai.gpt-5.6-sol` | `high` |

No Claude agent may use reasoning below `medium`. Native agent files contain
the concrete model and effort values so runtime selection is inspectable.
Validation fails when a native variant differs from its manifest profile.

The currently unspecified `developer` agent is assigned the `sonnet` profile.

## Skill Classification

### Universal Skills

A skill belongs under `skills/universal/` only when:

- Its behavior is valid in both Claude Code and Codex.
- Its main instructions do not name Claude or Codex models.
- It does not depend on a platform-only agent schema or command.
- Platform tool names are expressed as intent or isolated in a small reference.
- The same `SKILL.md` passes both discovery smoke tests.

Universal skills may include `references/claude-tools.md` and
`references/codex-tools.md` when only tool vocabulary differs. The main skill
must route explicitly to the applicable reference.

### Platform-Specific Skills

A skill belongs under `skills/claude/` or `skills/codex/` when model routing,
agent configuration, permissions, invocation semantics, or platform APIs cause
the workflow to differ materially.

The same conceptual skill may exist in both platform trees with the same skill
name. Only one platform tree is installed at a time. A name may not exist in
both `universal/` and a platform-specific tree.

Migration is conservative:

1. Audit each existing skill.
2. Remove model names from workflows that can delegate by semantic agent role.
3. Place a skill in `universal/` only after static and live compatibility
   checks pass.
4. Split the remaining skills into independently maintained variants.

## Installation

The public commands are:

```bash
node scripts/install.mjs claude
node scripts/install.mjs codex
```

User scope is the default. Project scope is explicit:

```bash
node scripts/install.mjs claude --scope project --target /path/to/repo
node scripts/install.mjs codex --scope project --target /path/to/repo
```

The destinations are:

| Content | Claude user | Claude project | Codex user | Codex project |
|---|---|---|---|---|
| Agents | `~/.claude/agents/` | `.claude/agents/` | `~/.codex/agents/` | `.codex/agents/` |
| Skills | `~/.claude/skills/` | `.claude/skills/` | `~/.agents/skills/` | `.agents/skills/` |
| Shared skill references | `~/.claude/skills/_shared/` | `.claude/skills/_shared/` | `~/.agents/skills/_shared/` | `.agents/skills/_shared/` |

The installer:

1. Validates the catalog before copying.
2. Selects every native agent variant for the target platform.
3. Merges universal and target-specific skills.
4. Copies `skills/_shared/` to the target skill root so existing
   `_shared/<file>` references resolve identically on both platforms.
5. Rejects duplicate skill names.
6. Preserves complete skill directories and relative references.
7. Supports `--dry-run`.
8. Is additive by default and never deletes unrelated installed files.

Each installation writes `.agents-catalog-install.json` beside the target
platform directories. The file records the installed platform, source catalog
version, destination paths, and content hashes. A later installation may
replace a path only when that state file owns it or the existing bytes already
match. An unowned collision fails with the conflicting path; `--force` is
required to adopt and replace it. Stale catalog-owned paths are reported but
not deleted unless a future explicit cleanup command is added.

## Validation

`scripts/validate-catalog.mjs` performs deterministic checks:

- Every agent manifest has the required native variants.
- Agent names and descriptions match their native files.
- Model and effort values match `model-policy.json`.
- Claude effort is never below `medium`.
- Universal skills contain no concrete Claude or Codex model IDs.
- Skill names match their immediate parent directory.
- Skill frontmatter satisfies the Agent Skills minimum fields.
- Universal and platform-specific install sets contain no duplicate names.
- Relative references remain inside the copied skill directory or resolve
  through the copied `_shared/` directory.
- Site and Litmus source paths resolve.

The validator uses structured JSON and YAML parsing. TOML assertions are
limited to the supported agent fields and are confirmed by Codex runtime smoke
tests.

## Runtime Verification

Tests install into isolated temporary user and project roots.

### Claude

1. Invoke a universal project skill and assert the expected token.
2. Launch representative Haiku, Sonnet, and Opus agents.
3. Read Claude JSON output or session records.
4. Assert exact model IDs and effort values.

### Codex

1. Invoke the same universal project skill and assert the expected token.
2. Spawn representative Luna and Sol custom agents.
3. Read isolated Codex session records.
4. Assert exact model IDs, reasoning efforts, agent roles, and sandbox modes.

Runtime tests are bounded and opt-in because they make model calls. Static
validation and installer tests run on every normal verification pass.

## README And Documentation

The root README becomes a short entry point:

1. What the catalog contains.
2. Claude and Codex quick-install commands.
3. Link to the catalog site.
4. Small repository map.
5. Links to detailed documentation.

Large inventories and the full Litmus manual move out of the README.

- `docs/getting-started.md`: installation and first use.
- `docs/compatibility.md`: universal versus platform-specific guarantees.
- `docs/platforms/claude.md`: native paths, models, and verification.
- `docs/platforms/codex.md`: native paths, models, and verification.
- `docs/authoring.md`: adding and classifying agents and skills.
- `docs/testing.md`: static, installer, site, Litmus, and live checks.

Historical design and implementation records remain under
`docs/superpowers/`.

## Catalog Site

The site extractor reads the new role and skill hierarchy.

Agent cards represent conceptual roles and show:

- Supported platforms.
- Profile.
- Exact model and effort per platform.
- Tabs for Claude, Codex, and legacy Kiro source.

Skill cards show one of:

- Universal.
- Claude.
- Codex.
- Claude and Codex variants grouped as one conceptual capability.

The site keeps its existing Overview, Catalog, and Workflows navigation. The
change is data and compatibility presentation, not a broad visual redesign.

## Litmus Migration

Litmus cases continue to target conceptual agent names. Provider adapters
resolve the requested name to the selected native variant.

Existing replays remain historical artifacts. New path and model assertions
are added without rewriting old recorded outputs. The core live manifest gains
only bounded loading/model-selection probes until broader cross-platform cases
are explicitly vetted.

## Error Handling

- Missing native variant: validation and installation fail with the role name.
- Unsupported or unavailable model: live verification fails and reports the
  configured model and installed CLI catalog.
- Duplicate skill name: installation stops before copying.
- Invalid skill metadata: validation reports the exact `SKILL.md`.
- Existing destination file: dry-run reports the change; normal installation
  replaces only files recorded in `.agents-catalog-install.json` or byte-equal
  files. Other collisions require `--force`.
- Partial copy failure: installation reports copied and pending paths and exits
  non-zero. It never deletes pre-existing unrelated content.

## Migration Order

1. Add model policy, manifests, validator, and installer tests.
2. Convert all agent roles to native Claude and Codex variants while retaining
   Kiro files.
3. Audit and move skills into universal or platform-specific trees.
4. Update Make targets and remove the five-root copy model as the public
   architecture.
5. Update the site extractor and compatibility UI.
6. Rewrite README and add focused documentation.
7. Update Litmus paths and add bounded runtime probes.
8. Run static, installer, site, replay, and live model-selection verification.

## Acceptance Criteria

- Every catalog agent has valid Claude and Codex native variants.
- Every native agent matches the requested model policy exactly.
- No Claude agent uses effort below `medium`.
- Universal skills are byte-identical when installed for both platforms.
- One-command user and project installation works in isolated directories.
- Claude and Codex discover a universal skill after installation.
- Claude reports the configured Haiku, Sonnet, and Opus model IDs.
- Codex child sessions report Luna `xhigh` for Haiku/Sonnet profiles and Sol
  `high` for Opus profiles.
- The site builds and displays platform compatibility and model details.
- The root README is concise and links to detailed documentation.
- Existing Litmus replays remain usable after path migration.
