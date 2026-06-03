## ADDED Requirements

### Requirement: Grading uses a 1–5 scale with defined criteria
All LLM-graded assertions (`llm-rubric`) SHALL use a 1–5 scoring scale instead of a 0–1 threshold, with each score value explicitly defined. The grader prompt SHALL specify: 1 = completely wrong or missing, 2 = partial with major gaps, 3 = acceptable but incomplete, 4 = good with minor issues, 5 = excellent. Scores SHALL be normalized to 0–1 before being used as promptfoo metric values (divide by 5). A test passes when the normalized score meets the configured threshold.

#### Scenario: Rubric prompt defines 1–5 criteria
- **WHEN** a test uses `llm-rubric`
- **THEN** the `value` field SHALL include descriptions for each score 1–5 and SHALL NOT use open-ended "score based on" language without criteria

#### Scenario: Normalized score feeds into promptfoo pass/fail
- **WHEN** the LLM returns a score of 1–5
- **THEN** the assertion normalizes it (score / 5) and uses the resulting 0–1 value for threshold comparison

### Requirement: Cost metric captures token usage, not just word count
Each test that measures cost SHALL track at minimum two signals: (a) a `latency` gate (hard fail above 30s), and (b) a `javascript` assertion that penalizes verbosity via word count as a proxy. Tests for agents that emit structured output SHALL also assert that the output is not padded with unnecessary preamble or explanation.

#### Scenario: Latency gate fails tests that exceed 30s
- **WHEN** any test runs
- **THEN** a `latency` assertion with threshold 30000 SHALL be present as a hard gate (pass/fail only, not a scored metric)

#### Scenario: Word count proxy penalizes unnecessarily verbose output
- **WHEN** a test measures cost
- **THEN** a `javascript` assertion SHALL compute a cost score that decreases as word count grows past a task-appropriate ceiling (e.g. 50 words for a one-liner task, 300 for a technical explanation)

### Requirement: Researcher agent tests verify sourced, relevant output
The test suite SHALL include at least 2 tests for the `researcher` agent that verify it produces answers grounded in sourced content with a concrete recommendation, not generic summaries.

#### Scenario: Researcher answers a technical library question with a source
- **WHEN** the researcher agent receives a question about a specific library or API (e.g. "how do I do X with library Y")
- **THEN** the output SHALL score 4 or higher on a 1–5 rubric that awards: 1 = no answer; 2 = vague/generic; 3 = correct but no source; 4 = correct with source or citation; 5 = correct, sourced, with concrete recommendation

#### Scenario: Researcher stays concise for simple lookups
- **WHEN** the researcher receives a question with a one-sentence answer
- **THEN** the output SHALL be under 100 words and SHALL contain the correct answer (verified via `icontains`)

### Requirement: Team-lead tests verify direct handling vs delegation decisions
The test suite SHALL include at least 3 tests for the `team-lead` agent that verify it: handles trivial tasks inline without spawning subagents, delegates complex multi-step tasks, and uses the correct client library (`valkey-glide`) for Valkey tasks.

#### Scenario: Team-lead handles a one-line rename directly
- **WHEN** the team-lead receives a task to rename a single variable in a provided snippet
- **THEN** the output SHALL contain the corrected code and SHALL NOT contain delegation keywords (`delegating to builder`, `subagent`, `worktree`)

#### Scenario: Team-lead uses valkey-glide for Valkey tasks
- **WHEN** the team-lead receives a Valkey implementation task
- **THEN** the output SHALL reference `valkey-glide` or `@valkey/valkey-glide` and SHALL NOT reference `ioredis`, `iovalkey`, or `node-redis`

#### Scenario: Team-lead uses pipeline or batch for bulk Valkey writes
- **WHEN** the team-lead produces code that writes multiple keys to Valkey
- **THEN** the code SHALL use pipelining or batching (presence of `pipeline` or `batch` or `exec`) rather than individual awaited `set()` calls
