# C# GLIDE Anti-Patterns

This document contains anti-patterns specific to C# GLIDE development. These patterns should be avoided in production code.

---

## Async/Await Patterns

### ❌ INCORRECT: Forgetting await
```csharp
// Wrong - returns Task, not value
var value = client.StringGetAsync("key");  // Task<ValkeyValue>
```

### ✅ CORRECT: Use await
```csharp
var value = await client.StringGetAsync("key");  // ValkeyValue
```

---

### ❌ INCORRECT: Synchronous Blocking
```csharp
// Wrong - can cause deadlocks
var value = client.StringGetAsync("key").Result;
```

### ✅ CORRECT: Use await
```csharp
var value = await client.StringGetAsync("key");
```

---

## Resource Management

### ❌ INCORRECT: Not Using await using
```csharp
// Wrong - client not disposed
var client = await GlideClient.CreateClient(config);
// ... operations ...
// Client never disposed!
```

### ✅ CORRECT: Use await using
```csharp
await using var client = await GlideClient.CreateClient(config);
// ... operations ...
// Client automatically disposed
```

**Why:** Without proper disposal, connections leak and exhaust the connection pool.

---

## Naming Conventions

### ❌ INCORRECT: Wrong Naming Convention
```csharp
// Wrong - C# uses PascalCase
await client.stringSetAsync("key", "value");
```

### ✅ CORRECT: PascalCase
```csharp
await client.StringSetAsync("key", "value");
```

**Why:** C# GLIDE follows .NET naming conventions (PascalCase for methods).

---

## Cluster Operations

### ❌ INCORRECT: Cluster Atomic Batch Across Slots
```csharp
// Wrong - CROSSSLOT error
var batch = new ClusterBatch(isAtomic: true);
batch.StringSet("key1", "value1");  // Different slots
batch.StringSet("key2", "value2");
await client.Exec(batch, raiseOnError: true);  // Throws
```

### ✅ CORRECT: Use hash tags
```csharp
var batch = new ClusterBatch(isAtomic: true);
batch.StringSet("{user}:1", "value1");  // Same slot
batch.StringSet("{user}:2", "value2");
await client.Exec(batch, raiseOnError: true);  // Success
```

**Why:** Atomic batches in cluster mode require all keys to map to the same hash slot.

---

## Cluster Operations

### ❌ INCORRECT: Atomic batch across slots
```csharp
// ❌ This fails - keys in different slots
var batch = new ClusterBatch(isAtomic: true);
batch.StringSet("key1", "value1");  // Slot A
batch.StringSet("key2", "value2");  // Slot B
await client.Exec(batch, raiseOnError: true);  // Throws RequestException: CROSSSLOT
```

### ✅ CORRECT: Non-atomic can span slots
```csharp
// ✅ This works - non-atomic can span slots
var pipeline = new ClusterBatch(isAtomic: false);
pipeline.StringSet("key1", "value1");
pipeline.StringSet("key2", "value2");
await client.Exec(pipeline, raiseOnError: true);
```

**Why:** Non-atomic batches automatically route to multiple nodes.

---

## Performance Optimization

### ❌ INCORRECT: Fetching entire JSON object
```csharp
// ❌ Inefficient — must fetch/parse entire object
var json = JsonSerializer.Serialize(user);
await client.StringSetAsync("user:123", json);
```

### ✅ CORRECT: Use Hash for structured data
```csharp
// ✅ Efficient — fetch only needed fields
await client.HashSetAsync("user:123", new HashEntry[]
{
    new("name", "John"),
    new("email", "john@example.com")
});
var name = await client.HashGetAsync("user:123", "name");
```

**Why:** Fetching only needed fields reduces network transfer and parsing overhead.

---

---

## FT.SEARCH Query Injection via `=>` Delimiter

The `=>` token in FT.SEARCH syntax separates a filter from a KNN vector clause. If user-controlled input is interpolated into query strings without sanitization, an attacker can inject a KNN clause that bypasses all filters and returns all documents.

### ❌ INCORRECT: Interpolating user input without sanitization
```csharp
// ❌ VULNERABLE — userFilter could contain "=>[KNN ...]"
var query = $"({userFilter}) {searchText}";
```

### ✅ CORRECT: Reject `=>` before query construction
```csharp
if (userFilter?.Contains("=>") == true)
    throw new ArgumentException("Filter must not contain '=>'");
var query = $"({userFilter}) {searchText}";
```

**Why:** There is no built-in escaping in Valkey Search syntax. Treat `=>` as a reserved delimiter and reject it in all user-controlled query fragments.