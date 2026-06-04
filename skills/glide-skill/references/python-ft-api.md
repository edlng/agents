# Python GLIDE FT (Search) Module API Reference

## ⚠️ CRITICAL - READ BEFORE USING FT MODULE

1. **NO CLIENT METHODS EXIST**: `client.ft_search()`, `client.ft_create()`, `client.ft()` DO NOT EXIST
2. **ALL FUNCTIONS ARE MODULE-LEVEL**: Import `ft` and call `ft.function(client, ...)`
3. **DO NOT INFER FROM REDIS-PY**: This is NOT Redis-py. Redis-py patterns DO NOT apply.
4. **THIS IS THE ONLY DOCUMENTATION**: Vector search API is not documented elsewhere. Do not infer usage from other sources.

## Import Pattern

```python
# Sync — top-level imports (preferred in v2.3+)
from glide_sync import (
    ft, GlideClient,
    FtCreateOptions, DataType, DistanceMetricType,
    VectorField, VectorFieldAttributesFlat, VectorFieldAttributesHnsw,
    VectorAlgorithm, VectorType,
    TagField, NumericField, TextField,
    FtSearchOptions, FtSearchLimit, ReturnField,
)

# Async — same names from glide
from glide import (
    ft, GlideClient,
    FtCreateOptions, DataType, DistanceMetricType,
    VectorField, VectorFieldAttributesFlat, VectorFieldAttributesHnsw,
    VectorAlgorithm, VectorType,
    TagField, NumericField, TextField,
    FtSearchOptions, FtSearchLimit, ReturnField,
)

# Deep imports also work (legacy, still valid):
from glide_shared.commands.server_modules.ft_options.ft_create_options import (
    FtCreateOptions, TextField, VectorField, VectorFieldAttributesFlat,
    VectorFieldAttributesHnsw, VectorAlgorithm, DistanceMetricType, VectorType, DataType,
)
from glide_shared.commands.server_modules.ft_options.ft_search_options import (
    FtSearchOptions, FtSearchLimit, ReturnField,
)

# WRONG - These do not exist:
# from glide_sync import FT  # NO — lowercase 'ft' only
# client.ft  # NO
# client.ft_search  # NO
# ft.FtCreateOptions  # NO — not public API; import FtCreateOptions directly
```

## Distance Metrics Mapping

```python
from glide_sync import DistanceMetricType  # top-level import (v2.3+)

distance_map = {
    "COSINE": DistanceMetricType.COSINE,
    "L2": DistanceMetricType.L2,
    "IP": DistanceMetricType.IP,
}
```

## Field Constructor Signatures

### TextField
```python
TextField(
    name: TEncodable,
    alias: Optional[TEncodable] = None,
    nostem: bool = False,
    weight: Optional[float] = None,
    withsuffixtrie: bool = False,
    nosuffixtrie: bool = False,
    sortable: bool = False,
)
```

### TagField
```python
TagField(
    name: TEncodable,
    alias: Optional[TEncodable] = None,
    separator: Optional[TEncodable] = None,
    case_sensitive: bool = False,
    sortable: bool = False,
)
```

### NumericField
```python
NumericField(
    name: TEncodable,
    alias: Optional[TEncodable] = None,
    sortable: bool = False,
)
```

### The `sortable` Parameter

All three field types accept `sortable: bool = False`. When set to `True`, the field value can be used for SORTBY in FT.SEARCH results. This enables sorting search results by that field's value.

```python
# Example: create index with sortable fields
schema = [
    TextField("title", sortable=True),
    NumericField("price", sortable=True),
    TagField("category", sortable=True),
]
```

## Core Functions

### ft.create()

**Creates a search index.**

```python
ft.create(
    client: GlideClient,
    index_name: str,
    schema: List[Field],
    options: Optional[FtCreateOptions] = None
) -> str  # Returns "OK"
```

**Parameters:**
- `client`: GlideClient instance (first parameter, always required)
- `index_name`: Name for the index
- `schema`: List of Field objects (TextField, VectorField, etc.)
- `options`: FtCreateOptions with prefixes, data type, etc.

