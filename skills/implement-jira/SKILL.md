---
name: implement-jira
description: Implement a Jira task end-to-end — fetches requirements, scans the codebase, plans with Opus, implements with Sonnet (delegating low-complexity subtasks to Haiku), runs tests with a fix loop, then runs a merged code review. Uses Valkey at localhost:8888 as a two-tier cache. Use when given a Jira issue key (e.g. FOO-123). Do NOT use for tasks without a Jira ticket, quick fixes, or exploratory coding — handle those directly.
---

# Implement Jira Task

Implement the Jira task `$ARGUMENTS` end-to-end using the workflow below. Follow each phase in order. Do not skip phases.

This workflow assigns specific models to each subagent for cost efficiency, and uses a two-tier cache (Anthropic prompt caching + Valkey at `localhost:8888`) so Requirements and Plan are loaded once and read cheaply by all downstream stages.

---

## Cache setup (run once at start)

The Valkey server is expected at `localhost:8888`. Use `valkey-glide` if available in the workspace; otherwise fall back to `valkey-cli -p 8888`.

Cache keys for this run:
- `jira:$ARGUMENTS:requirements` — full Requirements Document (TTL 24h)
- `jira:$ARGUMENTS:codebase_context` — Codebase Context note (TTL 24h)
- `jira:$ARGUMENTS:plan` — Planning output (TTL 24h)

Before Phase 1, check if `jira:$ARGUMENTS:requirements` already exists in Valkey. If yes and the user has not asked to refresh, skip Phase 1 and reuse the cached Requirements.

When spawning subagents in later phases, pass cache keys instead of full text where possible. Subagents read from Valkey via Bash (`valkey-cli -p 8888 GET <key>`) or `valkey-glide`. Also mark Requirements + Plan as Anthropic prompt-cached prefixes when they are inlined.

---

## Phase 1: Requirements

**Model: sonnet-4-6**

Use `mcp__atlassian__getJiraIssue` to fetch the Jira issue `$ARGUMENTS`. Extract:
- Summary (title)
- Description (full text)
- Acceptance criteria (if present in description or a custom field)
- Any linked issues or sub-tasks

Produce a concise **Requirements Document** with:
1. What must be built
2. Explicit acceptance criteria (infer them if not stated)
3. Edge cases or constraints mentioned in the ticket
4. Implicit constraints (security, perf, compatibility) that should not be missed

Write the result to Valkey: `valkey-cli -p 8888 SET jira:$ARGUMENTS:requirements "<doc>" EX 86400`.

---

## Phase 2: Codebase Scan

**Model: sonnet-4-6** (same agent as Phase 1 — no spawn needed)

Read the existing codebase to understand:
- **Language and runtime**: check `pyproject.toml`, `setup.py`, `package.json`, `pom.xml`, `build.gradle`, etc.
- **Code style**: indentation, naming conventions (snake_case vs camelCase), docstring format, type annotation usage, import ordering
- **Patterns in use**: how existing modules are structured, how errors are raised and handled, how logging is done, common base classes or decorators used
- **Existing utilities**: functions, helpers, or abstractions already present that the new code should call rather than re-implement
- **Dependencies**: what packages are already imported and used. If the task requires a key/value store or cache client, **use `valkey-glide`** — do not introduce `redis-py` or any other Redis-compatible client
- **Test conventions**: test file naming, fixture patterns, use of mocks vs real objects, assertion style

Produce a short **Codebase Context** note. Write to Valkey: `valkey-cli -p 8888 SET jira:$ARGUMENTS:codebase_context "<note>" EX 86400`.

---

## Phase 3: Planning

Spawn a single `superhuman` subagent. Pass the cache keys (subagent fetches Requirements + Codebase Context from Valkey).

Prompt: "You are the planner. **Effort budget: 5-10 tool calls to read context, then produce the plan.** Read `jira:$ARGUMENTS:requirements` and `jira:$ARGUMENTS:codebase_context` from Valkey at localhost:8888. Decompose the work into an ordered subtask list. For each subtask, output:
- `id`: short identifier
- `description`: one line
- `files`: list of files this subtask touches
- `complexity`: `low` or `medium`
- `depends_on`: list of subtask ids that must complete first

