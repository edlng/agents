#!/usr/bin/env bash
# Provider wrapper for claude-code (Claude Code CLI).
# Promptfoo calls: providers/claude_code.sh <prompt> <options_json> <context_json>
set -euo pipefail

PROMPT="$1"

claude -p --output-format text --max-turns 1 "$PROMPT"
