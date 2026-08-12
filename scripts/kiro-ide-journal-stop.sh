#!/usr/bin/env bash
# Read the Kiro IDE transcript after each turn and send a short context to the
# shared Codex GPT 5.6 Luna journaler without blocking the IDE.
set -euo pipefail

JQ="$(command -v jq || true)"
for candidate in /opt/homebrew/bin/jq /usr/local/bin/jq /usr/bin/jq; do
    [[ -n "${JQ}" ]] && break
    [[ -x "${candidate}" ]] && JQ="${candidate}"
done
[[ -z "${JQ}" ]] && exit 0

EVENT="$(cat)"
SESSION_ID="$(printf '%s' "${EVENT}" | "${JQ}" -r '.session_id // empty' 2>/dev/null || true)"
case "${SESSION_ID}" in
    ''|*[!A-Za-z0-9_-]*) exit 0 ;;
esac

KIRO_DIR="${KIRO_IDE_HOME:-${HOME}/.kiro}"
SESSION_DIR="sess_${SESSION_ID}"
if [[ "${SESSION_ID}" == sess_* ]]; then
    SESSION_DIR="${SESSION_ID}"
fi
TRANSCRIPT="$(
    find "${KIRO_DIR}/sessions" -type f \
        -path "*/${SESSION_DIR}/messages.jsonl" -print -quit 2>/dev/null || true
)"
PROMPT_FILE="/tmp/kiro-ide-journal-prompt-${SESSION_ID}"

[[ -n "${TRANSCRIPT}" ]] || {
    rm -f "${PROMPT_FILE}"
    exit 0
}

DATA=""
PROMPT_FROM_TRANSCRIPT=""
RESPONSE=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
    DATA="$(
        "${JQ}" -s -c '
            . as $records
            | def has_text($expected):
                .payload.type == $expected
                and (.payload.content | type == "string")
                and (.payload.content | length > 0);
            ([range(0; length) as $index
              | select($records[$index] | has_text("user"))
              | $index] | last // -1) as $user_index
            | ([range(0; length) as $index
                | select($records[$index] | has_text("assistant"))
                | $index] | last // -1) as $assistant_index
            | {
                prompt: (
                    if $user_index >= 0
                    then $records[$user_index].payload.content
                    else ""
                    end
                ),
                response: (
                    if $user_index >= 0 and $assistant_index > $user_index
                    then $records[$assistant_index].payload.content
                    else ""
                    end
                )
            }
        ' "${TRANSCRIPT}" 2>/dev/null || true
    )"

    if [[ -n "${DATA}" ]]; then
        PROMPT_FROM_TRANSCRIPT="$(printf '%s' "${DATA}" | "${JQ}" -r '.prompt // empty')"
        RESPONSE="$(printf '%s' "${DATA}" | "${JQ}" -r '.response // empty')"
        [[ -n "${RESPONSE}" ]] && break
    fi
    sleep 0.1
done

if [[ -z "${RESPONSE}" ]]; then
    rm -f "${PROMPT_FILE}"
    exit 0
fi

PROMPT=""
if [[ -f "${PROMPT_FILE}" ]]; then
    PROMPT="$(cat "${PROMPT_FILE}" 2>/dev/null || true)"
    rm -f "${PROMPT_FILE}"
fi
[[ -n "${PROMPT}" ]] || PROMPT="${PROMPT_FROM_TRANSCRIPT}"

PROMPT_TRUNCATED="${PROMPT:0:300}"
RESPONSE_TRUNCATED="${RESPONSE:0:500}"
CONTEXT="User asked: ${PROMPT_TRUNCATED}
Assistant responded (summary): ${RESPONSE_TRUNCATED}"

SCRIPTS_DIR="${KIRO_DIR}/scripts/ai-maturity"
JOURNALER="${SCRIPTS_DIR}/journal-entry.sh"
LOG_FILE="${SCRIPTS_DIR}/kiro-ide-journaler.log"

if [[ -x "${JOURNALER}" ]]; then
    printf '%s' "${CONTEXT}" | "${JOURNALER}" >> "${LOG_FILE}" 2>&1 &
fi

exit 0
