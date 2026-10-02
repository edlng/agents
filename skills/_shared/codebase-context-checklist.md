# Shared: Codebase Context Checklist

> Shared reference used by `review-pr`, `review-code`, and `implement-task`. Not a standalone skill. Single source of truth for what to capture when scanning a codebase before reviewing or implementing.

Read the touched files plus 1-2 callers/neighbors of the most non-trivial ones. For PR reviews, fetch file state from the PR's head ref; for local review and implementation, read the current working-tree state. Do not modify the user's working tree while gathering context.

Capture:

- Language, runtime, and package manager, from `pyproject.toml`, `setup.py`, `package.json`, `pom.xml`, `build.gradle`, `go.mod`, etc.
- Conventions: naming (snake_case vs camelCase), indentation, docstring/comment format, type-annotation usage, import ordering.
- Patterns in use: module structure, how errors are raised and handled, logging style, common base classes or decorators.
- Existing utilities: helpers/abstractions the new code should call rather than re-implement. Flag any case where the diff reimplements something that already exists nearby.
- Test conventions: test file naming, fixture patterns, mocks vs real objects, assertion style, unit vs integration split.
