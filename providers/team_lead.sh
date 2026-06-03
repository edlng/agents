#!/usr/bin/env bash
# Provider wrapper for the team-lead agent (via kiro-cli).
set -euo pipefail

PROMPT="$1"

kiro-cli chat --no-interactive --agent agents/team-lead.json "$PROMPT" | sed 's/^> //'
