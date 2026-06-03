## ADDED Requirements

### Requirement: Firecrawl stub enables researcher tests to run without network
The test suite SHALL include a stub shell script at `providers/stubs/firecrawl_stub.sh` that returns a deterministic fixture response matching the real Firecrawl search response shape. Researcher agent tests SHALL be configurable to use this stub via an environment variable or alternate provider script so they run without live network access.

#### Scenario: Firecrawl stub returns a valid fixture for any query
- **WHEN** `providers/stubs/firecrawl_stub.sh` is called with any query string
- **THEN** it SHALL exit 0 and output JSON with at least one `results` entry containing `url` and `content` fields

#### Scenario: Researcher test using the stub completes in under 10 seconds
- **WHEN** a researcher test is configured to use the stub provider
- **THEN** the test SHALL pass the `latency` gate (< 10000ms) without any live Firecrawl calls

### Requirement: review-pr and review-code tests mock Valkey cache calls
`review-pr` and `review-code` use Valkey at `localhost:8888` for caching. Tests for these skills SHALL NOT depend on a live Valkey instance. The test environment SHALL either: (a) run a real Valkey instance as part of test setup, or (b) use a mock/stub provider script that inlines cache content so the agent does not need to issue `valkey-cli` calls.

#### Scenario: review-pr test runs without live Valkey
- **WHEN** a review-pr skill test runs
- **THEN** the test SHALL complete successfully regardless of whether Valkey is running at localhost:8888 — either by having a local Valkey started in test setup, or by using a stub provider that bypasses cache reads

#### Scenario: review-code test runs without live Valkey
- **WHEN** a review-code skill test runs
- **THEN** the test SHALL complete successfully without requiring a live Valkey instance at localhost:8888

### Requirement: Generic knowledge tests are removed
The test suite SHALL NOT contain test cases whose primary purpose is to assess generic LLM factual knowledge (capitals of countries, branches of government, scientific explanations) because these test the base model rather than agent or skill behavior.

#### Scenario: No generic knowledge tests remain in promptfooconfig.yaml
- **WHEN** the rewrite is complete
- **THEN** `promptfooconfig.yaml` SHALL NOT contain test descriptions matching: "capital of France", "branches of US government", "why is the sky blue", "Tokyo JSON", or "async/await vs Promises" as standalone tests unconnected to a specific agent or skill

### Requirement: One cost-calibration baseline test is retained
At least one test SHALL be explicitly labeled as a cost-calibration baseline — a task that should produce a very short response — so the `cost` metric has a meaningful lower bound to compare against.

#### Scenario: Cost baseline test expects a response under 50 words
- **WHEN** the cost baseline test runs against any provider
- **THEN** a `javascript` cost assertion SHALL award a score of 1.0 for responses under 50 words, decaying toward 0 as word count grows — providing a stable reference point for the `cost` metric across providers
