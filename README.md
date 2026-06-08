# AI Agents & Skills

A collection of AI agents and skills for software development workflows.

## Agents

Agents are defined in `agents/` as JSON configs with co-located prompt files. They run via `kiro-cli` or `claude` and can be composed into multi-agent pipelines.

### Orchestration

| Agent | Model | Purpose |
|---|---|---|
| **team-lead** | Sonnet 4.6 | Orchestrator. Triages tasks: handles trivial ones directly, delegates complex work to builder/validator/documenter subagents. Does not write code itself. |
| **team-leader** | Sonnet 4.6 | Alternative orchestrator. Delegates to researcher/developer/tester/code-reviewer. |

### Implementation

| Agent | Model | Purpose |
|---|---|---|
| **builder** | Sonnet 4.6 | Worker agent. Executes one scoped implementation task at a time. Writes and edits code, runs shell commands. No subagents. |
| **developer** | - | General-purpose developer agent using the CAO MCP server. |
| **superhuman** | Opus 4.8 | Expert engineer for complex multi-step tasks requiring deep reasoning across AWS, backend, UI/UX, and architecture. |

### Quality Assurance

| Agent | Model | Purpose |
|---|---|---|
| **tester** | Sonnet 4.6 | Runs tests, linters, and security scans. Writes missing tests. Recommends merge only when all quality gates pass. |
| **validator** | Opus 4.8 | Read-only verification. Checks that a completed task satisfies its acceptance criteria. Scores and issues PASS/FAIL. |
| **code-reviewer** | Sonnet 4.6 | Read-only reviewer. Issues APPROVE/BLOCK verdicts with evidence-backed findings grouped by severity. |
| **glide-code-reviewer** | Sonnet 4.6 | Specialized GLIDE reviewer. Subagent of code-reviewer for Valkey GLIDE client code. |

### Research

| Agent | Model | Purpose |
|---|---|---|
| **researcher** | Sonnet 4.6 | External research via Firecrawl. Returns concise findings with source URLs and a concrete recommendation. |
| **research-validator** | Sonnet 4.6 | Adversarially verifies researcher findings by cross-checking cited source URLs. |
| **research-summarizer** | Sonnet 4.6 | Orchestrates researcher and research-validator, then produces a final synthesis. |

### Documentation

| Agent | Model | Purpose |
|---|---|---|
| **documenter** | Haiku 4.5 | Generates documentation after build/validate cycles. Read-only for implementation files. |

### Valkey & GLIDE

| Agent | Model | Purpose |
|---|---|---|
| **valkey-glide-implementor** | Sonnet 4.6 | Generates production-ready Valkey GLIDE code snippets across 6 languages for client setup, vector search, batch operations, caching, and session management. |

## Skills

Skills are invokable via `/skill-name` in Claude Code or Kiro CLI. They live in `skills/` and are synced to `~/.kiro/skills/` via `make push`. Project-scoped skills live in `.kiro/skills/`.

### Code Review

| Skill | Purpose |
|---|---|
| **review-code** | Self-review uncommitted or unpushed work. Multi-phase with parallel subagent reviewers and an Opus skeptic pass. |
| **review-pr** | Senior-grade PR review against the linked Jira ticket. Local-only output, never posts to GitHub. |
| **review-cookbook-pr** | PR review for cookbook PRs. Adds checks for direct function calls and Docker tag verification. |
| **multi-discipline-review** | Parallel review across security, correctness, design, performance, and testing lenses. |
| **code-review-excellence** | Reasoning framework for systematic code review. Lenses, self-challenge rubrics, and severity differentiation. |
| **requesting-code-review** | Workflow for verifying work meets requirements before requesting review. |
| **receiving-code-review** | Workflow for handling review feedback with technical rigor before implementing suggestions. |

### Writing & Documentation

| Skill | Purpose |
|---|---|
| **write-pr** | Generate a human-sounding PR description from the current git diff. Uses the repo's PR template. |
| **write-pr-comments** | Post approved inline PR comments from an Obsidian review note. |
| **write-narrative** | Draft a humanized technical narrative from a Jira issue. |
| **humanizer** | Remove AI writing patterns from text. |
| **pr-comment-humanizer** | Humanize PR review comments before posting. |

### Development Workflows

| Skill | Purpose |
|---|---|
| **implement-jira** | Fetch a Jira ticket, plan with Opus, implement with Sonnet, run tests, then review. |
| **systematic-debugging** | Structured debugging workflow before proposing any fix. |
| **test-driven-development** | TDD workflow for features and bugfixes. |
| **brainstorming** | Explore intent, requirements, and design before implementation. |
| **verification-before-completion** | Run verification commands and confirm output before claiming work is done. |
| **finishing-a-development-branch** | Guide completion of development work with structured options for merge, PR, or cleanup. |

### Planning & Orchestration

| Skill | Purpose |
|---|---|
| **writing-plans** | Write implementation plans from specs or requirements before touching code. |
| **executing-plans** | Execute written plans in a separate session with review checkpoints. |
| **dispatching-parallel-agents** | Handle 2+ independent tasks that can be worked on without shared state. |
| **subagent-driven-development** | Execute implementation plans with independent tasks via subagents. |
| **using-git-worktrees** | Ensure an isolated workspace exists via native tools or git worktree fallback. |

### Valkey & GLIDE

| Skill | Purpose |
|---|---|
| **glide** | Production-ready patterns for Valkey GLIDE clients across 6 languages. |
| **check-valkey-search-compatibility** | Analyze a repository for Redis/Valkey Search compatibility and recommend implementation paths. |

### Meta (Skill/Agent Management)

| Skill | Purpose |
|---|---|
| **create-skill** | Create a new skill for AI agents. |
| **update-skill** | Update or modify an existing skill. |
| **update-agent** | Update or modify an existing agent definition. |
| **writing-skills** | Create, edit, or verify skills before deployment. |
| **find-skills** | Discover and install agent skills. |
| **using-superpowers** | Establishes how to find and use skills at conversation start. |

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
