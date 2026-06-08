# Shared: Review Findings Schema & Lenses

> Shared reference used by `review-pr`, `review-cookbook-pr`, and `review-code`. Not a standalone skill. Single source of truth for the findings JSON schema, the review lenses, and the filing rules. Consuming skills may add skill-specific lenses or severity vocabularies on top of this base.

## Findings JSON schema

Output a flat JSON array of findings. Each finding has:

- `id`: short slug, e.g. `auth-missing-rate-limit`
- `lens`: see lenses below (consuming skill may add more)
- `file`: path
- `line_range`: e.g. `42-58`
- `severity`: `blocking` | `suggestion` | `nit` (consuming skill may override the vocabulary — e.g. local review uses `must_fix` | `should_fix` | `consider`)
- `claim`: one sentence — what is wrong
- `evidence`: one or two literal lines from the diff or codebase context that prove the claim. **If you cannot quote evidence, do not file the finding.**
- `suggested_fix`: concrete change

## The four core lenses

Apply all lenses in a single pass — load the diff once, not once per lens.

### Lens 1 — Requirements alignment
Does the implementation satisfy every acceptance criterion? Cite the requirement item by quoting it. Flag missing or partial coverage. Flag scope creep (changes unrelated to any requirement). Skip this lens if no requirements source exists.

### Lens 2 — Correctness & Security
Logic bugs, off-by-ones, race conditions, unhandled errors, dropped exceptions, broken invariants. Security: injection (SQL, command, template), authn/authz gaps, secrets in code, unsafe deserialization, SSRF, path traversal, weak crypto, missing input validation at trust boundaries. Be concrete about the threat model — generic "add validation" findings are rejected by the validator.

### Lens 3 — Design fit
Does the code match the codebase context? Reused existing utilities? Premature abstraction? Duplication of nearby code? Naming and error-handling consistent? Layering respected (e.g. data access not bleeding into HTTP handlers)? Only flag what conflicts with patterns visible in the codebase context — not matters of taste.

### Lens 4 — Testability
Do tests cover every acceptance criterion and the edge cases listed in requirements? Are tests asserting behavior or just exercising code paths? Mocked dependencies that should be real (e.g. mocked DB when an integration test would catch the bug)? Missing tests for new failure paths?

## Filing rules

- Only flag what is in the diff, except where context is essential to prove a diff issue (e.g. a caller breaks because of a signature change in the diff).
- Cite a real symbol, file, and line. Do NOT invent function names or files.
- Severity rubric (default vocabulary):
  - `blocking`: violates a requirement, introduces a real security/correctness bug, breaks an interface, or causes test failures.
  - `suggestion`: real improvement (perf, clarity, robustness) that does not block merge.
  - `nit`: pure style/naming.
- If you are not sure something is wrong, omit it. The validator pass will not rescue a weak finding — it will downgrade or reject it.
