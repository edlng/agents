# Node.js GLIDE Skill

## External Resources
- `../assets/nodejs-config.ts` - Client connection config templates, TLS/SSL, authentication (password, username, AWS IAM), cluster, standalone, etc.
- `nodejs-anti-patterns.md` - Anti-patterns to avoid in JavaScript/TypeScript GLIDE development including async iteration, Hash vs JSON performance patterns, and more.


## Package Selection

```javascript
// ✅ Correct
const { GlideClient, GlideClusterClient } = require("@valkey/valkey-glide");

// ❌ Wrong
const valkey = require("valkey"); // Different library
```

## Async Patterns

All operations return Promises. Use async/await:

```javascript
async function example() {
  const value = await client.get("key");
  await client.set("key", "value");
}
```

## Batch/Pipeline Operations

### Standalone Batch
```javascript
const { Batch } = require("@valkey/valkey-glide");

// Atomic batch
const batch = new Batch(true);
batch.set("key1", "value1");
batch.get("key1");
const results = await client.exec(batch, true);

// Non-atomic pipeline
const pipeline = new Batch(false);
pipeline.set("key1", "value1");
pipeline.set("key2", "value2");
const results = await client.exec(pipeline, true);
```

### Cluster Batch
```javascript
const { ClusterBatch } = require("@valkey/valkey-glide");

// Atomic batch (requires same slot)
const batch = new ClusterBatch(true);
batch.set("{user}:1", "Alice");
batch.get("{user}:1");
const results = await client.exec(batch, true);

// Non-atomic pipeline (can span slots)
const pipeline = new ClusterBatch(false);
pipeline.set("key1", "value1");
pipeline.set("key2", "value2");
const results = await client.exec(pipeline, true);
```

### Retry Strategies (Cluster Only)

**Retry on server errors:**
```javascript
const options = {
    retryStrategy: {
        retryServerError: true,
        retryConnectionError: false,
    },
};
const results = await client.exec(batch, true, options);
```

**Retry on connection errors:**
```javascript
const options = {
    retryStrategy: {
        retryServerError: false,
        retryConnectionError: true,
    },
};
const results = await client.exec(batch, true, options);
```

**Retry on both:**
```javascript
const options = {
    retryStrategy: {
        retryServerError: true,
        retryConnectionError: true,
    },
};
const results = await client.exec(batch, true, options);
```

**No retries:**
```javascript
const options = {
    retryStrategy: {
        retryServerError: false,
        retryConnectionError: false,
    },
};
const results = await client.exec(batch, true, options);
```

See SKILL.md for retry strategy decision matrix.

## Vector Search (FT Module)

### Import
```javascript
const { GlideFt, Decoder } = require("@valkey/valkey-glide");
```

### Create Index
```javascript
await GlideFt.create(
  client,
  "products_idx",
  [
    {
      name: "description_vector",
      alias: "vector",
      type: "VECTOR",
      attributes: {
        algorithm: "HNSW",
        type: "FLOAT32",
        dimensions: 768,
        distanceMetric: "L2",
      },
    },
  ],
  { dataType: "HASH", prefixes: ["product:"] }
);
```

### Store Vectors
```javascript
// Create binary vector from Float32Array
const vector = Buffer.from(new Float32Array([1.0, 2.0, 3.0]).buffer);

await client.hset("product:1", {
  name: "Product A",
  description_vector: vector,
});
```

### Search

**⚠️ SECURITY:** The `=>` token in FT.SEARCH syntax separates a filter from a KNN clause. If user-controlled input (e.g., a filter parameter) contains `=>`, an attacker can inject a KNN query that bypasses all filters and returns all documents. Reject `=>` in any user-supplied filter or field name before interpolating into query strings:
```javascript
if (userFilter && userFilter.includes("=>")) {
  throw new Error("Filter must not contain '=>'");
}
```

```javascript
const queryVector = Buffer.from(new Float32Array([1.5, 2.5, 3.5]).buffer);

const [count, documents] = await GlideFt.search(
  client,
  "products_idx",
  "*=>[KNN 10 @vector $vec]",
  {
    params: [{ key: "vec", value: queryVector }],
    returnFields: [{ fieldIdentifier: "name" }],
    decoder: Decoder.Bytes, // Required for binary data
  }
);

console.log(`Found ${count} results`);
documents.forEach(doc => {
  console.log(doc.key); // Buffer
  console.log(doc.value); // Array of field-value pairs
});
```

**FtSearchOptions type:**
```typescript
type FtSearchOptions = {
  timeout?: number;
  returnFields?: { fieldIdentifier: GlideString; alias?: GlideString }[];
  params?: GlideRecord<GlideString>;
  nocontent?: boolean;
};
```

### Drop Index
```javascript
await GlideFt.dropindex(client, "products_idx"); // Note: lowercase 'index'
```

## Cluster Operations

