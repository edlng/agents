#!/usr/bin/env bash
# Fire the Codex-backed journaler after every Claude Code response.
set -euo pipefail

# Preserve the legacy guard while also covering the Codex journaler's process.
if [[ "${AI_MATURITY_JOURNALER:-}" == "1" || "${CLAUDE_JOURNALER:-}" == "1" ]]; then
    exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/journaler.log"

EVENT="$(cat)"
SESSION_ID="$(echo "${EVENT}" | jq -r '.session_id // empty')"
RESPONSE="$(echo "${EVENT}" | jq -r '.last_assistant_message // empty')"

if [[ -z "${RESPONSE}" ]]; then
    exit 0
fi

PROMPT=""
PROMPT_FILE="/tmp/claude-journal-prompt-${SESSION_ID}"
if [[ -f "${PROMPT_FILE}" ]]; then
    PROMPT="$(cat "${PROMPT_FILE}")"
    rm -f "${PROMPT_FILE}"
fi

PROMPT_TRUNCATED="${PROMPT:0:1000}"
RESPONSE_TRUNCATED="${RESPONSE:0:1800}"
CONTEXT="User asked: ${PROMPT_TRUNCATED}
Assistant responded (summary): ${RESPONSE_TRUNCATED}"

export CONTEXT
if command -v setsid >/dev/null 2>&1; then
    setsid bash -c "printf '%s' \"\$CONTEXT\" | '${SCRIPT_DIR}/journal-entry.sh' >> '${LOG_FILE}' 2>&1" >/dev/null 2>&1 &
else
    nohup bash -c "printf '%s' \"\$CONTEXT\" | '${SCRIPT_DIR}/journal-entry.sh' >> '${LOG_FILE}' 2>&1" >/dev/null 2>&1 &
fi
disown 2>/dev/null || true

exit 0
