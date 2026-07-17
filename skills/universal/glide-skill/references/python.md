# General Python Guidelines

## 🚨 CRITICAL WARNINGS - READ FIRST

### Vector Search / FT Module

1. **DO NOT INFER FROM REDIS-PY**: This is Valkey GLIDE, NOT Redis-py. Redis-py patterns DO NOT apply here.
2. **NO CLIENT METHODS**: `client.ft_search()`, `client.ft_create()`, `client.ft()` DO NOT EXIST in GLIDE.
3. **ONLY SOURCE OF TRUTH**: `python-ft-api.md` is the ONLY documentation for vector search. Do not infer usage from any other source.
4. **MODULE-LEVEL FUNCTIONS ONLY**: All FT functions are `ft.function(client, ...)` NOT `client.ft_function(...)`

---

## External Resources

### FT Module API
- **`python-ft-api.md`** - Complete FT (Search) module API reference (READ THIS for vector search)

### Code Snippets
- `../assets/python-config.py` - Client connection config templates, TLS/SSL, authentication (password, username, AWS IAM), cluster, standalone, sync, async, etc.
- `python-batch-*.py` - Python batch and pipelining code examples
- https://glide.valkey.io/languages/python/api/glide_async/core/ - Python Async API Reference
- https://glide.valkey.io/languages/python/api/glide_sync/core/ - Python Sync API Reference

### Additional Anti-Patterns
- `python-anti-patterns.md` - Additional anti-patterns including test mocking patterns (sync and async), AsyncMock for async functions, GLIDE client event loop binding, pytest-asyncio fixture scope deadlocks, performance patterns (Hash vs JSON), code design patterns, and more

---

## Core Principles

1. Use Valkey GLIDE clients (`valkey-glide-sync` or `valkey-glide`), NOT the `valkey` package (Redis fork).
2. Use batching / pipelining when suitable to group operations for efficiency.

---

### Package Selection

#### ❌ NEVER use the Redis fork
```python
# NEVER use these imports
from valkey import Valkey
from valkey.commands.search import Search
```

#### ✅ ALWAYS use GLIDE

**Synchronous (for sync applications):**
```python
from glide_sync import GlideClient, GlideClusterClient, ft
from glide_sync import GlideClientConfiguration, GlideClusterClientConfiguration, NodeAddress
from glide_shared.commands.server_modules.ft_options.ft_search_options import FtSearchOptions
from glide_shared.commands.server_modules.ft_options.ft_create_options import (
    DistanceMetricType,
    VectorField,
    VectorFieldAttributesFlat,
    VectorAlgorithm,
    VectorType,
    TagField,
    NumericField,
)
```
**Package:** `valkey-glide-sync>=2.0.0`

**Asynchronous (for async applications):**
```python
from glide import GlideClient, GlideClusterClient, ft
from glide import GlideClientConfiguration, GlideClusterClientConfiguration, NodeAddress
from glide_shared.commands.server_modules.ft_options.ft_search_options import FtSearchOptions
from glide_shared.commands.server_modules.ft_options.ft_create_options import (
    DistanceMetricType,
    VectorField,
    VectorFieldAttributesFlat,
    VectorAlgorithm,
    VectorType,
    TagField,
    NumericField,
)
```
**Package:** `valkey-glide>=2.0.0`

**Note:** `glide_shared` is used by both sync and async packages for shared types and options.

---

### Binary Data Handling

**❌ WRONG - Not decoding bytes from search results:**
```python
results = ft.search(client, index_name, query)
print(results[1].keys())  # b'doc:1' instead of 'doc:1'
```

**✅ CORRECT - Decode bytes to strings:**
```python
for key, fields in results[1].items():
    str_key = key.decode() if isinstance(key, bytes) else key
    # See the section on 'Binary Data Handling' for complete implementation
```

Must decode to strings, but skip binary fields like embeddings, complete example below:

```python
from typing import Any


def _decode_docs(results) -> list[dict[str, Any]]:
    """Decode documents from the search results."""

    count = results[0]
    docs = []
    # Iterates results; decodes fields; skips binary embeddings
    if count > 0 and len(results) > 1:
        for key, fields in results[1].items():
            str_key = key.decode() if isinstance(key, bytes) else key
            str_fields = {}
            for field_key, field_value in fields.items():
                str_field_key = field_key.decode() if isinstance(field_key, bytes) else field_key
                # Skip binary fields (like vector embeddings)
                if str_field_key == "embedding":
                    continue
                try:
                    str_field_value = field_value.decode() if isinstance(field_value, bytes) else field_value
                    str_fields[str_field_key] = str_field_value
                except (UnicodeDecodeError, AttributeError):
                    pass  # Skip binary fields
            docs.append({"key": str_key, **str_fields})
    return docs
```