### Hash Tags for Slot Control
```javascript
// Keys with same hash tag go to same slot
await client.set("{user}:1:name", "Alice");
await client.set("{user}:1:email", "alice@example.com");

// Atomic batch works
const batch = new ClusterBatch(true);
batch.get("{user}:1:name");
batch.get("{user}:1:email");
const results = await client.exec(batch, true);
```

### Multi-Slot Operations
```javascript
// Non-atomic batch for different slots
const batch = new ClusterBatch(false);
batch.del(["key1", "key2", "key3"]);
const results = await client.exec(batch, true);
```

## Error Handling

```javascript
try {
  await client.set("key", "value");
} catch (error) {
  console.error("Error:", error.message);
}
```

## Best Practices

### Async Iteration
Use `for...of` for sequential operations, `Promise.all` for parallel, or batches for efficiency.

### Resource Cleanup
Always close clients in finally blocks to ensure cleanup even if errors occur.

---

## Common Pitfalls

### 1. Wrong Batch Class for Client Type
**Problem:** Using `Batch` with cluster client
**Solution:** Use `ClusterBatch` for cluster clients

### 2. Missing Decoder.Bytes for Binary Data
**Problem:** UTF-8 decoding fails on binary vectors
**Solution:** Use `decoder: Decoder.Bytes` option

### 3. CROSSSLOT Errors in Atomic Batches
**Problem:** Atomic operations with keys in different slots
**Solution:** Use hash tags `{tag}` to ensure same slot

### 4. Wrong Casing
**Problem:** Case-sensitive method names
**Solution:** Use correct casing (e.g., `dropindex` not `dropIndex`)

### 5. forEach with Async Callbacks
**Problem:** Fire-and-forget behavior
**Solution:** Use `for...of`, `Promise.all`, or batches

### 6. Missing finally Block
**Problem:** Client not closed if error occurs
**Solution:** Always use try/finally for cleanup

---

## Client Lifecycle Management

```javascript
// module-level — initialize before server starts, close on exit
let client = await GlideClient.createClient({ addresses: [...], requestTimeout: 500 });

process.on("SIGTERM", () => { client?.close(); process.exit(0); });
process.on("SIGINT",  () => { client?.close(); process.exit(0); });
```

---

# Performance Optimization

Config templates: `../assets/nodejs-config.ts`

## AZ Affinity

```typescript
import { GlideClusterClient, ReadFrom } from "@valkey/valkey-glide";

const client = await GlideClusterClient.createClient({
    addresses: [{ host: "cluster.endpoint.cache.amazonaws.com", port: 6379 }],
    readFrom: "AZAffinity" as ReadFrom,
    clientAz: "us-east-1a",
    requestTimeout: 500,
});
```

## Throughput Tuning

```typescript
const client = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
    inflightRequestsLimit: 2000, // Default: 1000
    requestTimeout: 500,
});
```

## Serverless / Lambda

```typescript
const client = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
    lazyConnect: true, // Defer connection until first command
    requestTimeout: 500,
});
```

## Retry Strategy

```typescript
const client = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
    connectionBackoff: {
        numberOfRetries: 10,
        factor: 500,
        exponentBase: 2,
    },
    requestTimeout: 500,
});
```

## Dedicated Blocking Client

```typescript
const blockingClient = await GlideClient.createClient({
    addresses: [{ host: "localhost", port: 6379 }],
    requestTimeout: 30000,
    clientName: "queue-worker",
});
const item = await blockingClient.blpop(["queue"], 30);
```

## Hash vs JSON for Structured Data

## Typed Error Handling

```typescript
import { ConnectionError, TimeoutError, RequestError } from "@valkey/valkey-glide";

try {
    const result = await client.get("key");
} catch (error) {
    if (error instanceof TimeoutError) {
        // Retry with exponential backoff
    } else if (error instanceof ConnectionError) {
        // Circuit breaker pattern
    } else if (error instanceof RequestError) {
        // Check command syntax
    }
}
```

## Concurrent Operations

```typescript
// For truly independent operations on different keys:
const [user, posts, comments] = await Promise.all([
    client.get("user:123"),
    client.lrange("posts:123", 0, -1),
    client.lrange("comments:123", 0, -1),
]);

// Handle partial failures:
const results = await Promise.allSettled([
    client.get("key1"),
    client.get("key2"),
]);
```

## Monitoring

### OpenTelemetry

```typescript
import { OpenTelemetry } from "@valkey/valkey-glide";

OpenTelemetry.init({
    traces: { endpoint: "http://localhost:4318/v1/traces", samplePercentage: 1 },
    metrics: { endpoint: "http://localhost:4318/v1/metrics" },
});
```

### Logging

```typescript
import { Logger } from "@valkey/valkey-glide";

Logger.setLoggerConfig("warn", "glide.log");  // Production
Logger.setLoggerConfig("error");               // Max performance
```

Server-side config: `server-configuration-guide.md`
