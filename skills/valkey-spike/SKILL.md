---
name: valkey-spike
description: Analyze a repository for Valkey integration opportunities and empirically verify feasibility. Combines static codebase analysis (existing Redis/cache usage, command compatibility, vector search potential) with a live spike against a running Valkey instance to prove the riskiest assumptions before committing to a full implementation. Use when evaluating whether a project is a good candidate for Valkey integration.
---

# Valkey Spike

Analyze the repository `$ARGUMENTS` (or the current working directory if blank) for Valkey integration opportunities, then **prove the riskiest assumptions work** against a live Valkey instance.

This is a four-phase workflow:
1. **Evidence Collection** - scan the codebase
2. **Compatibility & Value Analysis** - identify opportunities
3. **Empirical Verification** - spike the risky parts against real Valkey
4. **Synthesis** - produce the final assessment with proven/disproven claims

The critical difference from pure static analysis: Phase 3 actually installs the GLIDE dependency, connects to a running Valkey, and runs the specific API calls the integration would need. This catches bugs that docs and reasoning alone miss (binary encoding issues, missing methods, module availability, platform incompatibilities).

---

## Phase 1: Evidence Collection

### 1.1 - Detect Existing Redis / Valkey Usage

Search the codebase for direct Redis or Valkey usage:

```bash
# Dependency files
grep -rn "redis\|valkey\|valkey-glide\|ioredis\|redis-py\|aioredis\|jedis\|lettuce\|stackexchange.redis" \
  --include="*.json" --include="*.toml" --include="*.txt" --include="*.gradle" \
  --include="*.xml" --include="*.lock" -i .

# Source code imports and connections
grep -rn "import redis\|import valkey\|from redis\|from valkey\|require.*redis\|require.*valkey\|RedisClient\|ValkeyClient\|createClient\|StrictRedis\|ConnectionPool" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.rb" --include="*.cs" -i .
```

Record: file paths, line numbers, library versions, client type (sync/async).

### 1.2 - Detect Caching Backends (Non-Redis)

If no Redis found, look for other caching layers:

```bash
# In-memory / local caches
grep -rn "memcached\|lru_cache\|cachetools\|node-cache\|caffeine\|guava.*cache\|ehcache\|@Cacheable\|cache_page\|diskcache" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.rb" --include="*.cs" -i .

# Cache framework abstractions
grep -rn "CacheManager\|cache_backend\|CACHES\|spring.cache\|IDistributedCache\|cache\.set\|cache\.get" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.yml" --include="*.yaml" -i .
```

### 1.3 - Detect Vector Store / Search Implementations

```bash
# Vector store providers
grep -rn "pinecone\|weaviate\|qdrant\|milvus\|chromadb\|chroma\|pgvector\|opensearch.*vector\|elasticsearch.*vector\|faiss\|annoy\|lancedb" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.toml" --include="*.txt" --include="*.json" \
  --include="*.lock" -i .

# Vector search commands (RediSearch / VSS)
grep -rn "FT\.CREATE\|FT\.SEARCH\|FT\.AGGREGATE\|HNSW\|FLAT.*VECTOR\|VectorField\|SearchIndex" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.yml" --include="*.yaml" -i .

# LLM framework patterns
grep -rn "VectorStore\|vectorstore\|vector_store\|similarity_search\|add_documents\|as_retriever" \
  --include="*.py" --include="*.ts" --include="*.js" -i .
```

### 1.4 - Identify Commands in Use

If Redis/Valkey usage was found, extract the specific commands:

```bash
# Common command patterns
grep -rn "\.get\|\.set\|\.hset\|\.hget\|\.lpush\|\.rpush\|\.zadd\|\.expire\|\.ttl\|\.publish\|\.subscribe\|\.xadd\|\.xread\|\.eval\|\.evalsha\|\.cluster\|\.sentinel" \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" --include="*.rb" -i . | grep -i "redis\|client\|conn" | head -50

# Module commands (Search, JSON, TimeSeries, Bloom)
grep -rn "FT\.\|JSON\.\|TS\.\|BF\.\|CF\.\|CMS\.\|TDIGEST\." \
  --include="*.py" --include="*.ts" --include="*.js" --include="*.java" \
  --include="*.go" -i .
```

### 1.5 - Framework and Language Context

```bash
find . -maxdepth 3 \( -name "package.json" -o -name "pyproject.toml" \
  -o -name "requirements*.txt" -o -name "setup.py" -o -name "go.mod" \
  -o -name "pom.xml" -o -name "build.gradle" \) | head -20
```

