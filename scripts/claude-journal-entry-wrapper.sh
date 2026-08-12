#!/usr/bin/env bash
# Keep the existing Claude journal-entry path while using the shared Codex backend.
set -euo pipefail

exec "${HOME}/.codex/hooks/journal-entry.sh" --agent claude "$@"
