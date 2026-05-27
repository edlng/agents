#!/usr/bin/env bash
# Provider wrapper for kiro-cli.
# Promptfoo calls: providers/kiro_cli.sh <prompt> <options_json> <context_json>
set -euo pipefail

PROMPT="$1"

kiro-cli chat --no-interactive "$PROMPT"
