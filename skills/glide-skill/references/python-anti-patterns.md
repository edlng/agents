# Python GLIDE Anti-Patterns

This document contains additional anti-patterns specific to Python GLIDE development. **Critical constraints (package selection, binary data handling, vector search) are documented in `python.md`.**

---

## Testing Patterns

### ❌ INCORRECT: Mocking at definition location
```python
# ❌ WRONG: Mocking at glide_sync module
@patch("glide_sync.GlideClient")  # Won't work if already imported elsewhere
```

### ✅ CORRECT: Mock at import location
```python
# ✅ CORRECT: Mock at the import location
@patch("org.someproject.utilities.valkey.GlideClient")
@patch("org.someproject.utilities.valkey.GlideClusterClient")
def test_something(mock_cluster, mock_client):
    # Mock the create() class method
    mock_client.create.return_value = MagicMock()
    ...
```

**Why:** When code does `from module import get_client`, the name is bound locally. Patching at the source module has no effect on already-imported references.

### ❌ INCORRECT: Using MagicMock for async functions
```python
# ❌ WRONG: MagicMock for an async function — await hangs indefinitely
with patch("tools.search_manage_index.get_client", return_value=client):
    result = await manage_index(...)  # HANGS — await on non-coroutine
```

### ✅ CORRECT: Use AsyncMock for async functions
```python
from unittest.mock import AsyncMock, patch

# ✅ CORRECT: AsyncMock returns a coroutine that await can resolve
mock = AsyncMock(return_value=client)
with patch("tools.search_manage_index.get_client", mock):
    result = await manage_index(...)  # Works
```

For multiple tool modules that each import the same async function, patch every import location:
```python
mock = AsyncMock(return_value=client)
with (
    patch("tools.search_manage_index.get_client", mock),
    patch("tools.search_add_documents.get_client", mock),
    patch("tools.search_query.get_client", mock),
):
    ...
```

**Why:** `MagicMock.__call__` returns another `MagicMock`, not a coroutine. When you `await` it, the event loop blocks forever. `AsyncMock` returns a proper coroutine.

---

### ❌ INCORRECT: Caching GLIDE client across async tests
```python
_client = None

@pytest.fixture()
async def client():
    global _client
    if _client is None:
        _client = await GlideClient.create(config)  # Created on test 1's loop
    yield _client  # Test 2 hangs — different event loop
```

### ✅ CORRECT: Fresh client per test
```python
@pytest.fixture()
async def client():
    c = await GlideClient.create(config)
    yield c
    await c.close()
```

**Why:** A `GlideClient` is bound to the event loop it was created on (Rust FFI/tokio runtime). In pytest with `asyncio_mode = "auto"`, each test function gets its own event loop by default. A client cached from a previous test's loop will hang when used on the current test's loop. The same applies to any async resource cached as a singleton (e.g., `httpx.AsyncClient` inside an embeddings provider).

---

### ❌ INCORRECT: Module-scoped async fixtures
```python
@pytest.fixture(scope="module")
async def client():
    c = await GlideClient.create(config)
    yield c
    await c.close()  # Deadlocks — fixture setup blocks the event loop
```

### ✅ CORRECT: Function-scoped async fixtures
```python
@pytest.fixture()
async def client():
    c = await GlideClient.create(config)
    yield c
    await c.close()
```

**Why:** `scope="module"` or `scope="session"` on async fixtures causes deadlocks with `pytest-asyncio` in `auto` mode (at least through version 0.26). The fixture setup blocks the event loop. Use function scope and manage caching yourself if needed.

---

## Query Injection via `=>` Delimiter

The `=>` token in FT.SEARCH syntax separates a filter expression from a KNN vector clause. When user-controlled input is interpolated into query strings without sanitization, an attacker can inject a KNN clause that bypasses all filters and returns all documents. In MCP server contexts, the attacker is an AI agent manipulated via prompt injection.

### ❌ VULNERABLE: Interpolating user input into query without sanitization
```python
# ❌ DANGEROUS — filter_expression comes from user/agent input
def build_query(filter_expression: str, query_text: str) -> str:
    if filter_expression and query_text.strip() == '*':
        return filter_expression  # ← attacker sends "*=>[KNN 9999 @embedding $vector AS score]"
    return f'({filter_expression}) {query_text}'

# Attacker bypasses year filter, gets ALL documents including secrets
```

### ✅ CORRECT: Reject `=>` in user-supplied input before query construction
```python
# ✅ SAFE — reject injection payload before building query
def build_query(filter_expression: str | None, query_text: str) -> str | dict:
    if filter_expression and '=>' in filter_expression:
        return {'status': 'error', 'reason': "filter must not contain '=>'"}
    if filter_expression and query_text.strip() == '*':
        return filter_expression
    if filter_expression:
        return f'({filter_expression}) {query_text}'
    return query_text
```

**Why:** The `=>` delimiter is not documented as security-sensitive and there is no built-in escaping in Valkey Search syntax. Any application that interpolates user input into FT.SEARCH queries must treat `=>` as a reserved token and reject it in user-controlled fragments (filter expressions, vector field names, query text). This applies to all languages, not just Python.

---

## MCP / Server Framework Patterns

GLIDE raises `RequestError` for Valkey errors. These are normal Python exceptions, but in MCP server frameworks (e.g., FastMCP), unhandled exceptions during concurrent tool calls can crash the entire transport (stdio pipe closes, server dies). The crash is in the framework's async dispatch, not in GLIDE's native layer.

**Defensive pattern:** Pre-validate all preconditions before calling GLIDE methods that may raise. Return structured error dicts instead of letting exceptions propagate.

