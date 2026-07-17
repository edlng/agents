---
name: implement-task
description: Single-command entry point that takes a plain-text task description and runs plan → advisor loop → implement → test → review using existing agents. Use when given a task description without a Jira ticket. Do NOT use for tasks that have a Jira ticket (use implement-jira), for quick one-file fixes, or when a plan already exists (use subagent-driven-development).
---

> **Codex runtime:** Use Codex-native agent dispatch, task plans, user-input requests, MCP capabilities, and skill loading. Resolve agents from `~/.codex/agents` or `.codex/agents`; resolve skills from `~/.agents/skills` or `.agents/skills`.
>
> Match work to catalog roles: low effort uses `context-curator`, `explore`, or `documenter`; medium uses `builder`, `code-reviewer`, `tester`, or `researcher`; high uses `validator` or `superhuman`.

# Implement Task

Implement `$ARGUMENTS` end-to-end. Follow each phase in order.

**Announce at start:** "Implementing task: $ARGUMENTS"

**Continuous execution:** Do not pause between phases. The only reasons to stop are: a blocking review finding that needs human input, or ambiguity that genuinely prevents progress.

---

## Phase 1: Codebase Scan

Read the existing codebase to understand what the new code must match and reuse. Follow `_shared/codebase-context-checklist.md` (language/runtime, code style, patterns, existing utilities, test conventions).

Produce a short **Codebase Context** note. Hold it in working memory for downstream phases.

---

## Phase 2: Planning

Spawn a `superhuman` subagent.

Prompt: "You are the planner. **Effort budget: 5-10 tool calls to read context, then produce the plan.** Task: {$ARGUMENTS}. Codebase context: {codebase context note}.

Decompose the work into an ordered subtask list. For each subtask, output:
- `id`: short identifier (e.g. `s1`, `s2`)
- `description`: one line
- `files`: list of files this subtask touches
- `complexity`: `low` or `medium`
- `depends_on`: list of subtask ids that must complete first

Tag a subtask `low` only if it is mechanical (boilerplate, config, simple CRUD, format conversion) AND touches files disjoint from any `medium` subtask. Otherwise tag `medium`. Output strict JSON."

---

## Phase 3: Research Gate

Review the plan. For each subtask, check whether it references an external API, library, or system not already present in the codebase (compare against Phase 1 context).

**If unfamiliar tech is detected:** spawn a `researcher` subagent:

"Research the following technologies for use in this implementation. For each, find: official documentation URL, correct installation/import, key API surface relevant to our use case, and any gotchas or version constraints.

Technologies: {list}
Use case context: {relevant subtask descriptions}

Return findings in standard format (URL, Summary, Tradeoffs, Recommendation)."

Hold research findings for injection into implementer context.

**If all tech is already in the codebase:** skip.

---

## Phase 4: Context Enrichment

Spawn a `context-curator` subagent:

"A builder agent is about to implement: {$ARGUMENTS}. Subtasks touch: {comma-separated file list from plan}. Curate relevant memories."

Hold the returned `<context-memory>` block for injection into downstream subagents. If empty, proceed without it.

---

## Phase 5: Approach Advisor Loop (medium-complexity subtasks only)

**Why:** Catching a wrong approach before code exists is cheaper than catching it in review after implementation.

**Scope:** Only subtasks tagged `complexity == medium`. Skip entirely for `low`-complexity subtasks.

For each `medium` subtask, in dependency order:

### 5.1 Draft

Spawn a `builder` subagent:

"Subtask: {subtask description}. Files: {subtask files}. Codebase context: {Phase 1 note}. Context memory: {Phase 4 block, if any}.

**Do not write code.** Produce a short approach note (under 200 words): (1) the concrete change you'll make, (2) which existing utilities/patterns you'll reuse, (3) any interface or data-shape decisions, (4) risks or ambiguities."

### 5.2 Advise

Spawn a `validator` subagent:

"Task requirements: {$ARGUMENTS}. Codebase context: {Phase 1 note}. Proposed approach for subtask '{subtask id}': {approach note from 5.1}.

This is a proposed approach, not finished code. Check only:
1. Does the approach satisfy this subtask's slice of the task requirements?
2. Does it fit existing codebase patterns rather than inventing new ones?
3. Is there a simpler way that avoids unnecessary abstraction?

Respond with exactly one of:
- `APPROVED` - approach is sound, proceed.
- `REVISE: <specific, actionable feedback>` - state exactly what to change.

