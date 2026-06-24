---
name: test-valkey
description: Test Valkey integration code against a live valkey-bundle container. Starts Valkey from a clean slate, exercises staged/unstaged git changes (cache backend, vector search provider, rate limiter, connection parsing), validates all configurations by tearing down and recreating the container as needed. Use when Valkey integration code has been written and needs end-to-end verification before commit.
---

# Test Valkey

Run integration tests against the staged/unstaged Valkey changes in the current repository. Proves correctness by exercising the code against a real `valkey/valkey-bundle` container.

---

## Prerequisites

Before starting, verify:

```bash
# Container runtime
podman -v 2>/dev/null || docker -v 2>/dev/null

# Language runtime (detect from project)
node -v 2>/dev/null; python3 --version 2>/dev/null; go version 2>/dev/null; java -version 2>/dev/null
```

If neither Podman nor Docker is available, stop and report the blocker.

---

## Phase 1: Identify What to Test

### 1.1 - Scan Git Changes

```bash
# Staged changes
git diff --cached --name-only | grep -i valkey

# Unstaged changes
git diff --name-only | grep -i valkey
```

### 1.2 - Classify Components

For each changed file, classify into test categories:

| Category | Pattern examples | What to verify |
|----------|-----------------|----------------|
| Connection/Client | `**/valkey/client.*`, `**/connection.*` | Connection string parsing, TLS flag, password auth, error on invalid input |
| Cache Backend | `**/cache/**valkey*`, `**/backends/valkey*` | get/set/delete/clear/has/keys/TTL expiry/cleanup |
| Vector Search | `**/search*valkey*`, `**/valkey*search*` | FT.CREATE, FT.SEARCH KNN, HSET with binary vectors, FT.DROPINDEX, FT.INFO |
| Rate Limiter | `**/rate*limit*`, `**/glide*rate*` | Lua script execution, token bucket logic, refill timing |
| Session/State | `**/session*`, `**/state*` | Session storage, expiry, concurrent access |
| Pub/Sub | `**/pubsub*`, `**/events*` | Publish, subscribe, message delivery |
| Startup/Init | Entry points referencing Valkey env vars | Env var triggers init, backends injected correctly |

Only test categories that have changed files. Skip categories with no relevant changes.

---

## Phase 2: Container Lifecycle

### 2.1 - Clean Start

```bash
RUNTIME=$(command -v podman 2>/dev/null || command -v docker)
CONTAINER_NAME="valkey-test"
PORT=6379

# Tear down any existing test container
$RUNTIME rm -f $CONTAINER_NAME 2>/dev/null

# Start fresh valkey-bundle (includes search module)
$RUNTIME run -d --name $CONTAINER_NAME -p $PORT:6379 valkey/valkey-bundle:latest

# Wait for readiness
for i in $(seq 1 30); do
  (echo PING | nc -w 1 127.0.0.1 $PORT 2>/dev/null | grep -q PONG) && break
  sleep 0.5
done

# Verify search module loaded
printf 'MODULE LIST\r\n' | nc -w 2 127.0.0.1 $PORT | grep -qi search \
  && echo "search module: OK" \
  || echo "WARN: search module not found"
```

### 2.2 - Teardown Helper

Use between configuration tests to ensure clean state:

```bash
valkey_reset() {
  $RUNTIME rm -f $CONTAINER_NAME 2>/dev/null
  $RUNTIME run -d --name $CONTAINER_NAME -p $PORT:6379 valkey/valkey-bundle:latest
  for i in $(seq 1 30); do
    (echo PING | nc -w 1 127.0.0.1 $PORT 2>/dev/null | grep -q PONG) && break
    sleep 0.5
  done
}
```

### 2.3 - Password-Protected Configuration

Test that password auth works by restarting with `--requirepass`:

```bash
$RUNTIME rm -f $CONTAINER_NAME 2>/dev/null
$RUNTIME run -d --name $CONTAINER_NAME -p $PORT:6379 \
  valkey/valkey-bundle:latest valkey-server --requirepass testpass123

# Verify unauthenticated access is rejected
printf 'PING\r\n' | nc -w 2 127.0.0.1 $PORT | grep -q NOAUTH \
  && echo "auth required: OK"

# Verify authenticated access works
printf 'AUTH testpass123\r\nPING\r\n' | nc -w 2 127.0.0.1 $PORT | grep -q PONG \
  && echo "auth with password: OK"
```

---

## Phase 3: Test Execution

Create test scripts in `/tmp/valkey-test/` (never inside the target repo). Adapt to the project's language and GLIDE client library.

### 3.1 - Connection/Client Tests

**Unit tests (no container needed):**
- Parse valid connection strings: `valkey://host:port`, `valkeys://` (TLS), `redis://` (compat), `rediss://` (TLS)
- Extract password from URL: `valkey://:secret@host:port`
- Default port when omitted: `valkey://host` -> port 6379
- Reject invalid schemes, malformed URLs, missing host

**Integration tests (container required):**
- Connect to running Valkey, execute PING
- Connect with password auth (after container restart with `--requirepass`)
- Reject connection without password when auth is required
- Timeout on unreachable host

### 3.2 - Cache Backend Tests

Test against live Valkey:

