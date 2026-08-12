#!/usr/bin/env bash
# Capture the Kiro IDE prompt so the paired Stop hook can journal it.
set -euo pipefail

JQ="$(command -v jq || true)"
for candidate in /opt/homebrew/bin/jq /usr/local/bin/jq /usr/bin/jq; do
    [[ -n "${JQ}" ]] && break
    [[ -x "${candidate}" ]] && JQ="${candidate}"
done
[[ -z "${JQ}" ]] && exit 0

EVENT="$(cat)"
SESSION_ID="$(printf '%s' "${EVENT}" | "${JQ}" -r '.session_id // empty' 2>/dev/null || true)"
PROMPT="$(printf '%s' "${EVENT}" | "${JQ}" -r '.prompt // empty' 2>/dev/null || true)"

case "${SESSION_ID}" in
    ''|*[!A-Za-z0-9_-]*) exit 0 ;;
esac
[[ -z "${PROMPT}" ]] && exit 0

umask 077
printf '%s' "${PROMPT}" > "/tmp/kiro-ide-journal-prompt-${SESSION_ID}"

exit 0