**Complete Example:**
```python
from glide_sync import (
    ft, GlideClient,
    FtCreateOptions, TextField, VectorField,
    VectorFieldAttributesFlat, VectorFieldAttributesHnsw,
    VectorAlgorithm, DistanceMetricType, VectorType, DataType,
)

# Define schema with FLAT algorithm
schema_flat = [
    TextField("title"),
    VectorField(
        "embedding",
        VectorAlgorithm.FLAT,
        VectorFieldAttributesFlat(
            dimensions=768,
            distance_metric=DistanceMetricType.COSINE,
            type=VectorType.FLOAT32
        )
    )
]

# Define schema with HNSW algorithm
schema_hnsw = [
    TextField("title"),
    VectorField(
        "embedding",
        VectorAlgorithm.HNSW,
        VectorFieldAttributesHnsw(
            dimensions=768,
            distance_metric=DistanceMetricType.COSINE,
            type=VectorType.FLOAT32,
            number_of_edges=16,
            vectors_examined_on_construction=200,
            vectors_examined_on_runtime=10,
        )
    )
]

# Create index - NOTE: ft.create(client, ...) NOT client.ft_create(...)
result = ft.create(
    client=client,
    index_name="products_idx",
    schema=schema_hnsw,
    options=FtCreateOptions(data_type=DataType.HASH, prefixes=["product:"])
)
# Returns: "OK"
```

**Common Mistakes:**
```python
# ❌ WRONG - Method does not exist
await client.ft_create("idx", schema)

# ❌ WRONG - No ft attribute on client
client.ft.create("idx", schema)

# ✅ CORRECT - Module-level function
ft.create(client, "idx", schema)
```

### ft.search()

**Searches an index using a query.**

```python
ft.search(
    client: GlideClient,
    index_name: str,
    query: str,
    options: Optional[FtSearchOptions] = None
) -> FtSearchResponse  # [count, {key: {field: value}}]
```

**Parameters:**
- `client`: GlideClient instance (first parameter, always required)
- `index_name`: Name of the index to search
- `query`: Search query string (e.g., "*", "hello", "*=>[KNN 5 @vec $query]")
- `options`: FtSearchOptions with params, return fields, etc.

**Return Format:**
```python
# Returns: [count, {key: {field: value}}]
# Example: [2, {b'doc:1': {b'title': b'Hello'}, b'doc:2': {b'title': b'World'}}]
```

**Basic Search Example:**
```python
from glide_sync import ft, GlideClient

# Simple search - NOTE: ft.search(client, ...) NOT client.ft_search(...)
results = ft.search(
    client=client,
    index_name="products_idx",
    query="*",
    options=None
)

count = results[0]  # Number of results
docs = results[1]   # Dict of documents
```

**Vector Search Example:**
```python
from glide_sync import ft, GlideClient
from glide_shared.commands.server_modules.ft_options.ft_search_options import (
    FtSearchOptions
)
import struct

# Convert embedding to bytes
query_embedding = [0.1, 0.2, 0.3, ...]  # Your embedding
embedding_bytes = b''.join(struct.pack('f', x) for x in query_embedding)

# Vector search - NOTE: ft.search(client, ...) NOT client.ft_search(...)
results = ft.search(
    client=client,
    index_name="products_idx",
    query="*=>[KNN 5 @embedding $vec]",
    options=FtSearchOptions(params={"vec": embedding_bytes})
)

# Decode results (they are bytes)
for key, fields in results[1].items():
    str_key = key.decode() if isinstance(key, bytes) else key
    # See the section on 'Binary Data Handling' for complete decoding
```

**⚠️ FT.SEARCH: Wildcard `*` cannot follow a filter expression**

When combining a filter with a text query, `*` after a filter is rejected as invalid syntax.

```python
# ❌ WRONG — raises RequestError: Invalid wildcard '*' markers
query = "(@category:{Electronics}) *"

# ✅ CORRECT — filter alone acts as match-all within the filter
query = "@category:{Electronics}"

# ✅ CORRECT — combine filter with actual text query
query = "(@category:{Electronics}) headphones"
```

