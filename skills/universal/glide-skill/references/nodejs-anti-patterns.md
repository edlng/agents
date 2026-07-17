# JavaScript/TypeScript GLIDE Anti-Patterns

This document contains anti-patterns specific to JavaScript/TypeScript GLIDE development. These patterns should be avoided in production code.

---

## Async Iteration

### ❌ INCORRECT: forEach with async callbacks
```javascript
// ❌ Wrong - forEach doesn't await (fire-and-forget)
keys.forEach(async (key) => {
  await client.del(key);  // These run immediately, not sequentially
});
console.log("Done!"); // Lies - operations still running

// ❌ Wrong - fire-and-forget (operations not awaited)
keys.forEach(async (key) => {
  await client.del(key);
});
```

### ✅ CORRECT: Sequential with for...of
```javascript
// ✅ Correct - Sequential with for...of
for (const key of keys) {
  await client.del(key);
}
console.log("Actually done");
```

### ✅ CORRECT: Parallel with Promise.all
```javascript
// ✅ Correct - Parallel with Promise.all
await Promise.all(keys.map(key => client.del(key)));
console.log("All operations complete");
```

### ✅ CORRECT: Use batch
```javascript
// ✅ Best - Use batch for multiple operations
const batch = new Batch(false);
keys.forEach(key => batch.del([key]));
await client.exec(batch, true);
```

**Why:** forEach doesn't await async callbacks, causing fire-and-forget behavior.

---

## Batch Operations

### ❌ INCORRECT: Wrong Batch class for client type
```javascript
// ❌ Wrong
const batch = new Batch(true);
await clusterClient.exec(batch, true);
```

### ✅ CORRECT: Use ClusterBatch for cluster
```javascript
// ✅ Correct
const batch = new ClusterBatch(true);
await clusterClient.exec(batch, true);
```

**Why:** Cluster clients require ClusterBatch for proper slot routing.

---

## Binary Data Handling

### ❌ INCORRECT: Missing Decoder.Bytes
```javascript
// ❌ Wrong - UTF-8 decoding fails on binary vectors
const results = await GlideFt.search(client, "idx", query);
```

### ✅ CORRECT: Use Decoder.Bytes
```javascript
// ✅ Correct
const results = await GlideFt.search(client, "idx", query, {
  decoder: Decoder.Bytes,
});
```

**Why:** Binary data (like vector embeddings) cannot be decoded as UTF-8.

---

## Cluster Operations

### ❌ INCORRECT: CROSSSLOT in atomic batches
```javascript
// ❌ Wrong - different slots
const batch = new ClusterBatch(true);
batch.get("key1");
batch.get("key2"); // CROSSSLOT error
```

### ✅ CORRECT: Use hash tags
```javascript
// ✅ Correct - use hash tags
const batch = new ClusterBatch(true);
batch.get("{user}:1");
batch.get("{user}:2");
```

**Why:** Atomic batches in cluster mode require all keys in the same slot.

---

## Method Names

### ❌ INCORRECT: Wrong method name
```javascript
// ❌ Wrong
await GlideFt.dropIndex(client, "idx"); // Not a function
```

### ✅ CORRECT: Lowercase 'index'
```javascript
// ✅ Correct
await GlideFt.dropindex(client, "idx"); // lowercase 'index'
```

**Why:** Method names are case-sensitive.

---

## Resource Management

### ❌ INCORRECT: Missing finally block
```javascript
// ❌ Wrong - client not closed if error occurs
const client = await GlideClient.createClient({...});
await client.set("key", "value");
client.close();
```

### ✅ CORRECT: Always use finally
```javascript
// ✅ Correct - always closes
const client = await GlideClient.createClient({...});
try {
  await client.set("key", "value");
} finally {
  client.close();
}
```

**Why:** Ensures cleanup even if errors occur.

---

## Performance Optimization

### ❌ INCORRECT: Fetching entire JSON object
```typescript
// ❌ Inefficient — must fetch/parse entire object
await client.set("user:123", JSON.stringify(user));
const parsed = JSON.parse(await client.get("user:123"));
```

### ✅ CORRECT: Use Hash for structured data
```typescript
// ✅ Efficient — fetch only needed fields
await client.hset("user:123", { name: "John", email: "john@example.com", age: "30" });
const name = await client.hget("user:123", "name");
```

**Why:** Fetching only needed fields reduces network transfer and parsing overhead.

---

---

## FT.SEARCH Query Injection via `=>` Delimiter

The `=>` token in FT.SEARCH syntax separates a filter from a KNN vector clause. If user-controlled input is interpolated into query strings without sanitization, an attacker can inject a KNN clause that bypasses all filters and returns all documents.

### ❌ INCORRECT: Interpolating user input without sanitization
```javascript
// ❌ VULNERABLE — userFilter could contain "=>[KNN ...]"
const query = `(${userFilter}) ${searchText}`;
```

### ✅ CORRECT: Reject `=>` before query construction
```javascript
if (userFilter && userFilter.includes("=>")) {
  throw new Error("Filter must not contain '=>'");
}
const query = `(${userFilter}) ${searchText}`;
```

**Why:** There is no built-in escaping in Valkey Search syntax. Treat `=>` as a reserved delimiter and reject it in all user-controlled query fragments.