Do not nitpick style choices that don't affect correctness or fit."

### 5.3 Iterate

If `REVISE`: re-dispatch the `builder` subagent with the feedback appended, ask for a revised approach note (not code), return to 5.2. Cap at **3 rounds**. If still not approved after 3, log the disagreement, proceed with the latest approach.

If `APPROVED`: store the approach note for use in Phase 6.

---

## Phase 6: Implementation

Spawn a `builder` subagent as the implementor.

Prompt: "You are the implementor. **Effort budget: 30-60 tool calls total across all subtasks.** Task: {$ARGUMENTS}. Codebase context: {Phase 1 note}. Context memory: {Phase 4 block, if any}. Research findings: {Phase 3 findings, if any}. Plan: {Phase 2 JSON}.

For each subtask in the plan, in dependency order:
- If `complexity == medium`: implement it yourself, following the approved approach note for this subtask (below) rather than re-deriving the design.
- If `complexity == low` AND its `files` do not overlap any subtask currently in flight: spawn another `builder` subagent to handle it, passing only the subtask description, relevant files, and Codebase Context.
- Otherwise: handle it yourself.

**Approved approaches (medium subtasks):**
{for each medium subtask: id + approach note from Phase 5}

Match the existing codebase exactly: same language, code style, naming conventions, error-handling patterns, logging approach. Reuse existing utilities - do not re-implement what already exists. Write minimal code; do not add abstractions beyond what the task requires.

**Decisions log:** After completing each subtask, append a brief entry to `.decisions.md`: what was decided, why, which files were affected."

---

## Phase 7: Tests

Spawn a `tester` subagent.

Prompt: "Task requirements: {$ARGUMENTS}. Codebase context: {Phase 1 note}.

**Effort budget: 20-40 tool calls. Prioritize writing and running tests over exhaustive scanning.**

### 7.1 Discover existing test patterns

Scan for test files, frameworks, and conventions. Read 2-3 existing tests to internalize the style.

### 7.2 Unit tests

Write unit tests for every non-trivial function/method introduced. Derive test cases from the task requirements and edge cases. Each test must assert a concrete, meaningful outcome.

### 7.3 Integration tests

If the codebase has integration tests, follow that pattern. If the task involves I/O (persistence, network, external services), write at least one integration test. If purely logic with no I/O, skip.

### 7.4 Run the test suite

Detect the test runner from project config. Run and capture output.

### Fix loop (autonomous)
If tests fail:
- Analyze the failure
- If the failure suggests the plan was wrong (missing subtask, missed dependency): report back with BLOCKED and what needs re-planning.
- Otherwise (implementation or test bug): fix and re-run.
- Repeat until all tests pass.

A test that passes only because an assertion was weakened is NOT a fix."

**If tester reports BLOCKED (plan was wrong):** return to Phase 2 with the failure context appended to the task description. Re-run Phases 5-7 for affected subtasks.

---

## Phase 8: Code Review (merged three-lens)

Run `git diff` to produce a Changes snapshot.

Spawn a `code-reviewer` subagent:

"Task requirements: {$ARGUMENTS}. Review the following Changes through three lenses in one pass. Output strict JSON with three top-level keys: `requirements_alignment`, `code_quality`, `optimization`. Each value is a list of findings; each finding has `file`, `line_range`, `severity` (`blocking` or `suggestion`), and `description`.

**Lens 1 - Requirements Alignment**
Does the implementation satisfy the task requirements? Are any pieces missing or only partially implemented? Does the code handle edge cases?

**Lens 2 - Code Quality**
Apply code-review-excellence criteria. Focus on correctness, security, error handling, naming, test quality, consistency with existing codebase style.

**Lens 3 - In-Function Optimization + Dead Code**
Rules: (1) Do NOT suggest removing or renaming functions - they may be required for interface consistency. (2) Flag: in-function inefficiencies, unreachable branches, duplication within a function.

Changes:
{diff output}"

---

## Phase 9: Decision

**If zero blocking issues:**
Print the review summary, then:
```
✓ Code review passed. Implementation of task complete.
```
Stop.

**If blocking issues exist:**
Print the review summary, then fix them:
- Spawn a `builder` subagent with the blocking issues and affected files.
- After fixes, re-run Phase 7 (tests) and Phase 8 (review).
- Cap at **2 fix cycles**. If still blocking after 2, print the remaining issues and stop for human input.