Tag a subtask `low` only if it is mechanical (boilerplate, config, simple CRUD, format conversion) AND touches files disjoint from any `medium` subtask. Otherwise tag `medium`. Output strict JSON.

Write the resulting plan to `jira:$ARGUMENTS:plan` in Valkey."

---

## Phase 4: Implementation

Spawn one `builder` subagent as the Implementor.

Prompt: "You are the implementor. **Effort budget: 30-60 tool calls total across all subtasks.** Read `jira:$ARGUMENTS:plan`, `jira:$ARGUMENTS:requirements`, and `jira:$ARGUMENTS:codebase_context` from Valkey at localhost:8888.

For each subtask in the plan, in dependency order:
- If `complexity == medium`: implement it yourself.
- If `complexity == low` AND its `files` do not overlap any other subtask currently in flight: spawn another `builder` subagent to handle it. Pass only the subtask description, the relevant files list, and the Codebase Context.
- Otherwise: handle it yourself.

You MUST match the existing codebase exactly: same language, same code style, same naming conventions, same error-handling patterns, same logging approach. Reuse existing utilities and helpers — do not re-implement what already exists. If a cache or key/value client is needed, use `valkey-glide`. When `valkey-glide` has a native command implemented, you MUST use that over `custom_command` unless there is a clearly justified reason. Write minimal code; do not add abstractions beyond what the requirements ask for."

---

## Phase 5: Tests

Spawn a `tester` subagent. Pass cache keys.

Prompt: "Read `jira:$ARGUMENTS:requirements` and `jira:$ARGUMENTS:codebase_context` from Valkey at localhost:8888.

**Effort budget: 20-40 tool calls. Prioritize writing and running tests over exhaustive codebase scanning.**

### 5.1 — Discover existing test patterns

Before writing any tests, scan the codebase to understand what already exists:

```bash
# Find all test files
find . -type f \( \
  -name '*.test.ts' -o -name '*.spec.ts' -o -name '*.test.js' -o -name '*.spec.js' \
  -o -name 'test_*.py' -o -name '*_test.py' \
  -o -name '*_test.go' -o -name '*Test.java' \
\) -not -path '*/node_modules/*' -not -path '*/.git/*'

# Detect integration test directories or markers
find . -type d \( -name 'integration' -o -name 'e2e' -o -name 'integration_tests' \) \
  -not -path '*/node_modules/*'
grep -rn 'integration\|e2e\|IntegrationTest\|@pytest.mark.integration' \
  --include='*.py' --include='*.ts' --include='*.js' --include='*.go' \
  -l . | grep -v node_modules
```

Read at least 2–3 existing test files in full to internalize: test framework, fixture/setup patterns, how mocks are constructed, assertion style, and how integration tests differ from unit tests (e.g., real DB, real network, test containers, env flags).

### 5.2 — Unit tests

Write unit tests for every non-trivial function/method introduced by this implementation.

**What makes a unit test valid:**
- Tests a specific behaviour, not merely that code runs without error
- Every test asserts a concrete, meaningful outcome: a return value, a raised exception, a side-effect on a collaborator (via spy/mock), or a state change
- Each test covers exactly one logical case: happy path, a specific error condition, a boundary value, or an edge case from the requirements
- Mocks/stubs are used only to isolate the unit from I/O or external services — set them up to return realistic values, not empty dicts or None unless that is the specific case under test
- Test names describe what is being tested AND the expected outcome (e.g., `test_embed_returns_float32_vector_of_correct_dimension`, not `test_embed`)
- Do NOT write: tests that only assert a function was called with no result checked, tests where the assertion would pass even if the function body were deleted, or tests that are identical copies with no variation in inputs or expected outcomes

Derive test cases directly from the acceptance criteria and edge cases in the Requirements Document. Every acceptance criterion must have at least one test.

### 5.3 — Integration tests

Check whether the codebase has integration tests (from 5.1). If yes:

- Follow the exact same pattern: directory structure, naming convention, markers, fixtures, setup/teardown
- Write integration tests that exercise the full path through the new code against real collaborators (real DB, real Valkey, real API, test server) — not mocked ones
- Each integration test must verify an observable end-to-end outcome: a record written and readable from a real store, a real HTTP response body, a real query result matching expected data

