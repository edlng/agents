#!/usr/bin/env bash
# Universal agent eval provider.
# Usage (direct):    agent.sh <agent-name> <prompt>
# Usage (promptfoo): agent.sh <agent-name> <prompt> <options-json> <context-json>
#   When agent-name is "placeholder", reads the actual agent from context JSON vars.agent.
#
# - Reads the agent's system prompt and model from agents/
# - Runs via claude -p --output-format json
# - Outputs only the text result (for promptfoo)
# - Appends token metrics to evals/metrics/token_usage.jsonl
set -euo pipefail

AGENT_NAME="$1"
PROMPT="$2"
CONTEXT_JSON="${4:-}"

# If called as placeholder (global provider fallback), extract agent name from context vars
if [[ "$AGENT_NAME" == "placeholder" && -n "$CONTEXT_JSON" ]]; then
  AGENT_NAME=$(echo "$CONTEXT_JSON" | jq -r '.vars.agent // empty' 2>/dev/null || true)
  if [[ -z "$AGENT_NAME" ]]; then
    echo "ERROR: agent name not found in context vars" >&2
    exit 1
  fi
fi

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
AGENTS_DIR="$REPO_ROOT/agents"
METRICS_FILE="$REPO_ROOT/evals/metrics/token_usage.jsonl"

# Resolve system prompt: agents dir first, then skills dir, then inline JSON
SKILLS_DIR="$REPO_ROOT/skills"

if [[ -f "$AGENTS_DIR/${AGENT_NAME}-prompt.md" ]]; then
  SYSTEM_PROMPT=$(cat "$AGENTS_DIR/${AGENT_NAME}-prompt.md")
elif [[ -f "$AGENTS_DIR/${AGENT_NAME}.md" ]]; then
  SYSTEM_PROMPT=$(cat "$AGENTS_DIR/${AGENT_NAME}.md")
elif [[ -f "$SKILLS_DIR/${AGENT_NAME}/SKILL.md" ]]; then
  SYSTEM_PROMPT=$(cat "$SKILLS_DIR/${AGENT_NAME}/SKILL.md")
elif [[ -f "$AGENTS_DIR/${AGENT_NAME}.json" ]]; then
  INLINE=$(jq -r '.prompt // empty' "$AGENTS_DIR/${AGENT_NAME}.json" 2>/dev/null || true)
  if [[ -n "$INLINE" && "$INLINE" != "null" && ! "$INLINE" == file://* ]]; then
    SYSTEM_PROMPT="$INLINE"
  else
    echo "ERROR: No prompt file found for agent '$AGENT_NAME'" >&2
    exit 1
  fi
else
  echo "ERROR: No prompt file found for agent or skill '$AGENT_NAME'" >&2
  exit 1
fi

# Resolve model from JSON config (default to sonnet)
if [[ -f "$AGENTS_DIR/${AGENT_NAME}.json" ]]; then
  RAW_MODEL=$(jq -r '.model // "claude-sonnet-4.6"' "$AGENTS_DIR/${AGENT_NAME}.json" 2>/dev/null || echo "claude-sonnet-4.6")
else
  RAW_MODEL="claude-sonnet-4.6"
fi

# Normalize to short aliases that claude CLI accepts.
# Agent configs use full names (claude-sonnet-4.6, claude-opus-4.8) which the CLI may reject.
# Pattern: strip "claude-" prefix, then match the family name before the version number.
# This handles current and future versions (e.g., claude-sonnet-5.0 → sonnet).
normalize_model() {
  local raw="$1"
  # Already a short alias
  case "$raw" in
    sonnet|haiku|opus) echo "$raw"; return ;;
  esac
  # Strip "claude-" prefix if present, then extract family name
  local stripped="${raw#claude-}"
  case "$stripped" in
    sonnet*) echo "sonnet" ;;
    haiku*)  echo "haiku" ;;
    opus*)   echo "opus" ;;
    *)       echo "$raw" ;;  # Unknown model, pass through
  esac
}
MODEL=$(normalize_model "$RAW_MODEL")

