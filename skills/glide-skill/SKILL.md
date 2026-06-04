---
name: glide
description: Production-ready patterns for Valkey GLIDE clients across 6 languages. Activate when generating, reviewing, or debugging Valkey GLIDE code including client creation, batch/pipeline/transaction operations, clustering, authentication, TLS, error handling, and timeout configuration.
---

**Based on valkey-glide v2.4.0**

## ⚠️ CRITICAL: Version Verification

**BEFORE writing or reviewing any GLIDE code, determine the target GLIDE version.**

Detect version from: `package.json` (`@valkey/valkey-glide`), `requirements.txt`/`pyproject.toml` (`valkey-glide`), `pom.xml`/`build.gradle` (`io.valkey:valkey-glide`), `go.mod` (`valkey-glide/go`), `composer.json`, or `.csproj` (`Valkey.Glide`). If no version is pinned, ask the user.

**Version mismatch rules:**

| Situation | Action |
|-----------|--------|
| Target version **> v2.4.0** (newer than this skill) | **STOP.** Tell the operator: "The project uses GLIDE vX.Y.Z but this skill only covers up to v2.4.0. The GLIDE skill must be updated before I can reliably generate or review code for this version." Do not proceed until the skill is updated. |
| Target version **= v2.4.0** | Proceed normally using this skill's guidance. |
| Target version **< v2.4.0** (older) | Proceed with caution. Note that APIs may differ — the Batch API replaced Transaction/ClusterTransaction in v2.x, older versions use different class names and method signatures. Research the specific version's API via changelogs or docs before generating code. Flag any guidance from this skill that may not apply to the older version. |

## Language-Specific Guides
**Activation Triggers** - Use this skill when:
- User mentions "Valkey", "GLIDE", or "Valkey GLIDE"
- User asks about Redis/Valkey client libraries
- User needs help with caching, key-value storage, or in-memory databases
- User is implementing batch operations, pipelines, or transactions
- User is working with cluster mode or distributed caching
- User encounters errors like CROSSSLOT, connection timeouts, or resource leaks
- User asks about async patterns for database operations
- Building REST APIs with caching layers
- Implementing session storage for web applications
- Creating batch processing pipelines
- Developing microservices with distributed caching
- Migrating from Redis to Valkey
- Setting up cluster mode for high availability
- Implementing vector search for AI/ML applications

### ⚠️ CRITICAL: Language-Specific Guide Loading
**BEFORE generating any code, you MUST:**
1. Detect the language from file extension or imports
2. Load the corresponding language-specific guide using readFile
3. Confirm you've loaded it by mentioning which guide you loaded

**This is a BLOCKING REQUIREMENT - do not proceed with implementation until the language-specific guide is loaded.**

Load the corresponding guide when generating or reviewing code:

| Language/Framework | Reference File | Key Topics |
|-------------------|----------------|------------|
| **Python** | [Python-specific skill](references/python.md) | Mutable Default Arguments, Exception Handling, Class Attributes, Client Lifecycle |
| **Java** | [Java-specific skill](references/java.md) | CompletableFuture Patterns, Exception Unwrapping, GlideString for Binary Data, Client Lifecycle |
| **Go** | [Go-specific skill](references/go.md) | Context Pattern, Explicit Error Handling, Batch Pointer Dereferencing, Client Lifecycle |
| **Node.js** | [Node.js-specific skill](references/nodejs.md) | Promise-Based API, Decoder.Bytes for Binary Data, Static FT Methods, Client Lifecycle |
| **PHP** | [PHP-specific skill](references/php.md) | C Extension, PHPRedis Compatibility, Synchronous API, multi()/pipeline(), Client Lifecycle |
| **C#** | [C#-specific skill](references/csharp.md) | Task-Based Async, await using Pattern, CustomCommand for FT Module, Client Lifecycle |

Detect language via file extension (`.js`/`.ts`, `.py`, `.java`, `.go`, `.php`, `.cs`) or import (`@valkey/valkey-glide`, `from glide import`, `import glide.api.*`, `valkey-glide/go`, `use ValkeyGlide`, `using Valkey.Glide`) and load the matching guide.

### General Principles
1. Catch GLIDE-specific exceptions, not general catch-alls.
2. Comment to disambiguate sync vs async calls when not obvious from syntax.

---
## Timeout Configuration