**Helper pattern:**
```python
def build_text_query(query_text: str, filter_expr: str | None) -> str:
    if filter_expr and query_text.strip() == '*':
        return filter_expr
    if filter_expr:
        return f'({filter_expr}) {query_text}'
    return query_text
```

**⚠️ SECURITY: Sanitize user input before query construction**

The `=>` token in FT.SEARCH syntax separates a filter expression from a KNN vector clause. If user-controlled input (e.g., a filter parameter from an API or MCP tool) contains `=>`, an attacker can inject a KNN vector search that bypasses all intended filters and returns all documents. This is a confirmed query injection vulnerability.

**Always reject `=>` in user-supplied filter expressions and vector field names:**
```python
def sanitize_search_input(filter_expr: str | None, vector_field: str | None = None) -> dict | None:
    """Returns error dict if input contains injection payload, None if safe."""
    if filter_expr and '=>' in filter_expr:
        return {'status': 'error', 'reason': "filter must not contain '=>'"}
    if vector_field and '=>' in vector_field:
        return {'status': 'error', 'reason': "vector_field must not contain '=>'"}
    return None
```

Call this before constructing any FT.SEARCH query from user input:
```python
err = sanitize_search_input(filter_expr, vector_field)
if err:
    return err
query = build_text_query(query_text, filter_expr)
```

**Common Mistakes:**
```python
# ❌ WRONG - Method does not exist
results = await client.ft_search("idx", "*")

# ❌ WRONG - No ft attribute on client  
results = client.ft.search("idx", "*")

# ✅ CORRECT - Module-level function
results = ft.search(client, "idx", "*")
```

**Pagination with FtSearchLimit:**

`FtSearchOptions` does not accept `offset` or `limit` as direct keyword arguments. Pagination requires wrapping in `FtSearchLimit`.

```python
# ❌ WRONG — TypeError: unexpected keyword argument
FtSearchOptions(first_result=0, limit=10)
```

```python
# ✅ CORRECT — use FtSearchLimit
from glide_shared.commands.server_modules.ft_options.ft_search_options import (
    FtSearchLimit,
    FtSearchOptions,
)

options = FtSearchOptions(limit=FtSearchLimit(offset=0, count=10))
results = ft.search(client=client, index_name="idx", query="*", options=options)
```

### ft.dropindex()

**Drops an index.**

```python
ft.dropindex(client: GlideClient, index_name: str) -> str  # Returns "OK"
```

**Example:**
```python
from glide_sync import ft

# NOTE: ft.dropindex(client, ...) NOT client.ft_dropindex(...)
result = ft.dropindex(client, "products_idx")
```

### ft.list()

**Lists all indexes.**

```python
ft.list(client: GlideClient) -> List[bytes]  # Returns list of index names
```

**Example:**
```python
from glide_sync import ft

# NOTE: ft.list(client) NOT client.ft_list()
indexes = ft.list(client)
# Returns: [b'idx1', b'idx2']
```

### ft.info()

**Gets information about an index.**

```python
ft.info(client: GlideClient, index_name: str) -> FtInfoResponse
```

**Example:**
```python
from glide_sync import ft

# NOTE: ft.info(client, ...) NOT client.ft_info(...)
info = ft.info(client, "products_idx")
```

### ft.aggregate()

**Performs aggregation query.**

```python
ft.aggregate(
    client: GlideClient,
    index_name: str,
    query: str,
    options: Optional[FtAggregateOptions] = None
) -> FtAggregateResponse
```

**Parameters:**
- `client`: GlideClient instance (first parameter, always required)
- `index_name`: Name of the index to aggregate
- `query`: Search query string — **NOT the same as FT.SEARCH**. See wildcard warning below.
- `options`: FtAggregateOptions with LOAD, GROUPBY, REDUCE, SORTBY, etc.

**⚠️ CRITICAL: FT.AGGREGATE rejects wildcard `*` query**

