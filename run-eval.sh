#!/usr/bin/env bash
# Run evaluations with automatic recovery from DB errors.
set -euo pipefail

echo "==> Clearing promptfoo cache..."
rm -rf ~/.promptfoo

REPEAT=${EVAL_REPEAT:-3}
echo "==> Running evaluations (repeat=${REPEAT})..."
npx promptfoo eval --repeat "$REPEAT"

echo "==> Done. Run 'npm run eval:view' to see results in browser."
