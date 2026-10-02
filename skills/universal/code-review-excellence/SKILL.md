---
name: code-review-excellence
description: Reasoning framework for reviewing a diff or PR. Defines the review lenses (design fit, correctness and security, requirements, testability), severity labels (important, nit, pre-existing), a self-challenge check for each finding, and a question style for uncertain findings. Reports only findings that affect correctness or stated requirements and excludes style. Use when a reviewer agent or review skill analyzes a diff or PR and needs a frame for what to flag, how to grade it, and when to drop it.
---

# Code review excellence

A frame for deciding what to report in a code review and how to grade it. It applies to any diff, PR, or uncommitted change.

## Scope

Report only findings that affect correctness or stated requirements. Do not report style unless the change violates a pattern visible in the surrounding codebase. If the change is sound, say so explicitly and report no findings. Do not add findings to look thorough.

## Lenses

Load the diff once and apply the lenses in this order, matching `../_shared/review-findings-schema.md`.

1. **Design fit (primary).** Does the change look like it belongs in this codebase? Flag code that reimplements a utility that already exists nearby, breaks a naming, error-handling, or logging convention, crosses a layer boundary, or duplicates adjacent code. Flag only what conflicts with patterns you can point to in the codebase. A change that does not fit is a defect even when its logic is right.
2. **Correctness and security.** Trace the changed code with concrete inputs. Look for wrong logic, unhandled failure paths, races, data loss, and inputs that reach queries, commands, paths, or output without validation. Check authorization and secrets handling on any new entry point.
3. **Requirements.** Does the change satisfy each acceptance criterion? Quote the criterion. Flag missing or partial coverage and changes unrelated to any criterion. Skip this lens when no requirements source exists, and say it was skipped.
4. **Testability.** Does the change have tests that fail if the behavior breaks? Flag new behavior or a fixed bug with no test, and tests that pass without exercising the change.

## Severity

- `important`: a bug or stated-requirement gap that should be fixed before merging. Examples are a violated requirement, a real correctness or security bug, a broken interface, or missing tests for new behavior.
- `nit`: a minor issue worth fixing, not blocking. Include it only when it conflicts with a visible codebase pattern.
- `pre-existing`: a real bug in code the change did not introduce. Report it separately. It never affects the verdict.

## Self-challenge

Before reporting a finding, check it against the evidence. Drop the finding if any check fails.

1. Read the actual code at the cited lines, not the diff hunk alone. Confirm the code does what the finding says.
2. Identify an input or call path that reaches the problem. If callers, validation, or types make it unreachable, drop it.
3. Search the existing tests and the surrounding code for something that already covers or prevents it.
4. Confirm the cited line and file exist and the quoted evidence is verbatim.
5. Confirm the severity matches the impact. Downgrade when the impact is smaller than first judged.

## Question approach

When a finding is plausible but you cannot confirm it from the code, write it as a question that names the condition: "What happens if `items` is empty here? `items[0]` is read on line 42 with no length check." State what you checked and what you could not see. Do not assert a bug you have not traced.
