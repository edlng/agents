#!/usr/bin/env bash
# Capture the user's prompt for the Codex journaler.
set -euo pipefail

# The journaler's own codex exec must not create another journal prompt.
if [[ "${AI_MATURITY_JOURNALER:-}" == "1" ]]; then
    exit 0
fi

EVENT="$(cat)"
SESSION_ID="$(echo "${EVENT}" | jq -r '.session_id // empty')"
PROMPT="$(echo "${EVENT}" | jq -r '.prompt // empty')"

if [[ -n "${SESSION_ID}" && -n "${PROMPT}" ]]; then
    PROMPT_FILE="/tmp/codex-journal-prompt-${SESSION_ID}"
    echo "${PROMPT}" > "${PROMPT_FILE}"
fi

exit 0