### 1.6 - Understand the Project's Extension Model

**Critical for gateway/proxy/router/framework projects.** Identify how the project registers new integrations, providers, plugins, or backends:

1. Look at the registration pattern for existing similar integrations (e.g., how Qdrant, Milvus, Ollama, or other providers are added).
2. Determine whether the project supports:
   - **HTTP proxy passthrough** (forward requests to an upstream URL)
   - **Custom request handlers** (intercept requests and execute custom logic, returning responses directly)
   - **Pluggable backends** (interface-based backends selected by configuration)
3. Read the provider/plugin index file to understand the structural template for adding a new integration.

This step determines whether Valkey can become a **first-class routable target** in the project's domain model (not just infrastructure). If the project routes to vector databases (Qdrant, Milvus, Pinecone) via HTTP, and Valkey Search has no HTTP API (it uses RESP/FT.* commands), a custom request handler that translates HTTP requests into GLIDE calls is the integration pattern.

Record: extension mechanism, existing similar providers, whether custom (non-HTTP) handlers are supported.

### 1.7 - Record Phase 1 Findings

Write to `/tmp/valkey_spike_scan.json`:

```json
{
  "redis_valkey_found": true,
  "redis_files": ["path:line"],
  "redis_commands_used": ["GET", "SET", "HSET", "EXPIRE"],
  "module_commands_used": [],
  "caching_backends": [],
  "vector_store_providers": ["qdrant", "milvus"],
  "vector_search_in_redis": false,
  "framework": "raw",
  "language": "typescript",
  "extension_model": {
    "type": "provider registry with HTTP proxy + custom request handlers",
    "similar_providers": ["qdrant (HTTP proxy)", "milvus (HTTP proxy)"],
    "custom_handler_support": true,
    "registration_file": "src/providers/index.ts"
  }
}
```

---

## Phase 2: Compatibility & Value Analysis

Read `/tmp/valkey_spike_scan.json` before starting.

### 2.1 - Command Compatibility Check

If Redis usage was found, verify all detected commands are supported in Valkey.

**Valkey supports all core Redis commands.** Flag only if:
- Redis Stack module commands are used (RediSearch `FT.*`, RedisJSON `JSON.*`, RedisTimeSeries `TS.*`, RedisBloom `BF.*`/`CF.*`) requiring Valkey equivalents
- Enterprise-only features are used (Active-Active geo-replication, RedisGears)
- Very new Redis 7.4+ commands not yet in Valkey are used

### 2.2 - Value Gap Analysis

Answer honestly:

**If the project is a gateway, proxy, router, or framework that routes to external services:**
- Can Valkey become a **routable target/provider** using the same pattern as existing integrations (Qdrant, Milvus, Pinecone, etc.)?
- If existing vector DB providers use HTTP proxy passthrough, and Valkey Search uses RESP (no HTTP API), can a custom request handler bridge the gap by translating HTTP-shaped requests into GLIDE/FT.* commands?
- This is often the highest-impact opportunity: making Valkey a first-class provider that customers can route to, not just infrastructure under the hood.

**If Redis is already present and working:**
- What NEW capabilities can we bring beyond "switch your dependency"?
- Is there a vector search story? Can we add `FT.CREATE`/`FT.SEARCH`?
- Is there a caching improvement (smarter eviction, client-side caching, cluster mode)?

**If no Redis but a caching backend exists:**
- Can Valkey replace or supplement the existing cache with better performance?
- Is the cache pluggable (Django CACHES, Spring CacheManager)?

**If neither Redis nor caching exists:**
- What latency-sensitive operations would benefit from an in-memory store?
- Is there a vector search / RAG pipeline that needs low-latency retrieval?

### 2.3 - Identify Riskiest Assumptions

Before proceeding to Phase 3, explicitly list the 3-5 assumptions that MUST be true for the integration to work but CANNOT be confirmed by reading code alone. Examples:

- "GLIDE native module loads on the target platform/runtime"
- "FT.SEARCH KNN returns correct results when vectors are passed as Buffer"
- "The `client.keys()` method exists and behaves like ioredis"
- "Lua script caching via `new Script()` + `invokeScript()` works"
- "SET with expiry option uses the expected syntax"
- "The Valkey search module is available on the target deployment"
- "GlideFt.create / GlideFt.search typed methods work for the index schema needed"