If the codebase has no integration tests, check whether the acceptance criteria describe behaviour that inherently requires real I/O (e.g., 'data must be persisted', 'query must return matching results'). If so, create an `integration/` directory (or the project equivalent) and write at least one integration test with a comment explaining the external dependency needed to run it.

If integration tests are genuinely not applicable (pure logic, no I/O), state that explicitly and do not create placeholder files.

### 5.4 — Run the test suite

Detect the test runner from project config (`pytest.ini`, `pyproject.toml [tool.pytest]`, `jest.config.*`, `go test`, `Makefile` targets, etc.). Default for Python: `python -m pytest -x --tb=short`. Run and capture full output.

### Fix loop (autonomous)
If tests fail:
- Analyze the failure output
- If the failure suggests the **plan** was wrong (missing subtask, wrong partition, missed dependency): jump back to **Phase 3 Planning** with the failure context appended.
- Otherwise (implementation or test bug): fix the appropriate file(s) and re-run.
- Repeat until all tests pass — no human approval needed.

A test that passes only because an assertion was removed, an exception was swallowed, or a mock was made permissive is NOT a fix — fix the implementation or the test logic instead."

---

## Phase 6: Code Review (merged)

Run `git diff` (or list all created/modified files with full contents if not a git repo) to produce a **Changes** snapshot.

Spawn ONE `code-reviewer` subagent. The single subagent applies all three review lenses in one pass — this loads the diff once instead of three times.

Prompt: "Read `jira:$ARGUMENTS:requirements` from Valkey at localhost:8888. Review the following Changes through three lenses, in order. Output strict JSON with three top-level keys: `requirements_alignment`, `code_review_excellence`, `optimization_and_dead_code`. Each value is a list of findings; each finding has `file`, `line_range`, `severity` (`blocking` or `suggestion`), and `description`.

**Lens 1 — Requirements Alignment**
Check: (1) Does the implementation satisfy every acceptance criterion? (2) Are any requirements missing or only partially implemented? (3) Does the code handle the specified edge cases? Cite file names, line numbers, and the specific requirement item.

**Lens 2 — Code Review Excellence**
Apply the `code-review-excellence` skill criteria. Focus on correctness, security, error handling, naming, test quality, and consistency with the existing codebase style.

**Lens 3 — In-Function Optimization + Dead Code**
Important rules:
1. Do NOT suggest removing or renaming functions — a function may be required for interface consistency or called elsewhere even if it appears unused in this diff. Only flag internal logic issues, not the function's existence.
2. In-function optimizations: logic inside a function that can be made more efficient without changing external behavior — batching sequential external calls, replacing loops with bulk operations, eliminating redundant fetches, using more efficient data structures internally.
3. Dead branches: unreachable conditions, always-true/false guards, redundant else after return.
4. Duplication within a function: repeated logic inside the same function that can be collapsed.

Changes:
{changes}"

---

## Phase 7: Summary + Decision

Spawn a `documenter` subagent. Pass the reviewer JSON output.

Prompt: "Aggregate the three-lens review JSON into a **Review Summary** with two sections.

### Blocking Issues
Any finding with `severity: blocking`. An issue is blocking if it:
- Violates an acceptance criterion
- Introduces a security vulnerability
- Causes incorrect behavior or would cause test failures
- Significantly deviates from existing codebase patterns in a way that breaks consistency

### Suggestions & Optimizations
All `severity: suggestion` findings — optimization items, dead code, style/naming.

Then update Jira: post the summary as a comment on `$ARGUMENTS` via `mcp__atlassian__addCommentToJiraIssue`, and link any commits."

### Decision

**If zero blocking issues:**
Print the full Review Summary, then:
```
✓ Code review passed with no blocking issues.
Implementation of $ARGUMENTS is complete.
```
Stop.

**If one or more blocking issues exist:**
Print the full Review Summary, then:

> **Pausing for your approval before re-implementation.**
> The blocking issues above must be resolved. Reply "yes" to loop back to **Phase 3 Planning** with the blocking issues appended to the Requirements (cache invalidated and rewritten), or "no" to stop here and handle them manually.

If approved: append blocking issues to the cached Requirements (`valkey-cli -p 8888 APPEND` or re-`SET`), then restart from **Phase 3 Planning** so Opus can re-decompose with the new context. Do not re-run Phase 1 or Phase 2.

If denied: stop and summarize what was left unresolved.