## Client Creation Pattern

### Choose Sync vs Async

**Use synchronous client when:**
- Building sync applications (e.g., LangChain VectorStore, Flask apps)
- Integrating with sync-only frameworks
- Simplicity is preferred over concurrency

**Use asynchronous client when:**
- Building async applications (FastAPI, aiohttp)
- Need high concurrency
- Using async/await patterns throughout

### Choose Cluster vs Standalone

**Use cluster client when:**
- Running multiple GLIDE nodes
- Using multiple Valkey clusters

Otherwise, use standalone client.

### General Contract

```python
from glide_sync import (
    GlideClient,
    GlideClientConfiguration,
    GlideClusterClient,
    GlideClusterClientConfiguration,
    NodeAddress,
)
from glide_shared.exceptions import ConnectionError, ClosingError, TimeoutError

def get_client(valkey_url: str, **kwargs) -> GlideClient | GlideClusterClient:
    host, port = _parse_valkey_url(valkey_url)
    addresses = [NodeAddress(host, port)]
    config = GlideClientConfiguration(
        addresses=addresses,
        request_timeout=5000,
        **kwargs
    )
    return GlideClient.create(config)
```

**Key Points:**
- Import from `glide` instead of `glide_sync` for the async client
- Use `NodeAddress` for connection configuration
- Add `request_timeout` to prevent hanging
- Use `await` for async client operations
- Async client requires `await` on `.create()`

---

## Vector Search Constraints

**❌ WRONG - Adding .sort_by() to KNN queries:**
```python
results = ft.search(...).sort_by("score")  # Causes error
```

**✅ CORRECT - KNN results already sorted:**
```python
results = ft.search(...)  # Already sorted by score
```

**Both positional and keyword arguments work for ft.search():**
```python
# Positional — valid
results = ft.search(client, index_name, query, FtSearchOptions(params={"vector": embedding_buffer}))

# Keyword — also valid, more readable
results = ft.search(
    client=client,
    index_name=index_name,
    query=query,
    options=FtSearchOptions(params={"vector": embedding_buffer}),
)
```

**❌ WRONG - Using ft.FtCreateOptions:**
```python
from glide_sync import ft
ft.create(client, index_name, schema, ft.FtCreateOptions(prefixes=["doc:"]))
# ft.FtCreateOptions does NOT exist — FtCreateOptions is a standalone class
```

**✅ CORRECT - Import FtCreateOptions directly:**
```python
from glide_sync import ft, FtCreateOptions  # top-level import (v2.3+)
ft.create(client, index_name, schema, FtCreateOptions(prefixes=["doc:"]))
```

---

## FT.SEARCH Command Pattern

### Vector Similarity Search
Return value is a two-element array / list, first element being the number of documents, the second element
being a dictionary of those documents.  See the section on *Binary Data Handling* for an example.

```python
from glide_sync import ft
from glide_shared.commands.server_modules.ft_options.ft_search_options import FtSearchOptions

# ⚠️ SECURITY: Sanitize user-supplied filter and vector_field before interpolation.
# The '=>' token delimits filter from KNN clause — if user input contains '=>',
# an attacker can inject a KNN query that bypasses all filters.
if filter and '=>' in filter:
    raise ValueError("filter must not contain '=>'")
if '=>' in vector_field:
    raise ValueError("vector_field must not contain '=>'")

# Build KNN query, using `vector_field` to identify the vector field
base_query = f"*=>[KNN {k} @{vector_field} $vector AS score]"

# With metadata filter
if filter:
    base_query = f"({filter})=>[KNN {k} @{vector_field} $vector AS score]"

# Convert query vector to bytes
embedding_buffer = struct.pack(f"{len(query_vector)}f", *query_vector)

# Execute search
results = ft.search(
    client=client,
    index_name=index_name,
    query=base_query,
    options=FtSearchOptions(params={"vector": embedding_buffer}),
)

# Decode results to strings / dictionaries
docs = _decode_docs(results)
```