Unlike FT.SEARCH, FT.AGGREGATE does not accept `*` as a match-all query. It raises `RequestError: Invalid query string syntax`.

```python
# ❌ WRONG — raises RequestError
ft.aggregate(client, "idx", "*", options)

# ✅ CORRECT — use a field filter as match-all
ft.aggregate(client, "idx", "@price:[0 inf]", options)  # numeric range
ft.aggregate(client, "idx", "@category:{*}", options)    # tag wildcard (if applicable)
```

Use a numeric range `[0 inf]` on any indexed NUMERIC field, or a TAG filter. There is no universal match-all for FT.AGGREGATE.

**Return Format:**
```python
# Returns: List[Mapping[bytes, Any]] — a flat list of row dicts
# There is NO leading integer count (unlike FT.SEARCH and unlike valkey-py).
# Example:
[
    {b'category': b'electronics', b'total': b'1500'},
    {b'category': b'books', b'total': b'320'},
]
```

**⚠️ CRITICAL: LOAD is required for reducer fields**

FT.AGGREGATE does not automatically load document fields for REDUCE operations.
Fields used in SUM, AVG, MIN, MAX, etc. must be explicitly loaded with `loadFields`
before the GROUPBY stage. Without LOAD, reducers silently return 0.
COUNT is the only exception — it counts rows, not field values.

**❌ WRONG — SUM returns 0 because `price` is not loaded:**
```python
results = ft.aggregate(
    client=client,
    index_name="products_idx",
    query="@price:[0 500]",
    options=FtAggregateOptions(
        clauses=[
            FtAggregateGroupBy(
                ["@category"],
                [FtAggregateReducer("SUM", ["@price"], "total")],
            )
        ]
    ),
)
# Every row has total = '0'!
```

**✅ CORRECT — LOAD the field first:**
```python
from glide_sync import ft
from glide_shared.commands.server_modules.ft_options.ft_aggregate_options import (
    FtAggregateOptions,
    FtAggregateGroupBy,
    FtAggregateReducer,
)

results = ft.aggregate(
    client=client,
    index_name="products_idx",
    query="@price:[0 500]",
    options=FtAggregateOptions(
        loadFields=["@price"],  # ← Required for SUM/AVG/MIN/MAX
        clauses=[
            FtAggregateGroupBy(
                ["@category"],
                [FtAggregateReducer("SUM", ["@price"], "total")],
            )
        ]
    ),
)
# results: [{b'category': b'electronics', b'total': b'1500'}, ...]
```

Use `loadAll=True` to load all indexed fields (convenient but less efficient).

**⚠️ Response format differs from valkey-py**

GLIDE's `ft.aggregate()` returns a flat `List[Mapping[bytes, Any]]`. There is no leading
integer count element. This is different from valkey-py which returns `[count, row1, row2, ...]`.

**❌ WRONG — assuming valkey-py format:**
```python
raw = ft.aggregate(client, "idx", "*", options)
total = raw[0]        # This is a dict, not an int!
rows = raw[1:]        # Off by one — you're skipping the first result
```

**✅ CORRECT — GLIDE format:**
```python
raw = ft.aggregate(client, "idx", "*", options)
rows = raw            # Flat list of dicts
total = len(raw)      # Count them yourself
```

### ft.profile()

**Profiles a search query.**

```python
ft.profile(
    client: GlideClient,
    index_name: str,
    query: str,
    options: FtProfileOptions
) -> FtProfileResponse
```

## Alias Management

### ft.aliasadd()
```python
ft.aliasadd(client: GlideClient, alias: str, index_name: str) -> str
```

### ft.aliasdel()
```python
ft.aliasdel(client: GlideClient, alias: str) -> str
```

### ft.aliasupdate()
```python
ft.aliasupdate(client: GlideClient, alias: str, index_name: str) -> str
```

### ft.aliaslist()
```python
ft.aliaslist(client: GlideClient) -> Mapping[bytes, bytes]
```

## Query Explanation

### ft.explain()
```python
ft.explain(
    client: GlideClient,
    index_name: str,
    query: str,
    options: Optional[FtExplainOptions] = None
) -> bytes
```