# For researcher/research-validator: inject stub fixture so no live network calls needed
if [[ "$AGENT_NAME" == "researcher" || "$AGENT_NAME" == "research-validator" ]]; then
  FIXTURE=$(bash "$REPO_ROOT/evals/providers/stubs/firecrawl_stub.sh")
  PROMPT="$PROMPT

[Search results available — use these as your source]:
$FIXTURE"
fi

# For glide-dependent agents: inject minimal skill content so they don't try to read from disk
if [[ "$AGENT_NAME" == "glide-code-reviewer" || "$AGENT_NAME" == "valkey-glide-implementor" ]]; then
  GLIDE_STUB="[GLIDE Skill Reference — Python]:
- Client lifecycle: Create ONE GlideClient at startup, reuse across requests. NEVER create per-request.
- Batch API (v2.x): Use ClusterBatch/Batch for pipelining. Append ops, call exec(). Transaction is DEPRECATED.
- Imports: from glide import GlideClient, GlideClusterClient, GlideClientConfiguration, NodeAddress
- Batch: from glide import ClusterBatch (cluster) or Batch (standalone)
- Anti-patterns: asyncio.gather for parallel ops (use Batch), client-per-request, Transaction API, redis-py imports
- Error handling: RequestError, TimeoutError, ClosingError — wrap in try/except with retry for transient errors
- Search: use native ft.create/ft.search/ft.aggregate — NEVER use custom_command for search ops
- Dependencies: valkey-glide>=2.1.0 in requirements.txt"
  PROMPT="$GLIDE_STUB

$PROMPT"
fi

# Run the agent (cap budget to control cost in evals)
MAX_BUDGET="${EVAL_MAX_BUDGET:-0.40}"
START_TIME=$(date +%s)

# Most agents don't need tools in eval mode (single-turn, no filesystem).
# Allow tools only for agents whose test requires it.
TOOL_FLAG=""
case "$AGENT_NAME" in
  researcher|research-validator|builder) ;; # need tools for their workflow
  *) TOOL_FLAG="--allowedTools none" ;;
esac

RAW_OUTPUT=$(echo "$PROMPT" | claude -p \
  --output-format json \
  --model "$MODEL" \
  --max-budget-usd "$MAX_BUDGET" \
  $TOOL_FLAG \
  --system-prompt "$SYSTEM_PROMPT" \
  2>/dev/null)
DURATION=$(( $(date +%s) - START_TIME ))

# Handle budget-exceeded or error responses
IS_ERROR=$(echo "$RAW_OUTPUT" | jq -r '.is_error // false' 2>/dev/null || echo "true")
if [[ "$IS_ERROR" == "true" ]]; then
  ERROR_MSG=$(echo "$RAW_OUTPUT" | jq -r '.errors[0] // "Unknown error"' 2>/dev/null || echo "Provider error")
  echo "ERROR: $ERROR_MSG" >&2
  exit 1
fi

# Extract text result for promptfoo
RESULT=$(echo "$RAW_OUTPUT" | jq -r '.result // ""')

# Append token metrics (modelUsage has real counts including cache tokens)
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
MODEL_USAGE=$(echo "$RAW_OUTPUT" | jq -r '[.modelUsage // {} | to_entries[] | .value] | first // {}')
INPUT_TOKENS=$(echo "$MODEL_USAGE" | jq -r '(.inputTokens // 0) + (.cacheReadInputTokens // 0) + (.cacheCreationInputTokens // 0)')
OUTPUT_TOKENS=$(echo "$MODEL_USAGE" | jq -r '.outputTokens // 0')
TOTAL_COST=$(echo "$RAW_OUTPUT" | jq -r '.total_cost_usd // 0')

mkdir -p "$(dirname "$METRICS_FILE")"
echo "{\"agent\":\"$AGENT_NAME\",\"model\":\"$MODEL\",\"input_tokens\":$INPUT_TOKENS,\"output_tokens\":$OUTPUT_TOKENS,\"total_cost_usd\":$TOTAL_COST,\"duration_s\":$DURATION,\"timestamp\":\"$TIMESTAMP\"}" >> "$METRICS_FILE"

echo "$RESULT"
