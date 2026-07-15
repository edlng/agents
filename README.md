# AI Agents & Skills

A collection of AI agents and skills for software development workflows.

## Agents

Agents are defined in `agents/` as JSON configs with co-located prompt files. They run via `kiro-cli` or `claude` and can be composed into multi-agent pipelines.

### Orchestration

| Agent | Model | Purpose |
|---|---|---|
| **team-lead** | Sonnet | Orchestrator. Breaks specs into tasks, delegates to builder/validator/code-reviewer with two-stage review (spec compliance then quality). Never writes code. Continuous execution without pausing. |
| **context-curator** | Haiku | Memory curation. Given a task description, selects relevant memories from Valkey/Obsidian and returns a `<context-memory>` block for worker injection. |
| **team-leader** | Sonnet | Legacy orchestrator. Delegates to researcher/developer/tester/code-reviewer. |

### Implementation

| Agent | Model | Purpose |
|---|---|---|
| **builder** | Sonnet | Worker agent. Executes one scoped implementation task at a time. Reports DONE/DONE_WITH_CONCERNS/BLOCKED/NEEDS_CONTEXT. Fresh subagent per task. |
| **developer** | - | General-purpose developer agent using the CAO MCP server. |
| **superhuman** | Opus | Expert engineer for complex multi-step tasks requiring deep reasoning across AWS, backend, UI/UX, and architecture. |

### Quality Assurance

| Agent | Model | Purpose |
|---|---|---|
| **tester** | Sonnet | Runs tests, linters, and security scans. Writes missing tests. Recommends merge only when all quality gates pass. |
| **validator** | Opus | Spec compliance verification. Reads actual code and verifies requirements are met (nothing missing, nothing extra). Scores and issues PASS/FAIL. |
| **code-reviewer** | Sonnet | Code quality reviewer. Issues APPROVE/BLOCK verdicts with evidence-backed findings. Checks codebase alignment, correctness, maintainability. |
| **security-reviewer** | Sonnet | Threat-model-driven security analysis anchored to CWE taxonomy. Injection, access control, secrets, crypto, SSRF, path traversal. |
| **glide-code-reviewer** | Sonnet | Specialized GLIDE reviewer. Subagent of code-reviewer for Valkey GLIDE client code. |

### Research

| Agent | Model | Purpose |
|---|---|---|
| **researcher** | Sonnet | External research via Firecrawl. Returns concise findings with source URLs and a concrete recommendation. |
| **research-validator** | Sonnet | Adversarially verifies researcher findings by cross-checking cited source URLs. |
| **research-summarizer** | Sonnet | Orchestrates researcher and research-validator, then produces a final synthesis. |

### Documentation

| Agent | Model | Purpose |
|---|---|---|
| **documenter** | Haiku | Generates documentation after build/validate cycles. Read-only for implementation files. |

### Valkey & GLIDE

| Agent | Model | Purpose |
|---|---|---|
| **valkey-glide-implementor** | Sonnet | Generates production-ready Valkey GLIDE code snippets across 6 languages for client setup, vector search, batch operations, caching, and session management. |

## Skills

Skills are invokable via `/skill-name` in Claude Code, Kiro CLI, or Codex CLI. They live in `skills/` and are synced to `~/.kiro/skills/`, `~/.claude/skills/`, `~/.config/devin/skills/`, and `~/.codex/skills/` via `make push`. Project-scoped skills live in `.kiro/skills/`.

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
| **write-pr-comments** | Post inline PR review comments from an Obsidian review note. |
| **write-narrative** | Draft a humanized technical narrative from a Jira issue. |
| **humanizer** | Remove AI writing patterns from text. |
| **pr-comment-humanizer** | Humanize PR review comments before posting. |

### Development Workflows

| Skill | Purpose |
|---|---|
| **implement-jira** | Fetch a Jira ticket, plan with Opus, implement with Sonnet, run tests, then review. |
| **implement-task** | Single-command entry point that takes a plain-text task description and runs plan → advisor loop → implement → test → review. No Jira ticket needed. |
| **subagent-driven-development** | Execute implementation plans with independent tasks via subagents. Two-stage review (spec then quality) per task. |
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

