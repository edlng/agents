---
name: multi-discipline-review
description: |
  Multi-discipline code review using parallel sub-agents. Each sub-agent reviews a different discipline (security, correctness, design, performance, testing), applies self-challenge rubrics to validate its own findings, then the orchestrator consolidates and deduplicates. Use when reviewing code changes, PRs, or diffs.
---

# Multi-Discipline Code Review Agent

Review code by spawning parallel sub-agents — each focused on a single discipline — then consolidating their findings. Each sub-agent applies a self-challenge rubric before reporting, reducing false positives.

`$ARGUMENTS`: a diff, file path, branch name, or PR reference to review.

---

## Phase 1: Gather Context

Read the code to review. Determine:
- The diff or file contents
- Language, framework, existing patterns
- Any linked requirements (Jira ticket, PR description)

---

## Phase 2: Spawn Discipline Sub-Agents

Spawn the following sub-agents **in parallel** with model `claude-sonnet-4-6`. Each sub-agent receives the same diff/context and reviews ONLY its discipline.

### Sub-Agent 1: Security Reviewer

Prompt:
> "Review this code ONLY for security issues. Look for:
> - Injection vulnerabilities (SQL, XSS, command, path traversal)
> - Authentication/authorization gaps
> - Secrets or credentials in code
> - Unsafe deserialization
> - Missing input validation at trust boundaries
>
> For each finding, provide: file, line range, severity (critical/high/medium/low), description, and suggested fix.
>
> **RUBRIC — answer these before reporting each finding:**
> 1. Did you confirm the vulnerable code path is actually reachable?
> 2. Is there existing sanitization/validation you missed?
> 3. Can you name a specific attack vector, not just a theoretical risk?
>
> If you cannot answer YES to all three, downgrade or drop the finding."

### Sub-Agent 2: Correctness Reviewer

Prompt:
> "Review this code ONLY for correctness bugs. Look for:
> - Off-by-one errors
> - Null/undefined dereferences
> - Race conditions
> - Wrong error handling (swallowed errors, wrong error type)
> - Broken invariants or logic errors
> - Edge cases (empty collections, boundary values, overflow)
>
> For each finding, provide: file, line range, severity (critical/high/medium/low), description, and suggested fix.
>
> **RUBRIC — answer these before reporting each finding:**
> 1. Can you construct a specific input that triggers this bug?
> 2. Did you verify the code doesn't already handle this case elsewhere?
> 3. Is this a real bug or just a style preference?
>
> If you cannot answer YES to all three, downgrade or drop the finding."

### Sub-Agent 3: Design & Maintainability Reviewer

Prompt:
> "Review this code ONLY for design and maintainability. Look for:
> - Reimplementation of existing utilities in the codebase
> - Violations of the codebase's established patterns
> - Premature abstraction or unnecessary complexity
> - Poor naming or unclear intent
> - Missing or misleading documentation on non-obvious logic
> - Layering violations
>
> For each finding, provide: file, line range, severity (high/medium/low), description, and suggested fix.
>
> **RUBRIC — answer these before reporting each finding:**
> 1. Did you check whether the pattern you're suggesting actually exists in this codebase?
> 2. Is this a genuine maintainability concern or just your personal preference?
> 3. Would a new team member be confused by this code?
>
> If you cannot answer YES to all three, downgrade or drop the finding."

### Sub-Agent 4: Performance Reviewer

Prompt:
> "Review this code ONLY for performance issues. Look for:
> - N+1 queries or unnecessary database round-trips
> - Unbounded loops or allocations
> - Missing pagination on large datasets
> - Blocking I/O in hot paths
> - Unnecessary re-computation (missing memoization/caching)
> - Memory leaks (unclosed resources, growing collections)
>
> For each finding, provide: file, line range, severity (high/medium/low), description, and suggested fix.
>
> **RUBRIC — answer these before reporting each finding:**
> 1. Is this code actually in a hot path, or is it called rarely?
> 2. Can you estimate the actual impact (e.g., O(n²) with realistic n)?
> 3. Did you confirm there isn't already caching/batching handling this?
>
> If you cannot answer YES to all three, downgrade or drop the finding."

### Sub-Agent 5: Testing Reviewer

Prompt:
> "Review this code ONLY for test quality and coverage gaps. Look for:
> - New behavior without corresponding tests
> - Error/failure paths not tested
> - Tests that assert nothing meaningful
> - Mocks where real fixtures would catch bugs
> - Flaky test patterns (timing, ordering, shared state)
> - Missing edge case coverage
>
> For each finding, provide: file, line range, severity (high/medium/low), description, and suggested test.
>
> **RUBRIC — answer these before reporting each finding:**
> 1. Is the untested code actually new in this diff (not pre-existing)?
> 2. Would the test you're suggesting actually catch a real bug?
> 3. Is there an existing test that already covers this case?
>
> If you cannot answer YES to all three, downgrade or drop the finding."

---

## Phase 3: Consolidate

Once all sub-agents return, the orchestrator:

1. **Deduplicates**: If multiple sub-agents flagged the same line/issue, merge into one finding with the highest severity.
2. **Ranks**: Order by severity (critical → high → medium → low).
3. **Summarizes**: Group findings by discipline.
4. **Provides a verdict**:
   - 🔴 **Block** — critical/high issues that must be fixed
   - 🟡 **Approve with comments** — medium issues worth addressing
   - 🟢 **Approve** — only low/nit-level findings

---

## Output Format

```markdown
# Code Review: <target>

## Verdict: 🔴/🟡/🟢 <Block/Approve with comments/Approve>

## Critical & High (N)
| # | Discipline | File:Line | Issue | Fix |
|---|-----------|-----------|-------|-----|
| 1 | security  | src/auth.ts:42 | SQL injection via unsanitized input | Use parameterized query |

## Medium (N)
<same table format>

## Low / Nits (N)
<same table format>

## Summary
<1-2 paragraph overall assessment>
```

---

## Design Rationale

- **Parallel sub-agents**: Each discipline gets full attention without context dilution. A single-pass reviewer tends to fixate on one category.
- **Self-challenge rubrics**: Forces each sub-agent to validate its own claims before reporting. Dramatically reduces false positives and "generic advice" findings.
- **No model pinning**: Let the system choose models for sub-agents. This allows experimentation with model swapping without changing the skill.
- **Consolidation**: The orchestrator deduplicates and ranks, producing a clean final report rather than 5 separate dumps.
