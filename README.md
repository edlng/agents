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
kiro --version     # Kiro CLI
```

## Run Evaluations

```bash
npm run eval          # Run all test cases
npm run eval:view     # Open results in browser UI
npm run eval:cache-clear  # Clear cached responses
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

## Interpreting Results

- **Named metrics** appear as columns in the web UI — compare accuracy, completeness, and latency across providers
- **`cost_accuracy_ratio`** derived metric shows latency penalty per unit of quality (lower = better)
- **`scoring.js`** produces the final pass/fail by blending quality and latency signals
