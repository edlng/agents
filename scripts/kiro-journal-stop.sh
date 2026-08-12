#!/usr/bin/env bash
# Fire the Codex-backed journaler after every Kiro CLI response.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/journaler.log"

EVENT="$(cat)"
SESSION_ID="$(echo "${EVENT}" | jq -r '.session_id // empty')"
RESPONSE="$(echo "${EVENT}" | jq -r '.assistant_response // empty')"

if [[ -z "${RESPONSE}" ]]; then
    exit 0
fi

RESPONSE_TRUNCATED="${RESPONSE:0:500}"

PROMPT=""
PROMPT_FILE="/tmp/kiro-journal-prompt-${SESSION_ID}"
if [[ -f "${PROMPT_FILE}" ]]; then
    PROMPT="$(cat "${PROMPT_FILE}")"
    rm -f "${PROMPT_FILE}"
fi

PROMPT_TRUNCATED="${PROMPT:0:300}"
CONTEXT="User asked: ${PROMPT_TRUNCATED}
Assistant responded (summary): ${RESPONSE_TRUNCATED}"

echo "${CONTEXT}" | "${SCRIPT_DIR}/journal-entry.sh" >> "${LOG_FILE}" 2>&1 &

exit 0
