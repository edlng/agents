#!/usr/bin/env bash
# Provider wrapper for claude-code (Claude Code CLI).
# Promptfoo calls: providers/claude_code.sh <prompt> <options_json> <context_json>
set -euo pipefail

PROMPT="$1"

# Pipe prompt via stdin to avoid slash-command interpretation and ARG_MAX issues.
# Use --max-turns 3 to allow tool use (e.g. skill activation, file reads) before final answer.
echo "$PROMPT" | claude -p --output-format text --max-turns 3
