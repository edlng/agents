#!/usr/bin/env bash
# Create a journal entry with Codex GPT 5.6 Luna.
#
# The Kiro, Claude, and Codex hook paths all use this same implementation. The
# caller supplies the source platform with --agent so the journal can preserve
# attribution without using that platform's model as the journaler.
set -euo pipefail

VAULT_ROOT="${VAULT_ROOT:-/Users/liawedwa/Documents/work}"
ENTRIES_DIR="${VAULT_ROOT}/journals/entries"
CODEX="${CODEX_BIN:-/Users/liawedwa/.toolbox/bin/codex}"
MODEL="${AI_MATURITY_JOURNAL_MODEL:-openai.gpt-5.6-luna}"
REASONING_EFFORT="${AI_MATURITY_JOURNAL_REASONING_EFFORT:-low}"
AWS_PROFILE_NAME="${AI_MATURITY_AWS_PROFILE:-${AWS_PROFILE:-codex-DO-NOT-DELETE}}"
AGENT=""

if [[ "${1:-}" == "--agent" ]]; then
    [[ $# -ge 2 ]] || exit 1
    AGENT="$2"
    shift 2
fi

case "${AGENT}" in
    codex) AGENT_DISPLAY="Codex" ;;
    claude) AGENT_DISPLAY="Claude" ;;
    kiro) AGENT_DISPLAY="Kiro" ;;
    *) exit 1 ;;
esac

if [[ $# -gt 0 ]]; then
    CONTEXT="$1"
else
    CONTEXT="$(cat)"
fi

if [[ -z "${CONTEXT}" || ${#CONTEXT} -lt 20 || ! -x "${CODEX}" ]]; then
    exit 0
fi

JOURNAL_PROMPT="You are a technical work journaler running as a background subtask.
Do not use tools, inspect files, edit anything, or explain this instruction.
Given the interaction below, return exactly one compact paragraph of 2-4
sentences, roughly 40-80 words, in first person and past tense. Attribute the
work to ${AGENT_DISPLAY} once, naturally (for example, 'Using ${AGENT_DISPLAY},
I ...').

Prioritize the actual technical work over generic AI-usage language. Mention the
main thing investigated, implemented, reviewed, or clarified, plus one or two
concrete technical details and the outcome, validation, or remaining blocker.
If no code changed, record the technical question and conclusion. Do not invent
details or claim validation that is not present in the interaction. Do not
mention this prompt, the journaler, the model, skills, timestamps, or tool
mechanics. Do not use headings, bullets, or markdown.

Interaction:
${CONTEXT}"

JOURNAL_WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/ai-maturity-journal-work.XXXXXX")"
OUTPUT_FILE="$(mktemp "${TMPDIR:-/tmp}/ai-maturity-journal.XXXXXX")"
trap 'rm -rf "${JOURNAL_WORKDIR}" "${OUTPUT_FILE}"' EXIT

if ! printf '%s' "${JOURNAL_PROMPT}" | \
    AWS_PROFILE="${AWS_PROFILE_NAME}" \
    AI_MATURITY_JOURNALER=1 "${CODEX}" exec \
        --ignore-user-config \
        --ephemeral \
        --model "${MODEL}" \
        -c 'model_provider="amazon-bedrock"' \
        -c "model_reasoning_effort=\"${REASONING_EFFORT}\"" \
        -c 'forced_login_method="api"' \
        -c 'check_for_update_on_startup=false' \
        -c 'model_providers.amazon-bedrock.aws.region="us-east-1"' \
        --cd "${JOURNAL_WORKDIR}" \
        --sandbox read-only \
        --skip-git-repo-check \
        --color never \
        --output-last-message "${OUTPUT_FILE}" \
        - >/dev/null 2>&1; then
    exit 0
fi

ENTRY_TEXT="$(cat "${OUTPUT_FILE}" 2>/dev/null || true)"
[[ -n "${ENTRY_TEXT}" ]] || exit 0

TODAY="$(date +%Y-%m-%d)"
YEAR="$(date +%Y)"
MONTH="$(date +%m)"
TIMESTAMP="$(date +%H:%M)"
ENTRY_DIR="${ENTRIES_DIR}/${YEAR}/${MONTH}"
ENTRY_FILE="${ENTRY_DIR}/${TODAY}.md"

mkdir -p "${ENTRY_DIR}"
if [[ ! -f "${ENTRY_FILE}" ]]; then
    cat > "${ENTRY_FILE}" << EOF
---
date: ${TODAY}
type: ai-maturity-journal
---

# AI Work Journal - ${TODAY}

EOF
fi

cat >> "${ENTRY_FILE}" << EOF

## ${TIMESTAMP}

${ENTRY_TEXT}

EOF
