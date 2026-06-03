#!/usr/bin/env bash
# Provider wrapper for the researcher agent (via kiro-cli).
# Routes factual/research questions through the researcher agent definition.
set -euo pipefail

PROMPT="$1"

kiro-cli chat --no-interactive --agent agents/researcher.json "$PROMPT" | sed 's/^> //'