### ft.explaincli()
```python
ft.explaincli(
    client: GlideClient,
    index_name: str,
    query: str,
    options: Optional[FtExplainOptions] = None
) -> bytes
```

## Required Imports

### For ft.create():
```python
# Preferred (v2.3+) — top-level
from glide_sync import (
    ft, FtCreateOptions, TextField, TagField, NumericField,
    VectorField, VectorFieldAttributesFlat, VectorFieldAttributesHnsw,
    VectorAlgorithm, DistanceMetricType, VectorType, DataType,
)

# Legacy (still works)
from glide_sync import ft
from glide_shared.commands.server_modules.ft_options.ft_create_options import (
    FtCreateOptions, Field, TextField, VectorField,
    VectorFieldAttributesFlat, VectorFieldAttributesHnsw,
    VectorAlgorithm, DistanceMetricType, VectorType, DataType,
)
```

### For ft.search():
```python
# Preferred (v2.3+) — top-level
from glide_sync import ft, FtSearchOptions, FtSearchLimit, ReturnField

# Legacy (still works)
from glide_sync import ft
from glide_shared.commands.server_modules.ft_options.ft_search_options import (
    FtSearchOptions, FtSearchLimit, ReturnField,
)
```

### For ft.aggregate():
```python
# Preferred (v2.3+) — top-level
from glide_sync import (
    ft, FtAggregateOptions, FtAggregateGroupBy, FtAggregateReducer,
    FtAggregateFilter, FtAggregateSortBy, FtAggregateSortProperty,
    FtAggregateLimit, FtAggregateApply,
)

# Legacy (still works)
from glide_sync import ft
from glide_shared.commands.server_modules.ft_options.ft_aggregate_options import (
    FtAggregateOptions, FtAggregateGroupBy, FtAggregateReducer,
    FtAggregateFilter, FtAggregateSortBy, FtAggregateSortProperty,
    FtAggregateLimit, FtAggregateApply,
)
```

## Critical Reminders

### ❌ THESE DO NOT EXIST:
```python
client.ft_create(...)      # NO - Not a method
client.ft_search(...)      # NO - Not a method
client.ft.create(...)      # NO - No ft attribute
client.ft.search(...)      # NO - No ft attribute
ft.FtCreateOptions(...)    # NO - not part of ft's public API (use: from glide_sync import FtCreateOptions)
```

### ✅ CORRECT PATTERNS:
```python
from glide_sync import ft, FtCreateOptions, FtSearchOptions  # top-level (v2.3+)

ft.create(client, ...)     # YES - Module-level function
ft.search(client, ...)     # YES - Module-level function (positional or keyword args)
ft.dropindex(client, ...)  # YES - Module-level function
FtCreateOptions(...)       # YES - Imported directly, not from ft module
```

## Common Mistakes

1. **Using client methods**: `client.ft_search()` does not exist - use `ft.search(client, ...)`
2. **Using client.ft attribute**: `client.ft.create()` does not exist - use `ft.create(client, ...)`
3. **Wrong FtCreateOptions access**: `ft.FtCreateOptions` is not part of ft's public API — import `FtCreateOptions` directly from `glide_sync`
4. **Not decoding bytes**: Search results return bytes - must decode to strings
5. **Positional args**: Use keyword arguments for clarity
6. **Inferring from Redis-py**: This is NOT Redis-py - do not use Redis-py patterns
7. **Wrong pagination args**: `FtSearchOptions(first_result=0, limit=10)` does not work - use `FtSearchOptions(limit=FtSearchLimit(offset=0, count=10))`
8. **Missing LOAD in FT.AGGREGATE**: SUM/AVG/MIN/MAX return 0 without `loadFields` - add `loadFields=["@field"]` to `FtAggregateOptions`
9. **Wrong FT.AGGREGATE response format**: GLIDE returns `List[dict]` (flat), not `[count, row1, ...]` like valkey-py

## See Also

- `python.md` - Full Python guide
- `python-anti-patterns.md` - Common mistakes
