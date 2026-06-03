## Context

The project uses `promptfoo` to evaluate AI agents and skills via `exec:` providers that shell out to `kiro-cli`. Currently most tests assess generic LLM knowledge (geography, civics, JSON formatting) rather than the behaviors of the actual agents under evaluation. This creates noise: a test that checks whether an LLM knows the capital of France says nothing about whether the `researcher` agent correctly invokes Firecrawl or the `team-lead` correctly delegates to subagents.

External calls (Firecrawl, shell, Valkey) are live in production providers, making tests slow and non-deterministic. Tests need to run fast in CI without network dependencies.

## Goals / Non-Goals

**Goals:**
- Replace generic knowledge tests with agent-contract tests that verify observable behaviors (delegation, verdict structure, tool invocation patterns)
- Add skill-specific tests for `code-review-excellence`, `multi-discipline-review`, and `brainstorming`
- Provide mock/stub fixtures so Firecrawl and Valkey calls can be intercepted without real network/service
- Ensure all new tests are deterministic and complete in < 30s per assertion
- Find a **proper** system for rubrics. Perhaps a ranking of scale 1-5 (1 being very poor and 5 being excellent). Ensure it makes sense.
- Ensure we evaluate cost properly (perhaps counting tokens usage and latency and etc)

**Non-Goals:**
- Changing agent definitions or skill files (tests validate current contracts, not new ones)
- Full integration test coverage of all tools — focus on the specific behaviors defined in each agent's prompt
- UI or browser-based testing

## Decisions

### 1. Mock external calls via stub provider scripts, not test doubles inside agents

**Decision**: Create lightweight stub shell scripts under `providers/stubs/` that return fixture JSON. Tests that use `researcher` for external-call paths point to `providers/stubs/firecrawl_stub.sh` instead of the real Firecrawl provider.

**Rationale**: Agent `.json` files reference real MCP servers. Intercepting at the provider/shell level is the only seam available without modifying agent definitions. Stub scripts are trivially auditable.

**Alternative considered**: Patching the MCP server config per-test — rejected because it requires environment mutation between test runs and introduces ordering dependencies.

### 2. Assert on structured agent output contracts, not free-text

**Decision**: Code-reviewer-eval tests assert on fields like `verdict`, `findings[].severity`, `findings[].cwe` (exact strings or regex). Team-lead tests assert on delegation signals (`contains "delegating"` vs absence of delegation for trivial tasks).

**Rationale**: Free-text `llm-rubric` assertions are expensive and slow. Field-level assertions are cheap, fast, and precisely encode the agent contract. `llm-rubric` is reserved for cases where the semantics genuinely require a judge (e.g., "is this explanation accurate?").

**Alternative considered**: All `llm-rubric` — rejected for cost and flakiness reasons.

### 3. One spec file per capability from the proposal

**Decision**: Three spec files: `agent-test-coverage`, `skill-test-coverage`, `mock-external-calls`. Each maps directly to a capability named in the proposal.

**Rationale**: Keeps each spec focused and makes the task breakdown mechanical — one spec file = one clear domain of requirements.

### 4. Keep existing code-review tests that already test agent behavior

**Decision**: Tests 6–9 (SQL injection, off-by-one, clean code, multi-discipline) and Tests 11–13 (team-lead and code-reviewer-eval) already test agent behaviors well. Keep these; the rewrite targets only the generic knowledge tests (1–5, 10).

**Rationale**: Removing working agent-behavior tests regresses coverage. The problem is the first five tests plus Test 10, not the full suite.

## Risks / Trade-offs

- **Stub scripts may drift from real provider behavior** → Mitigation: document the fixture format inline in the stub; add a comment linking to the real MCP response schema.
- **Agent prompts may change, breaking field assertions** → Mitigation: pin assertions to the contract documented in each agent's `.json` description; update tests when agent definitions change.
- **Replacing Tests 1–5 removes the only cost-proxy calibration data** → Mitigation: keep one cheap/fast test (the "say hello in French" type) explicitly labeled as a cost-calibration baseline.

## Open Questions

- Should the skill tests invoke agents with the skill pre-loaded (via `resources`) or test the skill output directly via a thin wrapper? TBD during implementation — depends on whether `kiro-cli` supports inline skill injection per test.
