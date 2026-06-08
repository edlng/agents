# Shared: Valkey Cache Conventions

> This is a shared reference file used by multiple skills (`review-pr`, `review-cookbook-pr`, `review-code`, `implement-jira`). It is not a standalone skill. Do not edit one skill's copy in isolation — this file is the single source of truth for how those skills use Valkey caching.

These workflows use Valkey at `localhost:8888` as a shared cache across phases so context (diff, requirements, codebase notes, findings) is loaded once and read cheaply by all downstream subagents.

## Client

Use `valkey-glide` if available in the workspace; otherwise fall back to `valkey-cli -p 8888`. Subagents read a key via `valkey-cli -p 8888 GET <key>` or `valkey-glide`.

When `valkey-glide` has a native command implemented, use it over `custom_command` unless there is a clearly justified reason. For multi-command operations prefer `Batch` (or `ClusterBatch`) over external concurrency wrappers.

## Prompt caching

Mark cached values as Anthropic prompt-cached prefixes when they are inlined into a subagent prompt, so repeated reads are cheap.

## Key naming

Each consuming skill defines its own key prefix and TTL (documented in that skill's "Cache setup" section). The cache is a performance optimization, not a source of truth — if a key is missing, regenerate it.
