# Shared: Codebase Context Checklist

> Shared reference used by `review-pr`, `review-cookbook-pr`, `review-code`, and `implement-jira`. Not a standalone skill. Single source of truth for what to capture when scanning a codebase before reviewing or implementing.

Read the touched files plus 1-2 callers/neighbors of the most non-trivial ones. For PR reviews, fetch file state from the PR's head ref; for local review and implementation, read the current working-tree state. **Do not modify the user's working tree** while gathering context.

Capture:

- **Language, runtime, package manager** — from `pyproject.toml`, `setup.py`, `package.json`, `pom.xml`, `build.gradle`, `go.mod`, etc.
- **Conventions** — naming (snake_case vs camelCase), indentation, docstring/comment format, type-annotation usage, import ordering.
- **Patterns in use** — module structure, how errors are raised and handled, logging style, common base classes or decorators.
- **Existing utilities** — helpers/abstractions the new code should call rather than re-implement. Flag any case where the diff reimplements something that already exists nearby.
- **Test conventions** — test file naming, fixture patterns, mocks vs real objects, assertion style, unit vs integration split.
- **Key/value client** — `valkey-glide` is the standard. Flag any introduction of `redis-py`, `ioredis`, `jedis`, or other Redis clients unless there is a documented reason. When valkey-glide is used, prefer `Batch`/`ClusterBatch` for multi-command operations over `asyncio.gather`, `Promise.all` with individual commands, or pipeline wrappers — Batch is atomic, reduces round-trips, and is the idiomatic GLIDE pattern.
