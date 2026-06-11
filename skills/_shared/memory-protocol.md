# Shared: Memory Protocol

> Shared reference for agents that store or consume persistent memory. Defines scopes, storage backends, injection format, and conventions.

## Overview

Agents persist decisions, preferences, and project knowledge across sessions. A **context-curator** agent selects relevant memories before worker dispatch, keeping worker context focused.

## Storage Backends

| Backend | Use | Access |
|---------|-----|--------|
| **Valkey** (localhost:8888) | Hot cache, session-scoped, fast lookup | `valkey-cli -p 8888` or `valkey-glide` |
| **Obsidian** (vault notes) | Durable long-term memory, human-readable | MCP Obsidian tools (`read_note`, `search_notes`, `write_note`) |

Valkey is the fast path for inter-session sharing. Obsidian is the durable store and audit trail.

## Memory Scopes

| Scope | TTL | Storage | Use when |
|-------|-----|---------|----------|
| `session` | 4h (Valkey), not persisted to Obsidian | Valkey only | Ephemeral coordination within a pipeline run |
| `project` | 90d | Valkey + Obsidian | Architecture decisions, conventions, file context for the current repo |
| `global` | Never expires | Obsidian only | User preferences, cross-project coding standards |
| `agent` | Never expires | Obsidian only | Role-specific patterns a particular agent role always applies |

## Key Naming (Valkey)

```
mem:{scope}:{project_id}:{key}
```

- `project_id` = git remote slug (normalized) or `sha256(realpath(cwd))[:12]`
- For `global`/`agent` scope: `project_id` = `_global`
- Key = slug derived from content (kebab-case, max 64 chars)

Examples:
```
mem:project:awslabs-valkey-glide:testing-framework
mem:session:awslabs-valkey-glide:current-branch-context
mem:global:_global:user-prefers-concise
mem:agent:_global:code-reviewer-patterns
```

## Storage Format (Obsidian)

Memories live in the vault under `memory/`:
```
memory/
├── global/
│   └── user-prefers-concise.md
├── agent/
│   └── code-reviewer-patterns.md
└── project/
    └── {project-slug}/
        └── testing-framework.md
```

Each note:
```markdown
---
scope: project
key: testing-framework
tags: [testing, pytest]
created: 2026-06-11T08:30:00Z
updated: 2026-06-11T08:30:00Z
---

Always use pytest for testing. Do not use unittest. Fixtures live in conftest.py at the repo root.
```

## Injection Format

The context-curator produces a `<context-memory>` block prepended to worker instructions:

```
<context-memory>
- [project] testing-framework: Always use pytest. Fixtures in conftest.py at repo root.
- [global] user-prefers-concise: User prefers concise responses without trailing summaries.
- [session] current-branch: Working on feature/add-vector-search, base is main.
</context-memory>
```

Budget: max 3000 characters. Fewer high-quality entries over many low-relevance ones.

## Storing Memories

Any agent can store a memory when it discovers something worth persisting:

1. Write to Valkey with appropriate TTL:
   ```bash
   valkey-cli -p 8888 SET "mem:project:{id}:{key}" "{content}" EX {ttl_seconds}
   ```
2. For durable memories (project/global/agent scope), also write an Obsidian note via `write_note`.

Keep memories to 1-2 sentences. Store decisions and conclusions, not conversation.

## Recalling Memories

Search Valkey first (fast path), fall back to Obsidian `search_notes` for durable memories:

```bash
# Scan for project memories
valkey-cli -p 8888 KEYS "mem:project:{id}:*"

# Get a specific memory
valkey-cli -p 8888 GET "mem:project:{id}:testing-framework"
```

## When to Store

Store immediately when you discover:
- User preferences or corrections ("I prefer X over Y")
- Project conventions not captured in config files
- Architecture decisions and their rationale
- Recurring mistakes or corrections
- Important context about the current task that downstream agents need

## When NOT to Store

- Transient debugging output
- Full file contents (reference by path instead)
- Anything already in committed config/docs (point to the file)
- Conversation filler or intermediate reasoning