**Key Points:**
- Use `ft.search()` function, not a method on client and not `client.ft_search()`
- Use keyword arguments: `client=`, `index_name=`, `query=`, `options=`
- Use `FtSearchOptions` for parameters
- Results format: `[count, {key: {field: value}}]`
- **IMPORTANT:** GLIDE returns bytes - decode to strings for JSON/display
- **Skip binary fields** like vector embeddings (can't decode to UTF-8)

---

## FT.CREATE Command Pattern
Vector fields, tag fields, and numeric fields should be parameterized.
- Tag fields are used for exact matching.
- Numeric fields are used for range matching.

```python
from glide_sync import ft
from glide_shared.commands.server_modules.ft_options.ft_create_options import (
    DistanceMetricType,
    VectorField,
    VectorFieldAttributesFlat,
    VectorAlgorithm,
    VectorType,
    TagField,
    NumericField,
    FtCreateOptions,
)

# Build schema
schema = [
    VectorField(
        "content_vector",
        VectorAlgorithm.FLAT,  # or VectorAlgorithm.HNSW
        VectorFieldAttributesFlat(
            dimensions=1536,
            distance_metric=DistanceMetricType.COSINE,
            type=VectorType.FLOAT32,
        ),
    ),
    TagField("category"),
    NumericField("year"),
]

# Create index
ft.create(
    client,
    index_name,
    schema,
    FtCreateOptions(prefixes=["doc:"]),
)
```

### Index Creation
⚠️ **CRITICAL**: Do NOT use `client.ft_create()` to create an index.

**Key Points:**
- Use `ft.create()` function, not a method
- Import `FtCreateOptions` from ft_create_options
- Use typed field objects (VectorField, TagField, NumericField)
- Pass `FtCreateOptions` (not `ft.FtCreateOptions`) as 4th argument — import from `glide_sync` directly

---

## Add Document Pattern
Documents are stored via `HSET` with vector bytes:

```python
embedding_buffer = struct.pack(f"{len(embedding)}f", *embedding)
fields = {"embedding": embedding_buffer}
if metadata:
    fields.update(metadata)
client.hset(key, fields)
```

---

## Check Index Exists / Drop Index

**⚠️ Use `ft.list()` — not `ft.info()` — to check index existence.** `ft.info()` raises `RequestError` on missing indices, which can crash MCP server transports. `ft.list()` always returns cleanly.

```python
from glide_sync import ft

async def index_exists(client, index_name: str) -> bool:
    """Safe index existence check — never raises."""
    existing = await ft.list(client)
    names = {i.decode() if isinstance(i, bytes) else str(i) for i in (existing or [])}
    return index_name in names
```

**Drop index safely:**
```python
if await index_exists(client, index_name):
    ft.dropindex(client, index_name)
```

**Create index safely:**
```python
if not await index_exists(client, index_name):
    ft.create(client, index_name, schema, options)
```

---

## Type Hints

### Client Type Union

**Synchronous:**
```python
from typing import TYPE_CHECKING, Union

if TYPE_CHECKING:
    from glide_sync import GlideClient, GlideClusterClient
    GlideClientType = Union[GlideClient, GlideClusterClient]
    # Or with Python 3.10+
    GlideClientType = GlideClient | GlideClusterClient
```

**Asynchronous:**
```python
from typing import TYPE_CHECKING, Union

if TYPE_CHECKING:
    from glide import GlideClient, GlideClusterClient
    GlideClientType = Union[GlideClient, GlideClusterClient]
    # Or with Python 3.10+
    GlideClientType = GlideClient | GlideClusterClient
```

**Why:** Avoid runtime import errors when GLIDE is not installed.

---

## Best Practices

## Common Pitfalls

**Package selection, binary data decoding, and FT module API are covered in sections above and in `python-ft-api.md`. Mock patterns are in `python-anti-patterns.md`. The table below lists pitfalls NOT covered elsewhere.**

| Description | Problem | Solution |
|-------------|---------|----------|
| Wrong FT API Pattern | Using `client.ft_search()`, `client.ft_create()`, or `client.ft.search()` | All FT functions are module-level: `ft.function(client, ...)` — see `python-ft-api.md` |
| Wrong FtCreateOptions Access | Using `ft.FtCreateOptions(...)` — does not exist | Import `FtCreateOptions` directly: `from glide_sync import FtCreateOptions` |
| Missing FtSearchOptions | Passing params directly to `ft.search()` | Wrap params in `FtSearchOptions(params={...})` |
| Adding .sort_by() to KNN Queries | Trying to sort KNN results manually | KNN results are pre-sorted by score, don't add sorting |
| Expecting ping() to Return Bool | Assuming `client.ping()` returns `True` for success | `ping()` returns `b'PONG'` (bytes), not a boolean. Check with `== b'PONG'` |
| Using Integer Cursor with scan() | Passing integer cursor to `scan()`: `cursor = 0` | `scan()` requires bytes cursor: `cursor = b"0"` and returns bytes |
| Wrong FtSearch Pagination | `FtSearchOptions(first_result=0, limit=10)` — TypeError | Use `FtSearchOptions(limit=FtSearchLimit(offset=0, count=10))` — see `python-ft-api.md` |
| Missing LOAD in FT.AGGREGATE | SUM/AVG/MIN/MAX return 0 | Add `loadFields=["@field"]` to `FtAggregateOptions` — see `python-ft-api.md` |
| Wrong FT.AGGREGATE Response Format | Assuming `raw[0]` is count like valkey-py | GLIDE returns flat `List[dict]`, no leading count — see `python-ft-api.md` |

---

## Dependencies

### Package Installation

**Synchronous applications:**
```toml
[project.optional-dependencies]
valkey = ["valkey-glide-sync>=2.0.0"]
```

**Asynchronous applications:**
```toml
[project.optional-dependencies]
valkey = ["valkey-glide>=2.0.0"]
```

**Why optional:** Keeps base package lightweight, users install only what they need.

---

## Client Lifecycle Management

**Async (FastAPI lifespan):**
```python
@asynccontextmanager
async def lifespan(app: FastAPI):
    global _client
    _client = await GlideClient.create(config)
    yield
    await _client.close()
```

**Sync:**
```python
_client = GlideClient.create(config)
atexit.register(lambda: _client.close())
```

---
# Pipelining and Batching GLIDE Patterns

Language-specific implementation details for Valkey GLIDE Python clients.

## Batch Commands (Python)

### Sync Client

See `python-batch-sync.py` code template

### Async Client

See `python-batch-async.py` code template

### Error Handling

See `python-batch-error-handling.py` code template

### Key Points

- Use `raise_on_error` parameter (Python uses snake_case)
- Import `BatchRetryStrategy` from `glide_shared.commands.server_modules.batch_options`
- Async client requires `await` on `exec()`
- Errors are `RequestError` exceptions from `glide_shared.exceptions`
- See SKILL.md for retry strategy decision matrix

### Retry Strategies (Cluster Only)
**Retry on server errors:**
```python
from glide_shared.commands.batch_options import BatchRetryStrategy

options = ClusterBatchOptions(
    retry_strategy=BatchRetryStrategy(
        retry_server_error=True,
        retry_connection_error=False,
    )
)
results = await client.exec(batch, raise_on_error=True, options=options)
```

**Retry on connection errors:**
```python
options = ClusterBatchOptions(
    retry_strategy=BatchRetryStrategy(
        retry_server_error=False,
        retry_connection_error=True,
    )
)
results = await client.exec(batch, raise_on_error=True, options=options)
```

**Retry on both:**
```python
options = ClusterBatchOptions(
    retry_strategy=BatchRetryStrategy(
        retry_server_error=True,
        retry_connection_error=True,
    )
)
results = await client.exec(batch, raise_on_error=True, options=options)
```

**No retries:**
```python
options = ClusterBatchOptions(
    retry_strategy=BatchRetryStrategy(
        retry_server_error=False,
        retry_connection_error=False,
    )
)
results = await client.exec(batch, raise_on_error=True, options=options)
```

---

# Performance Optimization

Config templates: `../assets/python-config.py`

## AZ Affinity

```python
from glide import GlideClusterClient, GlideClusterClientConfiguration, NodeAddress, ReadFrom

config = GlideClusterClientConfiguration(
    addresses=[NodeAddress("cluster.endpoint.cache.amazonaws.com", 6379)],
    read_from=ReadFrom.AZ_AFFINITY,
    client_az="us-east-1a",
    request_timeout=500,
)
client = await GlideClusterClient.create(config)
```

## Throughput Tuning

```python
config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    inflight_requests_limit=2000,  # Default: 1000
    request_timeout=500,
)
```

## Serverless / Lambda

```python
config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    lazy_connect=True,  # Defer connection until first command
    request_timeout=500,
)
client = await GlideClient.create(config)
```

## Dedicated Blocking Client

```python
blocking_client = await GlideClient.create(GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    request_timeout=30000,
    client_name="queue-worker",
))
item = await blocking_client.blpop(["queue"], 30)
```

## Hash vs JSON for Structured Data

## Context Manager Pattern

```python
# Async
async with await GlideClient.create(config) as client:
    result = await client.get("key")

# Sync
with GlideClient.create(config) as client:
    result = client.get("key")
```

## Monitoring

### OpenTelemetry

```python
from glide import OpenTelemetry, OpenTelemetryConfig, OpenTelemetryTracesConfig, OpenTelemetryMetricsConfig

OpenTelemetry.init(OpenTelemetryConfig(
    traces=OpenTelemetryTracesConfig(
        endpoint="http://localhost:4318/v1/traces",
        sample_percentage=1,  # 1% for production
    ),
    metrics=OpenTelemetryMetricsConfig(
        endpoint="http://localhost:4318/v1/metrics",
    ),
))
```

### Logging

```python
from glide import Logger

Logger.set_logger_config("warn", "glide.log")   # Production
Logger.set_logger_config("error")                # Max performance
```

## Concurrent Operations (Async)

```python
# When operations are truly independent and on different keys:
user, posts, comments = await asyncio.gather(
    client.get("user:123"),
    client.lrange("posts:123", 0, -1),
    client.lrange("comments:123", 0, -1),
)
```

Server-side config: `server-configuration-guide.md`
