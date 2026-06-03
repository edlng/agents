#!/usr/bin/env bash
# Provider wrapper for the code-reviewer-eval agent (via kiro-cli).
set -euo pipefail

PROMPT="$1"

kiro-cli chat --no-interactive --agent agents/code-reviewer.json "$PROMPT" | sed 's/^> //'