## Shared References

Shared refs in `skills/_shared/` encode cross-cutting conventions used by multiple agents and skills:

| Reference | Purpose |
|---|---|
| **security-constraints.md** | Standardized security boundary for all agents: credential access, exfiltration, destructive commands, prompt injection resistance. |
| **memory-protocol.md** | Scoped memory system using Valkey (hot) + Obsidian (durable). Defines scopes, key naming, injection format, and store/recall conventions. |
| **async-dispatch-protocols.md** | Idle-based delivery, callback patterns, and anti-patterns for async agent dispatch. |
| **five-root-sync.md** | Sync convention for keeping agents/skills identical across `~/.kiro/`, `~/.claude/`, `~/agents/`, `~/.config/devin/`, and `~/.codex/`. |
| **pr-review-base.md** | Base workflow for senior-grade PR reviews (context, merged-lens review, validator pass, report). |
| **review-findings-schema.md** | JSON schema and severity labels for review findings. |
| **validator-skeptic-pass.md** | Self-challenge rubric for the Opus validator skeptic pass. |
| **humanizer-rules.md** | Rules for removing AI writing patterns from text. |
| **valkey-cache-conventions.md** | How skills use Valkey at localhost:8888 as a shared cache. |
| **codebase-context-checklist.md** | What to gather about touched files before reviewing. |
| **no-github-writes.md** | Enforcement rule: reviews are local-only, never posted to GitHub. |

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
make push   # copy from repo → ~/.kiro and ~/.claude
make pull   # copy from ~/.kiro → repo
make status # diff between repo and ~/.kiro
```

## Evaluation Suite

### Litmus: cheap production checks

Litmus is the compact path for deterministic agent checks and bounded live probes. Start with a zero-cost replay, then use a small live budget only when needed:

```bash
make litmus-list
make litmus-replay AGENT=code-reviewer CASE=eval-exec-injection
make litmus-probe AGENT=code-reviewer CASE=clean-code-approval BUDGET=0.25
make litmus-batch MANIFEST=core BUDGET=0.60
make litmus-batch MANIFEST=core BUDGET=0.60 BATCH_ARGS='--jobs 1'
make litmus-batch MANIFEST=full BUDGET=1000 BATCH_ARGS='--include-replay-only --jobs 3'
make litmus-compare BASELINE=litmus/results/<baseline> CURRENT=litmus/results/<current>
```

`replay` checks captured output without calling a model and costs `$0`. The complete 20-case catalog is replay-only by default. `probe` and `batch` reject a case unless it is explicitly marked `"live": true`; the initial live core contains only the two code-reviewer checks with recorded low-cost production runs. The explicit `--include-replay-only` batch override is reserved for a deliberate full-catalog run. Batches run up to three probes concurrently by default; pass `--jobs 1` for serial execution. Litmus reserves each pending case's target before scheduling another one, but Claude's dollar cap is advisory rather than a strict per-call ceiling. Treat a live budget as a scheduling limit, inspect recorded actual cost after every run, and expect concurrent calls to be able to overshoot it. The verified `code-reviewer/clean-code-approval` probe cost `$0.03` with a `$0.25` target. Replay, probe, and batch write pretty JSON and Markdown artifacts under `litmus/results/`; these results are intentionally committed for future comparison and visualization.

Add cases as JSON under `litmus/cases/<agent>/`, pair them with replay output under `litmus/replays/<agent>/`, and include only vetted, low-risk cases in `litmus/manifests/core.json`. A native provider API with an output-token cap is required before a live budget can be treated as a strict spend ceiling.

## Credits

Orchestration patterns (review iteration loop, security constraints block, memory protocol, async dispatch conventions) adapted from [CLI Agent Orchestrator (CAO)](https://github.com/awslabs/cli-agent-orchestrator) by AWS Labs — an open-source multi-agent orchestration framework for AI coding CLIs. CAO's supervisor/worker protocols and scoped memory system informed the design of the team-lead agent and shared references in this collection.