These become the test cases for Phase 3.

### 2.4 - PR Acceptance Research

Research whether the target repository would accept a Valkey contribution:

1. Search recent PRs/issues for: new integrations merged, Redis/Valkey mentions
2. Check for closed-without-merge PRs attempting similar integrations
3. Check `CONTRIBUTING.md`, PR templates, required CI checks
4. Look for open issues requesting Valkey/Redis support

---

## Phase 3: Empirical Verification (The Spike)

This is what separates analysis from proof. For each risky assumption from 2.3, write and run a minimal test against a live Valkey instance.

### 3.1 - Prerequisites Check

```bash
# Confirm tooling
echo "node: $(node -v 2>&1)"; echo "python: $(python3 --version 2>&1)"
echo "docker: $(docker -v 2>&1)"

# Find a running Valkey instance (check common ports)
for port in 6379 8888; do
  (echo PING | nc -w 1 127.0.0.1 $port 2>/dev/null) && echo "Valkey on :$port"
done

# If no Valkey running, start one with the search module
# docker run -d --name valkey-spike -p 6379:6379 valkey/valkey-bundle:8.1
```

If no Valkey is available and Docker is not running, note this as a blocker and skip to Phase 4 with "UNVERIFIED" status on risky assumptions.

### 3.2 - Probe Valkey Capabilities

```bash
# Check version and loaded modules
printf 'INFO server\r\nMODULE LIST\r\n' | nc -w 3 127.0.0.1 <PORT> | head -40
```

Record: Valkey version, loaded modules (search, bf, json, etc.). If the integration requires vector search and the `search` module is not loaded, note it.

### 3.3 - Install GLIDE and Verify Exports

Create an isolated scratch directory (NEVER inside the target repo):

```bash
mkdir -p /tmp/valkey-spike && cd /tmp/valkey-spike
```

Install the GLIDE package for the target language:

| Language | Package | Command |
|----------|---------|---------|
| Node.js/TS | `@valkey/valkey-glide` | `npm init -y && npm install @valkey/valkey-glide` |
| Python | `valkey-glide` | `python3 -m venv .venv && .venv/bin/pip install valkey-glide` |
| Java | `io.valkey:valkey-glide` | Create minimal pom.xml / build.gradle |
| Go | `github.com/valkey-io/valkey-glide/go/v2` | `go mod init spike && go get ...` |

Verify the expected exports/classes load without error. If the native module fails to load, this is a hard blocker for the integration.

### 3.4 - Write the Spike Script

Write a single script in `/tmp/valkey-spike/` that exercises each risky assumption from 2.3. Structure:

```
1. Connect to Valkey
2. For each risky assumption:
   a. Attempt the exact API call the integration would use
   b. Print PASS or FAIL with the error
3. Clean up test keys
4. Disconnect
```

**Rules for the spike script:**
- **Before writing the script**, read the GLIDE skill reference for the target language. Prefer typed module methods (`GlideFt.create`, `GlideFt.search`, `GlideJson`) over `customCommand` when available.
- Test the EXACT calls the integration needs, not simplified versions
- For binary data (vectors): pass raw `Buffer` (Float32Array backed), NEVER `Buffer.toString()`. Test with `Decoder.Bytes` for reading binary fields back.
- For command mapping (e.g., ioredis -> GLIDE): test each command the integration would use
- For commands with production concerns (KEYS blocks, large SMEMBERS): also test the production-safe alternative (SCAN, SSCAN) and document which to use in production
- Clean up all test keys at the end

### 3.5 - Run and Record Results

Run the spike script. Record each assumption as:
- **PASS** - works as expected
- **FAIL (fixable)** - doesn't work as written, but a clear alternative exists (document it)
- **FAIL (blocker)** - fundamental incompatibility, no workaround

For each result, also note:
- **Structural implications**: Does the API shape difference require changes beyond the immediate integration file? (e.g., async initialization requiring startup code changes, different error types requiring upstream catch updates)
- **Traps avoided**: Non-obvious correctness requirements discovered during the spike, even if the test passed (e.g., "vectors must be raw Buffer, not toString('binary')" or "KNN results are pre-sorted, SORTBY on the alias field errors")

Update `/tmp/valkey_spike_scan.json` with results:

