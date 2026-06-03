## Why

The current test suite tests generic knowledge tasks (geography, civics) rather than the agents and skills it's meant to evaluate — producing shallow signal about whether the AI agents under test actually perform their intended jobs. The tests need to be rewritten to be agent-specific, meaningful, and mockable where external calls would introduce flakiness.

## What Changes

- Remove or replace generic knowledge tests (capital of France, branches of government, sky is blue, Tokyo JSON) with tests that directly exercise the agents and skills being evaluated
- Add meaningful tests for each agent type: `researcher`, `team-lead`, `code-reviewer-eval`
- Add tests for key skills: `code-review-excellence`, `multi-discipline-review`, `brainstorming`
- Introduce mock/stub patterns for external calls (Firecrawl, shell, Valkey) so tests are deterministic and fast
- Align assertions to agent contracts: structured output shapes, verdict fields, delegation behavior, skill invocation patterns

## Capabilities

### New Capabilities
- `agent-test-coverage`: Dedicated test cases for each agent (`researcher`, `team-lead`, `code-reviewer-eval`) that verify agent-specific behaviors (tool use, delegation, verdict output)
- `skill-test-coverage`: Test cases that verify each skill produces correct outputs (code-review-excellence findings, brainstorming artifacts, multi-discipline review structure)
- `mock-external-calls`: Patterns and fixtures for mocking Firecrawl, shell, and Valkey calls so evals run without live dependencies

### Modified Capabilities
- None — no existing spec files to update

## Impact

- `promptfooconfig.yaml`: Replace/add test cases; all current generic tests are candidates for removal or replacement
- `providers/`: May need lightweight stub providers or fixture responses for mocked tests
- `agents/`: No changes to agent definitions; tests must work against existing agent contracts
- `skills/`: No changes to skill files; tests validate current skill behavior