```
1. Connect and create cache backend instance
2. set() -> get() round-trip (verify value matches)
3. set() with TTL -> wait past TTL -> get() returns null
4. delete() -> get() returns null
5. has() returns true for existing key, false for missing
6. keys() returns only keys in the target namespace
7. clear() removes all keys in namespace but not other namespaces
8. Stats tracking: hits/misses/sets/deletes increment correctly
9. close() disconnects cleanly without error
```

### 3.3 - Vector Search Tests

Test the full lifecycle against valkey-bundle (requires search module):

```
1. Connect to Valkey
2. Create index: VECTOR(HNSW, N dims, COSINE) + TEXT + TAG fields
3. Verify duplicate index creation is rejected (conflict/409)
4. Get index info: verify metadata returned
5. Upsert documents with vector + text + tag fields
6. KNN search: verify correct number of results returned
7. Filtered search: TAG/NUMERIC filter, verify filtered results
8. Filter injection guard: filter containing "=>" must be rejected
9. Delete documents: verify removed from search results
10. Drop index: verify info/search on dropped index fails
11. Input validation: invalid index names, empty vectors, invalid doc IDs
```

**Critical correctness checks:**
- Vectors must be stored as raw bytes (e.g. `Buffer.from(new Float32Array([...]).buffer)`) - NEVER stringified
- KNN query format: `(filter)=>[KNN k @vector $BLOB AS __score]`
- Use byte decoder when reading fields that may contain binary data

### 3.4 - Rate Limiter Tests

Test Lua-based token bucket (or similar pattern):

```
1. Connect to Valkey
2. Create rate limiter instance with known capacity and window
3. Consume tokens up to capacity -> all return allowed=true
4. Consume one more -> returns allowed=false with waitTime > 0
5. Check-only mode (no consume) -> does not decrement tokens
6. Wait for partial refill -> verify tokens restored proportionally
7. Verify key TTL is set (keys expire after inactivity)
```

### 3.5 - Application Startup (E2E)

Test the application starts correctly with the Valkey connection env var:

```bash
# Identify the correct env var and startup command from the project
# Common patterns: VALKEY_CONNECTION_STRING, VALKEY_URL, REDIS_URL

# Start application with Valkey env, timeout after 10s
<ENV_VAR>=valkey://localhost:6379 timeout 10 <start_command> &
APP_PID=$!
sleep 3

# Verify application is responsive (health endpoint or similar)
curl -s http://localhost:<port>/health || echo "No health endpoint"

# Check for Valkey-related errors in output
kill $APP_PID 2>/dev/null
```

### 3.6 - Password Auth E2E

After restarting container with `--requirepass`:

```
1. Connect WITH correct password in connection string -> succeeds, can read/write
2. Connect WITHOUT password -> connection fails with auth error
3. Connect with WRONG password -> connection fails with auth error
```

---

## Phase 4: Configuration Matrix

Test each configuration by tearing down and recreating the container:

| Config | Container args | Connection string | What to verify |
|--------|---------------|-------------------|----------------|
| Default (no auth, no TLS) | (none) | `valkey://localhost:6379` | Basic connectivity, all operations work |
| Password auth | `--requirepass secret` | `valkey://:secret@localhost:6379` | Auth handshake, rejection without password |
| Custom port | `-p 7777:6379` | `valkey://localhost:7777` | Port parsing, connectivity on non-default port |
| Redis scheme compat | (none) | `redis://localhost:6379` | `redis://` scheme accepted, TLS=false |
| TLS scheme parsing | (unit test only) | N/A | `valkeys://` and `rediss://` set useTLS=true |

For each configuration:
1. `valkey_reset` (or recreate with new args)
2. Run the relevant subset of Phase 3 tests
3. Record PASS/FAIL

---

## Phase 5: Results and Cleanup

### 5.1 - Summary

Print a results table:

```
| Test Suite | Tests | Pass | Fail | Skip |
|-----------|-------|------|------|------|
| Connection/client | N | N | 0 | 0 |
| Cache backend | N | N | 0 | 0 |
| Vector search | N | N | 0 | 0 |
| Rate limiter | N | N | 0 | 0 |
| Startup E2E | N | N | 0 | 0 |
| Auth config | N | N | 0 | 0 |
```

### 5.2 - Cleanup

```bash
$RUNTIME rm -f $CONTAINER_NAME 2>/dev/null
rm -rf /tmp/valkey-test
```

### 5.3 - Verdict

- **ALL PASS**: Code is ready to commit. Report confidence level.
- **FAILURES in test logic only**: Fix the test and re-run. Do not modify source under test without explicit approval.
- **FAILURES in source code**: Report each failure with:
  - What was expected
  - What actually happened
  - The specific file and line causing the issue
  - Suggested fix (do not apply without approval)

---

## Rules

- NEVER modify the source code under test. Only create test scripts in `/tmp/valkey-test/`.
- ALWAYS start from a clean container state for each configuration test.
- ALWAYS clean up containers and temp files when done.
- If a test requires the search module and it is not available, use `valkey/valkey-bundle` (not `valkey/valkey`).
- Binary vectors: use raw byte buffers (e.g. Float32Array -> Buffer). Never stringify.
- Use byte decoders when reading fields that may contain binary data.
- Report exact error messages on failure, not summaries.
- Adapt test scripts to the project's language and GLIDE client library (Node.js, Python, Java, Go).
- Only test components that have actual changes in the git diff. Skip unchanged categories.
