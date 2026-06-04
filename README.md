# AI Agents & Skills

A collection of AI agents and skills for software development workflows.

## Agents

Agents are defined in `agents/` as JSON configs with co-located prompt files. They run via `kiro-cli` or `claude` and can be composed into multi-agent pipelines.

| Agent | Model | Purpose |
|---|---|---|
| **team-lead** | Sonnet 4.6 | Orchestrator. Triages tasks: handles trivial ones directly, delegates complex work to builder/validator/documenter subagents. Does not write code itself. |
| **builder** | Sonnet 4.6 | Worker agent. Executes one scoped implementation task at a time. Writes and edits code, runs shell commands. No subagents. |
| **validator** | Opus 4.8 | Read-only verification. Checks that a completed task satisfies its acceptance criteria. Scores and issues PASS/FAIL. |
| **documenter** | Haiku 4.5 | Generates documentation after build/validate cycles. Read-only for implementation files. |
| **tester** | Sonnet 4.6 | Runs tests, linters, and security scans. Writes missing tests. Recommends merge only when all quality gates pass. |
| **code-reviewer** | Sonnet 4.6 | Read-only reviewer. Issues APPROVE/BLOCK verdicts with CWE-anchored findings grouped by severity. |
| **glide-code-reviewer** | Sonnet 4.6 | Specialized GLIDE reviewer. Subagent of code-reviewer for Valkey GLIDE client code. |
| **researcher** | Sonnet 4.6 | External research via Firecrawl. Returns concise findings with source URLs and a concrete recommendation. |
| **research-validator** | Sonnet 4.6 | Adversarially verifies researcher findings by cross-checking cited source URLs. |
| **research-summarizer** | Sonnet 4.6 | Orchestrates researcher and research-validator, then produces a final synthesis. |
| **superhuman** | Opus 4.8 | Expert engineer for complex multi-step tasks requiring deep reasoning across AWS, backend, UI/UX, and architecture. |
| **team-leader** | Sonnet 4.6 | Alternative orchestrator. Delegates to researcher/developer/tester/code-reviewer. |
| **developer** | - | General-purpose developer agent using the CAO MCP server. |

## Skills (Claude Code)

Skills are invokable via `/skill-name` in Claude Code. They live in `skills/` and are synced to `~/.kiro/skills/` via `make push`.

| Skill | Purpose |
|---|---|
| **review-code** | Self-review uncommitted or unpushed work. Multi-phase with parallel subagent reviewers and an Opus skeptic pass. |
| **review-pr** | Senior-grade PR review against the linked Jira ticket. Local-only output, never posts to GitHub. |
| **write-pr** | Generate a human-sounding PR description from the current git diff. Uses the repo's PR template. |
| **multi-discipline-review** | Parallel review across security, correctness, design, performance, and testing lenses. |
| **implement-jira** | Fetch a Jira ticket, plan with Opus, implement with Sonnet, run tests, then review. |
| **write-narrative** | Draft a humanized technical narrative from a Jira issue. |
| **write-pr-comments** | Post approved inline PR comments from an Obsidian review note. |
| **systematic-debugging** | Structured debugging workflow before proposing any fix. |
| **test-driven-development** | TDD workflow for features and bugfixes. |
| **brainstorming** | Explore intent, requirements, and design before implementation. |
| **glide-skill** | Production-ready patterns for Valkey GLIDE clients across 6 languages. |
| **humanizer** | Remove AI writing patterns from text. |

Full list: `ls skills/`

## Getting Started

```bash
npm install
```

Ensure the CLIs are available:

```bash
claude --version
kiro-cli --version
```

Sync skills and agents to your local `~/.kiro`:

```bash
make push   # copy from repo → ~/.kiro
make pull   # copy from ~/.kiro → repo
make status # diff between repo and ~/.kiro
```

## Evaluation Suite

Automated evaluation of agent and skill quality using [promptfoo](https://promptfoo.dev). One test per agent/skill; each test uses multiple assertions to validate distinct behaviors in a single LLM call.

### Running evals

```bash
./run-eval.sh          # run all tests, cache preserved between runs
./run-eval.sh --reset  # clear state and run (use when DB errors occur)
npm run eval:view      # open results in browser
```

### Test structure

Tests are in `promptfooconfig.yaml`, grouped by agent/skill. Each test uses a combination of:

- `javascript` assertions for deterministic checks (keyword presence, word counts, structural patterns)
- `llm-rubric` for subjective quality where keyword matching falls short
- `icontains` for exact substring requirements

Scoring is 80% quality / 20% cost (output length proxy). See `scoring.js`.

### Adding a test

```yaml
- description: 'agent-name — what behavior this checks'
  options:
    provider: 'exec: bash providers/your_provider.sh'
  vars:
    task: 'The prompt to send'
  assert:
    - type: javascript
      value: |
        const lower = output.toLowerCase();
        const passes = lower.includes('expected thing');
        return { pass: passes, score: passes ? 1 : 0, reason: `Found: ${passes}` };
      metric: accuracy
      weight: 3
```