```json
{
  "spike_results": {
    "glide_native_loads": "PASS",
    "vector_search_knn": "FAIL (fixable) - must use GlideFt.search + Buffer, not customCommand + toString",
    "keys_method": "FAIL (fixable) - use scan() or customCommand(['KEYS', pattern])",
    "script_invoke": "PASS",
    "set_with_ttl": "PASS"
  },
  "structural_implications": [
    "GlideClient.createClient is async (unlike ioredis sync/lazy) - startup code must await"
  ],
  "traps": [
    "Float32 vectors must be raw Buffer, not Buffer.toString('binary') - stringifying corrupts payload",
    "KNN results are pre-sorted by distance - SORTBY on the AS alias field errors"
  ],
  "valkey_version": "9.1.0",
  "modules_loaded": ["search", "bf"],
  "blockers": []
}
```

### 3.6 - Cleanup

```bash
rm -rf /tmp/valkey-spike
```

---

## Phase 4: Synthesis

Read `/tmp/valkey_spike_scan.json` (with spike results) and produce the final assessment.

### Compound Opportunity Analysis

Before writing the output, evaluate whether multiple opportunities can be combined into a single cohesive integration. Ask:
- Can a shared dependency serve both roles (e.g., GLIDE for cache backend AND vector search provider)?
- Can shared client infrastructure (singleton connection, config parsing) reduce marginal effort?
- Is the combined narrative stronger than any single opportunity? (e.g., "one Valkey instance handles caching + vector search + rate limiting" is more compelling than "swap your Redis client")

### Output Format

```
# Valkey Integration Assessment
Repository: <path>
Language: <detected>    Framework: <detected>

## 1. Current State

<2-3 paragraphs describing what the project currently does for caching,
data storage, and search. Specific about files and libraries found.>

## 2. Valkey Compatibility

Status: FULLY COMPATIBLE | MOSTLY COMPATIBLE (issues listed) | NO EXISTING USAGE

<Command compatibility issues if any. If none, state drop-in replacement.>

## 3. Value Proposition

<3-5 paragraphs answering: What does this project get from Valkey that it
doesn't already have? Be honest. If the answer is "not much", say that.>

### Verdict

ONE OF:
- **STRONG VALUE-ADD**: Clear new capability (vector search provider, replacing expensive vector DB, new routable target)
- **MODERATE VALUE-ADD**: Useful but incremental (adding caching, pluggable backend)
- **COMPATIBILITY ONLY**: Works but doesn't add new value
- **NOT A FIT**: Project doesn't benefit from an in-memory store

## 4. Spike Results

| Assumption | Result | Notes |
|-----------|--------|-------|
| <assumption from 2.3> | PASS / FAIL (fixable) / FAIL (blocker) | <one line> |
| ... | ... | ... |

<If any FAIL (fixable): describe the correct approach that was proven to work.>
<If any FAIL (blocker): explain why the integration cannot proceed as designed.>

### Structural Implications
<List API shape differences that require changes beyond the backend file itself.>

### Implementation Traps
<Non-obvious correctness requirements discovered during the spike.>

## 5. Opportunity Map

| Opportunity | Feasibility | Impact | Spike Status |
|-------------|-------------|--------|--------------|
| Vector search provider | H/M/L | H/M/L | Proven / Unverified / Blocked |
| Cache backend | H/M/L | H/M/L | Proven / Unverified / Blocked |
| Rate limiting | H/M/L | H/M/L | Proven / Unverified / Blocked |
| Session management | H/M/L | H/M/L | Proven / Unverified / Blocked |

<If multiple opportunities compound, note: "Combined integration recommended -
shared GLIDE dependency serves both X and Y with minimal marginal effort.">

## 6. PR Strategy

Confidence: HIGH | MEDIUM | LOW

<How to frame the PR. What angle resonates with maintainers.
What the spike proved that gives confidence in the approach.>

## 7. Next Steps

<Ordered list: file issue, write narrative, implement, etc.
Include any corrections discovered during the spike that must be
incorporated into the implementation.>
```

---

## Decision Rules

- If verdict is **NOT A FIT** or all spike results are **FAIL (blocker)**: stop after Phase 4 output. Do not recommend proceeding.
- If verdict is **COMPATIBILITY ONLY**: recommend only if there's a clear vector search or new-capability angle.
- If no Valkey instance is available for Phase 3: output the assessment with "UNVERIFIED" spike status and note that empirical verification is needed before implementation.
- If Phase 3 reveals the planned API approach is wrong but a working alternative exists: document both the broken approach and the working one. The working approach becomes the implementation spec.
