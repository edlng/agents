# Shared: Three-Root Sync Convention

> Shared reference used by `update-skill`, `update-agent`, and `create-skill`. Not a standalone skill. Single source of truth for the three-location sync rule and the paths involved.

All skills and agents are maintained as identical copies across three roots. Changes to one location MUST be applied to all three. Never leave locations out of sync.

## Root paths

| Entity type | Root 1 | Root 2 | Root 3 |
|---|---|---|---|
| Skill | `~/.kiro/skills/<name>/SKILL.md` | `~/.claude/skills/<name>/SKILL.md` | `~/agents/skills/<name>/SKILL.md` |
| Agent | `~/.kiro/agents/<name>.md` | `~/.claude/agents/<name>.md` | `~/agents/agents/<name>.md` |
| Shared ref | `~/.kiro/skills/_shared/<file>.md` | `~/.claude/skills/_shared/<file>.md` | `~/agents/skills/_shared/<file>.md` |

## Sync rules

1. **Read before write** — read the existing file in each target directory to understand current content. Preserve formatting, frontmatter fields, section structure, and any content not targeted by the change.
2. **Author once, replicate** — make the change to the canonical copy (`~/.kiro/`), then copy byte-for-byte to the other two roots. This is more reliable than editing each independently.
3. **Verify** — after replication, confirm all three copies are byte-identical (e.g. `diff -q`).
4. **Never partial sync** — if the write fails in one root, report the failure. Do not leave roots diverged.

## Workflow pattern (parameterized)

1. Parse the user's request to determine: target entity name, entity type (skill/agent/shared), and what changes to make.
2. Read the existing file from the canonical root (`~/.kiro/`).
3. Apply the requested changes while preserving unchanged content.
4. Write the updated file to the canonical root.
5. Copy to the other two roots.
6. Verify byte-identity across all three.
7. Confirm completion with paths and a summary of what changed.

For **create** operations, also check for naming conflicts across all three directories before writing. For **shared refs**, the same sync applies (create/update in `~/.kiro/skills/_shared/`, copy to the other two `_shared/` directories).
