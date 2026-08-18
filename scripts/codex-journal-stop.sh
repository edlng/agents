#!/usr/bin/env bash
# Fire the Codex-backed journaler after every Codex turn.
set -euo pipefail

# The journaler's codex exec also loads Codex hooks. Do not recurse.
if [[ "${AI_MATURITY_JOURNALER:-}" == "1" ]]; then
    echo '{}'
    exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/journaler.log"

EVENT="$(cat)"
SESSION_ID="$(echo "${EVENT}" | jq -r '.session_id // empty')"
RESPONSE="$(echo "${EVENT}" | jq -r '.last_assistant_message // empty')"

if [[ -z "${RESPONSE}" ]]; then
    echo '{}'
    exit 0
fi

RESPONSE_TRUNCATED="${RESPONSE:0:1800}"

PROMPT=""
PROMPT_FILE="/tmp/codex-journal-prompt-${SESSION_ID}"
if [[ -f "${PROMPT_FILE}" ]]; then
    PROMPT="$(cat "${PROMPT_FILE}")"
    rm -f "${PROMPT_FILE}"
fi

PROMPT_TRUNCATED="${PROMPT:0:1000}"
CONTEXT="User asked: ${PROMPT_TRUNCATED}
Assistant responded (summary): ${RESPONSE_TRUNCATED}"

echo "${CONTEXT}" | "${SCRIPT_DIR}/journal-entry.sh" --agent codex >> "${LOG_FILE}" 2>&1 &

echo '{}'
exit 0
