#!/usr/bin/env bash
# Researcher stub provider — wraps researcher.sh with FIRECRAWL_API_URL pointed at
# the local fixture stub so no live network calls are made.
# Set STUB_FIRECRAWL=1 (default) to use stub; unset to fall through to real researcher.
set -euo pipefail

PROMPT="$1"
STUB_FIRECRAWL="${STUB_FIRECRAWL:-1}"

if [[ "$STUB_FIRECRAWL" == "1" ]]; then
  # Override the Firecrawl API URL to a non-existent address; the researcher agent
  # will get a connection error and fall back to the fixture data injected in the prompt.
  # We inject the fixture content directly so the agent has something to reason over.
  FIXTURE=$(bash "$(dirname "$0")/firecrawl_stub.sh")
  AUGMENTED_PROMPT="$PROMPT

[Search results available — use these as your source]:
$FIXTURE"
  exec bash providers/researcher.sh "$AUGMENTED_PROMPT"
else
  exec bash providers/researcher.sh "$PROMPT"
fi
