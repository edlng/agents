#!/usr/bin/env bash
# Grading provider for LLM-as-judge assertions (llm-rubric, model-graded-closedqa).
# Used as the defaultTest.options.provider in promptfooconfig.yaml.
set -euo pipefail

PROMPT="$1"

# Check if prompt is a JSON chat array and flatten it
if [[ "$PROMPT" =~ ^\[.*\]$ ]]; then
  # Extract content from chat array
  PROMPT=$(echo "$PROMPT" | jq -r '.[] | select(.content) | .content' | tr '\n' ' ')
fi

# Pipe prompt via stdin to avoid issues with special characters in code blocks.
echo "$PROMPT" | claude -p --output-format text --max-turns 1 --append-system-prompt "Output only valid minified JSON, no prose, no code fences"
