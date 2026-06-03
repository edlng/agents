# Promptfoo Skill/Agent Evaluations

Evaluate AI skills and agents across 5 dimensions: **accuracy**, **cost**, **cost-accuracy ratio**, **scoring rubrics**, and **completeness**.

Uses `claude-code` and `kiro-cli` as providers — no API keys required.

## Setup

```bash
npm install
```

Ensure both CLIs are available:

```bash
claude --version   # Claude Code CLI (used as provider + grader)
kiro-cli --version     # Kiro CLI
```

## Run Evaluations

```bash
./run-eval.sh          # Run all tests, cache preserved between runs
./run-eval.sh --reset  # Clear state + run (use when DB errors occur)
npm run eval           # Run all test cases directly
npm run eval:view      # Open results in browser UI
npm run eval:reset     # Nuke promptfoo state (fixes DB errors)
```

## Evaluation Dimensions

| Dimension | How It's Measured | Assertion Types Used |
|-----------|-------------------|---------------------|
| **Accuracy** | Exact match + LLM-graded correctness | `contains`, `model-graded-closedqa` |
| **Cost** | Latency as proxy (faster = cheaper) | `latency` |
| **Cost-Accuracy Ratio** | Custom scoring function + derived metric | `derivedMetrics`, `assertScoringFunction` |
| **Scoring Rubrics** | Multi-criteria LLM judge (via claude-code) | `llm-rubric` |
| **Completeness** | All required elements present | `contains-all`, `javascript`, `is-json` |

## Project Structure

```
├── promptfooconfig.yaml       # Main config: providers, tests, assertions
├── providers/
│   ├── claude_code.sh         # exec: provider wrapping `claude -p`
│   ├── kiro_cli.sh            # exec: provider wrapping `kiro chat`
│   └── grader.sh              # LLM-as-judge grading via claude -p
├── prompts/
│   └── skill_eval.txt         # Prompt template (uses {{task}} and {{context}} vars)
├── scoring.js                 # Custom scoring function (latency-weighted quality)
└── package.json
```

## How Providers Work

Since there are no API keys, providers use promptfoo's `exec:` mechanism to shell out to CLI tools:

- **claude-code**: `claude -p --output-format text --max-turns 1 "<prompt>"`
- **kiro-cli**: `echo "<prompt>" | kiro chat - --mode ask`
- **grader** (for LLM-as-judge assertions): same as claude-code

## Customizing

### Add a new test case

```yaml
tests:
  - description: 'Your test description'
    vars:
      task: 'The task to evaluate'
      context: 'domain context'
    assert:
      - type: llm-rubric
        value: 'Your grading criteria here'
        metric: rubric_quality
        weight: 2
      - type: contains
        value: 'expected substring'
        metric: accuracy
        weight: 3
```

### Adjust cost-accuracy tradeoff

Edit `scoring.js` to change the weight split between quality (85%) and latency (15%):

```js
const finalScore = qualityScore * 0.85 + costPenalty * 0.15;
```

### Use only one provider

Comment out the provider you don't want in `promptfooconfig.yaml`:

```yaml
providers:
  - id: 'exec: bash providers/claude_code.sh'
    label: claude-code
  # - id: 'exec: bash providers/kiro_cli.sh'
  #   label: kiro-cli
```

## Cost Efficiency

### Current cost per eval run

| Component | Calls/run | Notes |
|-----------|-----------|-------|
| Provider calls | 44 (4 deterministic × 1 + 6 non-deterministic × 3, × 2 providers) | Down from 60 with flat repeat=3 |
| LLM grading calls | 22 (5 tests with llm-rubric/g-eval × 2 providers, weighted by repeat) | Down from 54+ with redundant assertions |
| **Total LLM invocations** | **~66** | **Down from ~114** — cache preserved between runs |

### Design decisions

1. **Cache preserved between runs** — `run-eval.sh` no longer clears `~/.promptfoo`. Cached results are reused when prompts/tests haven't changed. Use `--reset` flag only when encountering DB errors.

2. **LLM grading only where JS can't cover** — Tests 1, 2, 6, 7 use deterministic `icontains`/`javascript` assertions that fully validate correctness. LLM grading is reserved for subjective quality (Tests 3, 5, 8, 9, 10) where keyword matching isn't sufficient.

3. **Per-test repeat** — Only non-deterministic tests (code reviews, explanations) run 3 times. Factual Q&A and structured output tests run once since variance is near zero.

4. **Full model for all grading** — `grader.sh` uses the same Claude model regardless of task complexity. This is the largest remaining inefficiency. If API-based grading is ever added, trivial checks (e.g., "does it mention Paris?") should use a cheaper model.

### Reducing cost further

```bash
# Quick iteration: single pass, rely on cache
npx promptfoo eval                          # uses default repeat=1

# Override repeat for specific investigation
EVAL_REPEAT=5 ./run-eval.sh                 # higher repeat for statistical analysis

# Reset only when needed
./run-eval.sh --reset                       # clear state + run
```

## Interpreting Results

- **Named metrics** appear as columns in the web UI — compare accuracy, completeness, and latency across providers
- **`cost_accuracy_ratio`** derived metric shows latency penalty per unit of quality (lower = better)
- **`scoring.js`** produces the final pass/fail by blending quality and latency signals
