#!/usr/bin/env bash
# Run evaluations. Cache is preserved between runs for efficiency.
# Use --reset flag or make eval-reset to clear state when needed.
# Use --filter <pattern> to run a subset of tests (matches description).
#
# CACHING NOTE: promptfoo caches responses by task text + provider command,
# NOT by system prompt content. If you edit an agent prompt file, you MUST
# use --no-cache or --reset to get fresh results. Otherwise stale cached
# outputs will be scored against your new assertions.
set -euo pipefail

FILTER=""
if [[ "${1:-}" == "--reset" ]]; then
  echo "==> Clearing promptfoo state..."
  rm -rf ~/.promptfoo
  shift
fi
if [[ "${1:-}" == "--filter" ]]; then
  FILTER="$2"
  shift 2
fi

REPEAT=${EVAL_REPEAT:-1}
FILTER_ARG=""
[[ -n "$FILTER" ]] && FILTER_ARG="--filter-pattern $FILTER"

echo "==> Running evaluations (repeat=${REPEAT}${FILTER:+, filter=$FILTER})..."
npx promptfoo eval --repeat "$REPEAT" $FILTER_ARG

echo "==> Done. Run 'make eval-view' to see results in browser."