- **Connection timeout**: Time to establish connection (typically 2s)
- **Request timeout**: Time per command (typically 250ms)

Increase timeouts for: large batches (>1000 keys), vector search, blocking ops (BLPOP/BRPOP with timeout), high-latency networks, cross-node cluster ops, large values (>1MB).

**Timeout exception types:** Python `TimeoutError`, Java `TimeoutException`, Go error type check, Node.js error message check, PHP exception message check, C# `TimeoutException`.

**Production handling:**
1. Catch timeout-specific exceptions (not generic)
2. Log: operation name, key(s), configured timeout, duration
3. Fallback strategies: cache miss → DB; cache write → log+continue; critical read → retry (max 2-3); batch → partial retry
4. Emit metrics; alert if >1% timeout rate
5. Retry on transient issues; fail-fast on strict SLA; circuit-break after N consecutive timeouts

---
## ⚠️ CRITICAL: Package Selection

**Use Valkey GLIDE** - the official recommended client with better performance and active development.

| Language | Package | Installation | Notes |
|----------|---------|--------------|-------|
| **Python** | `valkey-glide` (async)<br>`valkey-glide-sync` (sync) | `pip install valkey-glide` | Use `glide_shared` for shared types |
| **Java** | `io.valkey:valkey-glide` | Maven/Gradle with platform classifier | Requires `os-maven-plugin` or `osdetector` |
| **Go** | `github.com/valkey-io/valkey-glide/go/v2` | `go get` | Requires Go 1.22+ |
| **Node.js** | `@valkey/valkey-glide` | `npm install` | Promise-based API |
| **PHP** | `valkey_glide` extension | PECL/pie/source | C extension, not Composer package |
| **C#** | `Valkey.Glide` | `dotnet add package` | Current: v0.9.0, v2.0+ for IAM/TLS |

**Platform support:** Java requires native binaries (linux-x86_64, linux-aarch_64, osx-x86_64, osx-aarch_64, windows-x86_64). PHP and C# also use native components.

**Compatibility layers:** PHP provides PHPRedis compatibility (`ValkeyGlide::registerPHPRedisAliases()`), C# provides StackExchange.Redis compatibility.

**❌ Don't use:** Redis forks (`valkey` Python package, `jedis`/`lettuce` Java, `go-redis` Go, `redis` PHP extension, `StackExchange.Redis` C# without compatibility layer).

---
## Batch Commands (Pipeline and Transaction)

**Batch API** replaces deprecated Transaction/ClusterTransaction APIs. Two modes:

**Atomic Batch (Transaction):**
- All commands execute as single atomic unit (MULTI/EXEC)
- Sequential execution, no interleaving
- **Cluster constraint:** All keys must map to same hash slot
- Use case: Consistency and isolation required

**Non-Atomic Batch (Pipeline):**
- Commands sent in single request, no atomicity
- Can span multiple slots/nodes in cluster
- Other operations may interleave
- Use case: Bulk independent operations

**Classes:**
- `Batch` / `StandaloneBatch` - Standalone mode
- `ClusterBatch` - Cluster mode
- Constructor: `Batch(isAtomic: bool)` or `ClusterBatch(isAtomic: bool)`

**Execution:**
```
client.exec(batch, raiseOnError, options?)
```

**Error Handling (`raiseOnError`):**
- `true`: Raises first error as exception
- `false`: Returns errors in result array at corresponding positions

**Options:**

`BatchOptions` (Standalone):
- `timeout`: Max wait time (ms)

`ClusterBatchOptions`:
- `timeout`: Max wait time (ms)
- `retryStrategy`: Retry config (non-atomic only)
  - `retryServerError`: Retry on TRYAGAIN (may reorder)
  - `retryConnectionError`: Retry entire batch (may duplicate)
- `route`: Single-node routing

**Retry Strategy Decision Matrix:**

| `retryServerError` | `retryConnectionError` | When | Trade-off |
|---|---|---|---|
| ✅ | ❌ | Resharding, TRYAGAIN, transient OOM | May reorder commands |
| ❌ | ✅ | Network instability, node failover, pool exhaustion | May duplicate entire batch |
| ✅ | ✅ | High availability + idempotent ops (SET, not INCR) | Reorder + duplication possible |
| ❌ | ❌ | SLA-bound, non-idempotent (INCR/LPUSH), app-level retry | Fail fast on any error |