### ❌ INCORRECT: Relying on try/except for control flow in MCP tools
```python
# ❌ RISKY — RequestError can crash MCP transport
try:
    await ft.info(client, index_name)
except RequestError:
    return {'status': 'error', 'reason': 'Index not found'}
```

### ✅ CORRECT: Pre-validate using safe operations
```python
# ✅ SAFE — ft.list() never raises
if not await index_exists(client, index_name):
    return {'status': 'error', 'reason': 'Index not found'}
await ft.info(client, index_name)  # Now safe
```

**Safe operations that never crash:**
- `ft.list(client)` — always returns a list
- `client.exists([key])` — always returns an int
- `client.hget(key, field)` — returns None if missing
- `client.custom_command(['JSON.GET', key, path])` — returns None if missing

**Operations that require pre-validation:**
- `ft.info()`, `ft.create()`, `ft.dropindex()`, `ft.search()` on non-existent index
- `JSON.ARRPOP`, `JSON.ARRTRIM` on non-existent key
- `JSON.ARRAPPEND` on non-array value

---

## JSON Module Patterns

### ❌ INCORRECT: Skipping json.dumps for JSON.SET sub-paths
```python
# ❌ WRONG — raw string without JSON encoding
await client.custom_command(['JSON.SET', key, '$.name', 'Alice'])
```

### ✅ CORRECT: Always use json.dumps for JSON.SET values
```python
import json

# ✅ CORRECT — json.dumps produces '"Alice"' which is valid JSON
await client.custom_command(['JSON.SET', key, '$.name', json.dumps('Alice')])

# Works for all types and all paths:
await client.custom_command(['JSON.SET', key, '$', json.dumps({"name": "Alice"})])
await client.custom_command(['JSON.SET', key, '$.score', json.dumps(42)])
await client.custom_command(['JSON.SET', key, '$.tags', json.dumps(["a", "b"])])
```

**Why:** `JSON.SET` expects a JSON-encoded value at ALL paths, including sub-paths. If a user reports "double-quoting", the issue is in response parsing, not in how the value is written.

---

### ❌ INCORRECT: Calling JSON array ops without checking key/type
```python
# ❌ RISKY — crashes if key doesn't exist or path isn't an array
await client.custom_command(['JSON.ARRPOP', key, '$.tags'])
```

### ✅ CORRECT: Pre-validate with JSON.TYPE
```python
async def require_array(client, key: str, path: str) -> dict | None:
    """Returns error dict if not an array, None if OK."""
    exists = await client.exists([key])
    if not exists:
        return {'status': 'error', 'reason': f"Key '{key}' not found"}
    result = await client.custom_command(['JSON.TYPE', key, path])
    jtype = result[0].decode() if isinstance(result, list) else result
    if jtype != 'array':
        return {'status': 'error', 'reason': f"Path is type '{jtype}', not array"}
    return None

# Usage:
err = await require_array(client, key, '$.tags')
if err:
    return err
await client.custom_command(['JSON.ARRPOP', key, '$.tags'])
```

**Why:** `JSON.ARRPOP`, `JSON.ARRTRIM`, and `JSON.ARRAPPEND` raise `RequestError` on non-existent keys or non-array paths, which can crash MCP server transports.

---

## Performance Optimization

### ❌ INCORRECT: Fetching entire JSON object
```python
# ❌ Inefficient — must fetch/parse entire object
user = json.loads(await client.get("user:123"))
name = user["name"]
```

### ✅ CORRECT: Use JSON.GET with path
```python
# ✅ Efficient — fetch only needed fields
name = await client.json_get("user:123", "$.name")
```

**Why:** Fetching only needed fields reduces network transfer and parsing overhead.

---

## Code Patterns

### ❌ INCORRECT: Exceptions as Control Flow
```python
async def fetch_data(key: str) -> str:
    value = await client.get(key)
    if value is None:
        raise ValueError("Not found")  # Don't use exceptions for normal logic
    return value
```

### ✅ CORRECT: Status-Based Returns
```python
async def fetch_data(key: str) -> dict:
    value = await client.get(key)
    if value is None:
        return {"status": "error", "msg": "Not found"}
    return {"status": "ok", "data": value}
```

**Why:** Exceptions are expensive and make code hard to read. Use status returns for predictable logic.

---

### ❌ INCORRECT: Static Method-Only Classes
```python
class CacheUtils:
    @staticmethod
    async def get_user(client, user_id: str) -> str:
        return await client.get(f"user:{user_id}")
```

### ✅ CORRECT: Module-Level Functions
```python
async def get_user(client, user_id: str) -> str:
    return await client.get(f"user:{user_id}")
```

**Why:** Python has modules for namespacing. Static-only classes add unnecessary boilerplate.

---

### ❌ INCORRECT: Tight Coupling
```python
class UserService:
    def __init__(self, host: str, port: int):
        self.config = GlideClientConfiguration([NodeAddress(host, port)])
        self.client = await GlideClient.create(self.config)
```

### ✅ CORRECT: Protocol-Based Abstraction
```python
from typing import Protocol

class CacheClient(Protocol):
    async def get(self, key: str) -> str: ...
    async def set(self, key: str, value: str): ...

class UserService:
    def __init__(self, cache: CacheClient):
        self.cache = cache
```

**Why:** Tight coupling makes testing difficult and prevents swapping implementations.

---

### ❌ INCORRECT: Wildcard Imports
```python
from glide import *  # Namespace pollution
```

### ✅ CORRECT: Explicit Imports
```python
from glide import GlideClient, GlideClientConfiguration, NodeAddress
```

**Why:** Wildcard imports pollute namespace, cause name collisions, and break static analysis.

---
