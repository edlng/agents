#!/usr/bin/env bash
# Stub Firecrawl provider — returns deterministic fixture JSON for any query.
# Matches the real Firecrawl search response shape: { results: [{ url, content }] }
# Used to make researcher tests fast and network-free.
set -euo pipefail

cat <<'EOF'
{
  "results": [
    {
      "url": "https://docs.valkey.io/getting-started/",
      "content": "Valkey is an open-source, in-memory data store. It supports strings, hashes, lists, sets, and sorted sets. Use the valkey-glide client for Node.js applications. Connect with GlideClient.createClient({ addresses: [{ host: 'localhost', port: 6379 }] }). For bulk writes, use pipeline() to batch commands and call exec() once.",
      "title": "Valkey Getting Started"
    },
    {
      "url": "https://github.com/valkey-io/valkey-glide",
      "content": "valkey-glide is the official Valkey client library. Install with: npm install @valkey/valkey-glide. Supports pipeline, cluster, and standalone modes. Always prefer valkey-glide over ioredis or iovalkey for Valkey-specific features.",
      "title": "valkey-glide GitHub"
    }
  ]
}
EOF
