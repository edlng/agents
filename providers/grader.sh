#!/usr/bin/env bash
# Grading provider for LLM-as-judge assertions (llm-rubric, model-graded-closedqa).
# Used as the defaultTest.options.provider in promptfooconfig.yaml.
set -euo pipefail

PROMPT="$1"

claude -p --output-format text --max-turns 1 "$PROMPT"
