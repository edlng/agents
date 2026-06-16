#!/usr/bin/env bash
# Run evaluations. Cache is preserved between runs for efficiency.
# Use --reset flag or make eval-reset to clear state when needed.
set -euo pipefail

if [[ "${1:-}" == "--reset" ]]; then
  echo "==> Clearing promptfoo state..."
  rm -rf ~/.promptfoo
fi

REPEAT=${EVAL_REPEAT:-1}
echo "==> Running evaluations (repeat=${REPEAT})..."
npx promptfoo eval --repeat "$REPEAT"

echo "==> Done. Run 'make eval-view' to see results in browser."