**Multi-Node Support (Cluster Pipeline):**
- GLIDE splits pipeline into sub-pipelines per node
- Dispatches independently, reassembles responses in order
- Redirection errors (MOVED/ASK) always handled automatically
- Retry strategies apply per command, not all-or-nothing

**Deprecation:**
- `Transaction` → `Batch(true)`
- `ClusterTransaction` → `ClusterBatch(true)`

---
## Common Patterns Across Languages

### Client Creation
- **Always set explicit timeouts** to avoid connection issues
- **Use language-specific cleanup patterns** (try-with-resources, async with, defer, await using)
- **Configure retry strategies** for production resilience

### Batch Operations
- **Atomic batches** (transactions) require same hash slot in cluster mode
- **Non-atomic pipelines** can span multiple slots
- **Use `raiseOnError`/`raise_on_error`** to control error handling

### Cluster Operations
- **Use hash tags** `{tag}` to control slot assignment
- **CROSSSLOT errors** occur when atomic operations span slots
- **Non-atomic pipelines** automatically route to correct nodes

### Error Handling
- **Use specific exception types** (ConnectionException, TimeoutException, RequestException)
- **Don't swallow errors** - log or propagate appropriately
- **Unwrap exceptions** correctly (Java's ExecutionException.getCause())

---

## Client Lifecycle Management

One client per application (or per distinct cluster). GLIDE multiplexes over a single connection — it is thread/goroutine-safe and reconnects automatically. See each language guide for singleton and shutdown patterns.

- **Singleton:** Python: module-level var + `lifespan`; Java: `static` field or `@Bean`; Go: package-level var + `defer client.Close()`; Node.js: module-level `let` + `process.on('SIGTERM')`; PHP: `global`/static per FPM worker; C#: `AddSingleton` or `IHostedService`
- **Shutdown:** call `close()` / `DisposeAsync()` on exit to flush in-flight requests
- **Reconnection:** do NOT recreate on `RequestException`/`TimeoutException` — only recreate on `ClosingError` (client was explicitly closed)

---

## Performance Optimization

Each language guide has a Performance Optimization section.  See language-specific guide for location of templates.

Key patterns: AZ Affinity (>80% reads), `inflightRequestsLimit` tuning (Node.js/Python/Java), `lazyConnect` for serverless, dedicated blocking client, cluster scan, SCAN vs FT.* search, Hash vs JSON strings, OpenTelemetry, log levels (warn/error), concurrent patterns (asyncio.gather/Promise.all/CompletableFuture/goroutines). Client is thread-safe; Batch objects are NOT.

### Universal Anti-Patterns

1. **Per-request client creation** — create once at startup, reuse everywhere
2. **Missing request timeouts** — always configure (500ms for web apps)
3. **Sequential operations** — use batching (10-100 commands) or concurrent execution or `MGET` in lieu of many `GET`s
4. **Blocking commands on shared client** — use dedicated client with longer timeout
5. **Large batch sizes** — keep under 1000 commands, optimal 10-100
6. **Missing error handling & retries** — configure exponential backoff
7. **No TTL strategy** — add TTLs with jitter to prevent thundering herd
8. **Hot key** - single key with disproportionate traffic saturates one shard/node; shard it (`counter:{shard_N}`), use read replicas, or add a local cache layer
9. **Oversized values** — compress >10KB, split >100KB

### Server Configuration

For infrastructure guidance (cluster sizing, memory policy, ElastiCache node types, monitoring):
→ See [`references/server-configuration-guide.md`](references/server-configuration-guide.md)

### Performance Checklist

- [ ] Client reuse (not per-request)
- [ ] Request timeout configured (500ms recommended)
- [ ] Batching for bulk ops (10-100 commands)
- [ ] AZ Affinity for read-heavy (>80% reads)
- [ ] Connection backoff configured
- [ ] Concurrent patterns where appropriate
- [ ] Error handling with retry strategy
- [ ] Hash data structures for structured data
- [ ] Values <100KB
- [ ] Hash tags for related keys in cluster
- [ ] Dedicated client for blocking commands
- [ ] lazyConnect for serverless/Lambda
- [ ] OpenTelemetry enabled for monitoring
- [ ] Logging set to warn/error for production

## Valkey Module Detection

Detect module usage via command patterns and provide optimization guidance.

### Supported Modules
- **Valkey-Search** (`FT.*`): Full-text search, secondary indexing
- **Valkey-JSON** (`JSON.*`): Native JSON document storage
- **Valkey-BloomFilter** (`BF.*`, `CF.*`, `CMS.*`, `TOPK.*`): Probabilistic data structures

### Module Anti-Patterns

**Valkey-Search**: Missing index definitions before queries; wildcard prefix searches; no pagination; not using `FT.AGGREGATE` for aggregations; missing `LOAD` clause in `FT.AGGREGATE` — reducers (SUM, AVG, MIN, MAX) silently return 0 without explicit `LOAD` of the fields they operate on (COUNT is the exception); `FT.AGGREGATE` rejects wildcard `*` query (use a field filter like `@price:[0 inf]` instead); `FT.SEARCH` rejects `*` after a filter expression (use the filter alone for match-all); auto-detecting search mode (semantic vs text) based on provider availability without allowing an explicit `mode` override — user intent must take priority over auto-detection; **query injection via `=>`** — the `=>` token in FT.SEARCH syntax delimits a filter from a KNN vector clause; user-controlled input interpolated into query strings without sanitization allows an attacker to inject `*=>[KNN ...]` and bypass all filters, converting a text search into a vector search that returns all documents. **Always reject `=>` in user-supplied filter expressions and vector field names before constructing queries.**

**Valkey-JSON**: Full document `JSON.GET` instead of path-based queries; not using `JSON.MGET` for batching; documents >100KB without splitting; missing `JSON.NUMINCRBY` for atomic updates; skipping `json.dumps()` encoding for `JSON.SET` sub-path values (all paths require JSON-encoded values); calling `JSON.ARRPOP`/`JSON.ARRTRIM`/`JSON.ARRAPPEND` without pre-validating key existence and type.

**Valkey-BloomFilter**: Wrong false-positive rate; undersized initial capacity; not using Cuckoo Filters (`CF.*`) when deletions needed; sequential `BF.ADD` instead of `BF.MADD`.

### Safe vs Unsafe Operations

Operations that raise `RequestError` can crash MCP/server framework transports. Pre-validate before calling unsafe operations.

| Operation | Safe? | Notes |
|-----------|-------|-------|
| `ft.list(client)` | ✅ Safe | Always returns a list |
| `ft.search(client, idx, ...)` | ⚠️ Pre-validate | Raises if index doesn't exist |
| `ft.info(client, idx)` | ⚠️ Pre-validate | Raises if index doesn't exist |
| `ft.create(client, idx, ...)` | ⚠️ Pre-validate | Raises if index already exists |
| `ft.dropindex(client, idx)` | ⚠️ Pre-validate | Raises if index doesn't exist |
| `FT.AGGREGATE` via custom_command | ⚠️ Pre-validate | Raises on invalid query (e.g., `*`) |
| `client.exists([key])` | ✅ Safe | Returns 0 or 1 |
| `client.hget(key, field)` | ✅ Safe | Returns None if missing |
| `JSON.GET` | ✅ Safe | Returns None if missing |
| `JSON.SET` | ✅ Safe | Creates key if missing |
| `JSON.ARRPOP` / `JSON.ARRTRIM` | ⚠️ Pre-validate | Raises on non-existent key |
| `JSON.ARRAPPEND` | ⚠️ Pre-validate | Raises on non-array target |

### Pattern-to-Module Recommendations
- `GET` + JSON parse + modify + `SET` → use `JSON.SET` with path syntax
- `SCAN` + pattern matching for search → use `FT.CREATE` index + `FT.SEARCH`
- Large `SISMEMBER`/`SMEMBERS` + filtering → use `BF.EXISTS` for probabilistic membership

### Module Configuration
**Search**: Use appropriate field types (TEXT, NUMERIC, TAG, GEO). Set `STOPWORDS`, `MAXPREFIXEXPANSIONS`. Use `SORTBY` with indexed fields.

**JSON**: Configure `json-max-size`. Use path-based operations. Use `JSON.FORGET` to remove unused paths.

**BloomFilter**: Calculate capacity from expected cardinality. Error rate: 0.01 general, 0.001 critical. Pre-allocate with `BF.RESERVE`.
