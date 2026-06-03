## ADDED Requirements

### Requirement: review-code skill tests verify it catches real bugs and avoids false positives
The test suite SHALL include at least 2 tests for the `review-code` skill that verify multi-lens analysis: one test on code with a real bug (correctness or security), one test on clean code confirming no false MUST_FIX findings.

#### Scenario: review-code identifies a real correctness or security bug
- **WHEN** an agent with `review-code` loaded reviews a snippet with a known bug (e.g. off-by-one, SQL injection)
- **THEN** the output SHALL score 4+ on a 1–5 rubric: 1 = no finding; 2 = vague finding without evidence; 3 = finding identified but no fix; 4 = finding with evidence and fix; 5 = finding, evidence, fix, and CWE or severity level

#### Scenario: review-code does not hallucinate findings on clean code
- **WHEN** an agent with `review-code` loaded reviews a well-written function with proper validation and error handling
- **THEN** the output SHALL NOT contain `BLOCK`, `MUST_FIX`, or `critical` findings, and SHALL score 4+ on a 1–5 rubric that rewards: 1 = false critical finding; 2 = false high finding; 3 = approved but no acknowledgment; 4 = approved with note on quality; 5 = accurately identifies clean code and states only nits or nothing

### Requirement: review-pr skill tests verify it fetches PR context and structures findings
The test suite SHALL include at least 2 tests for the `review-pr` skill that verify it reads a PR diff and produces structured findings grouped by severity — without posting to GitHub.

#### Scenario: review-pr produces blocking and suggestion findings for a PR with issues
- **WHEN** an agent with `review-pr` loaded reviews a PR reference containing known bugs
- **THEN** the output SHALL contain a `## Blocking` section with at least one finding, each with file, line range, evidence, and fix

#### Scenario: review-pr output contains no GitHub write operations
- **WHEN** the review-pr skill completes a review
- **THEN** the output SHALL NOT contain `gh pr review`, `gh pr comment`, or any indication that a review was posted to GitHub — findings SHALL be local only

### Requirement: write-pr skill tests verify it produces accurate, human-sounding descriptions
The test suite SHALL include at least 2 tests for the `write-pr` skill that verify it reads a diff, fills in a PR template accurately, and produces output that does not sound AI-generated.

#### Scenario: write-pr fills all required template sections from the diff
- **WHEN** an agent with `write-pr` loaded runs against a diff with a clear intent
- **THEN** the output SHALL contain non-empty Summary, Changes, and Tests sections, and the Changes section SHALL reference at least one actual file name from the diff

#### Scenario: write-pr output avoids AI writing markers
- **WHEN** write-pr produces a PR description
- **THEN** the output SHALL NOT contain words in the AI vocabulary list (`streamlines`, `robust`, `enhance`, `leverage`, `crucial`, `pivotal`) and SHALL score 4+ on a 1–5 humanizer rubric: 1 = clearly AI-generated (multiple markers); 2 = 1–2 markers present; 3 = neutral but stiff; 4 = natural with minor stiffness; 5 = reads like a developer wrote it

### Requirement: multi-discipline-review skill tests verify parallel reviewer output covers 3+ disciplines
The test suite SHALL include at least 1 test for the `multi-discipline-review` skill that verifies it produces findings across distinct disciplines for a snippet with issues in multiple areas.

#### Scenario: multi-discipline-review covers at least 3 disciplines on a complex snippet
- **WHEN** an agent with `multi-discipline-review` loaded reviews code with security, correctness, and design issues
- **THEN** the output SHALL include findings from at least 3 of: security, correctness, design, performance, testing — verified by presence of discipline labels or section headers in the output
