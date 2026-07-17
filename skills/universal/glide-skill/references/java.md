# General Java Guidelines

## External Resources
- `../assets/java-config.java` - Client connection config templates, TLS/SSL, authentication (password, username, AWS IAM), cluster, standalone, etc.
- `java-anti-patterns.md` - Anti-patterns to avoid in Java GLIDE development, including exception handling, Hash vs JSON performance, thread safety, and more.

## Core Principles

1. Use Valkey GLIDE clients (`valkey-glide`), NOT Jedis or Lettuce clients.
2. Avoid catching general exceptions when handling GLIDE errors - use specific exception types.
3. Use batching / pipelining when suitable to group operations for efficiency.
4. Prefer async chaining over blocking calls for production applications.
5. Add comments to clarify sync vs async when not obvious from syntax.

---

## Package Selection

### ✅ CORRECT: Use GLIDE

**Maven:**
```xml
<build>
    <extensions>
        <extension>
            <groupId>kr.motd.maven</groupId>
            <artifactId>os-maven-plugin</artifactId>
            <version>1.7.1</version>
        </extension>
    </extensions>
</build>
<dependencies>
    <dependency>
        <groupId>io.valkey</groupId>
        <artifactId>valkey-glide</artifactId>
        <classifier>${os.detected.classifier}</classifier>
        <version>[2.0.0,)</version>
    </dependency>
</dependencies>
```

**Gradle:**
```gradle
plugins {
    id 'com.google.osdetector' version '1.7.3'
}

dependencies {
    implementation group: 'io.valkey', name: 'valkey-glide', version: '2.+', classifier: osdetector.classifier
}
```

**Key Points:**
- Classifier is required (native binaries per platform)
- Use `os-maven-plugin` or `osdetector` for platform detection
- Supports: linux-x86_64, linux-aarch_64, osx-x86_64, osx-aarch_64, windows-x86_64

### ❌ INCORRECT: Don't use Jedis or Lettuce
```java
// NEVER use these
import redis.clients.jedis.*;
import io.lettuce.core.*;
```
---

## Client Creation Pattern

### Core Imports
```java
import glide.api.GlideClient;
import glide.api.GlideClusterClient;
import glide.api.models.configuration.GlideClientConfiguration;
import glide.api.models.configuration.GlideClusterClientConfiguration;
import glide.api.models.configuration.NodeAddress;
import glide.api.models.Batch;
import glide.api.models.ClusterBatch;
import glide.api.models.exceptions.RequestException;
import glide.api.models.exceptions.TimeoutException;
import glide.api.models.exceptions.ConnectionException;
```

### Choose Cluster vs Standalone

**Use cluster client when:**
- Running multiple GLIDE nodes
- Using multiple Valkey clusters

Otherwise, use standalone client.

## Async vs Blocking

### When to Use Async (Recommended)
- Production applications
- High concurrency requirements
- Non-blocking I/O frameworks (Netty, etc.)
- Better thread utilization

### When to Use Blocking
- Simple scripts or demos
- Sequential processing requirements
- Simpler code for prototypes

### Async Pattern
Use async method chaining, and narrow exception checking via `instanceof` in `exceptionally` handlers.

```java
GlideClientConfiguration config = GlideClientConfiguration.builder()
    .address(NodeAddress.builder()
        .host("localhost")
        .port(6379)
        .build())
    .requestTimeout(10000)  // Recommended: explicit timeout
    .build();

// Async (non-blocking) - using CompletableFuture chaining
GlideClient.createClient(config).thenCompose(client -> {
    return client.set("key", "value")
        .thenCompose(ok -> client.get("key"))
        .thenAccept(value -> System.out.println(value))
        .exceptionally(e -> {
            // Direct access to GLIDE exceptions, no unwrapping
            if (e instanceof RequestException) {
                System.err.println("Request error: " + e.getMessage());
            }
            return null;
        })
        .whenComplete((v, e) -> {
            try {
                client.close();
            } catch (ExecutionException ex) {
                System.err.println("Error closing: " + ex.getMessage());
            }
        });
}).join(); // Only block at the end
```

### Blocking Pattern

```java
GlideClientConfiguration config = GlideClientConfiguration.builder()
    .address(NodeAddress.builder()
        .host("localhost")
        .port(6379)
        .build())
    .requestTimeout(10000)
    .build();

// Blocking (sync) - using .get() on CompletableFuture
try (GlideClient client = GlideClient.createClient(config).get()) {
    client.set("key", "value").get();
    String value = client.get("key").get();
    System.out.println(value);
}
```

**Key Points:**
- Client creation returns `CompletableFuture<GlideClient>`
- Async: Use `.thenCompose()`, `.thenAccept()`, etc. for chaining
- Blocking: Call `.get()` or `.join()` to block thread
- Always set explicit `requestTimeout()` (default may be too short)
- Use try-with-resources for automatic cleanup in blocking mode

---

### Exception Handling Differences
These catch an `ExecutionException`, branch on the enclosed narrower exception, perform any narrow-specific processing, and then rethrows it.

**Async chaining:**
```java
client.get("key")
    .exceptionally(e -> {
        // Exception may be wrapped - unwrap to check actual cause
        Throwable cause = (e instanceof CompletionException && e.getCause() != null) 
            ? e.getCause() : e;
        
        if (cause instanceof RequestException) {
            // Handle and optionally rethrow
            System.err.println("Request error: " + cause.getMessage());
            throw new CompletionException((RequestException) cause);  // Rethrow
        }
        return null;  // Or return default value
    });
```

**Blocking with .get():**
```java
try {
    client.get("key").get();
} catch (ExecutionException e) {
    // Must unwrap: actual exception is in getCause()
    if (e.getCause() instanceof RequestException) {
        RequestException re = (RequestException) e.getCause();
        // Handle error
    }
}
```

**Blocking with .join():**
```java
try {
    client.get("key").join();
} catch (CompletionException e) {
    // Must unwrap: actual exception is in getCause()
    if (e.getCause() instanceof RequestException) {
        RequestException re = (RequestException) e.getCause();
        // Handle error
    }
}
```

**Key Finding:** Async exceptions may arrive wrapped in `CompletionException` - unwrap with `getCause()` before checking type. Rethrow by wrapping in new `CompletionException` to propagate up the chain.

---

## Batch Commands (Java)

### Standalone Client

**Async (recommended):**
```java
Batch pipeline = new Batch(false);  // Non-atomic (pipeline)
pipeline.set("key1", "value1");
pipeline.set("key2", "value2");
pipeline.get("key1");

client.exec(pipeline, true)
    .thenAccept(results -> {
        // results is Object[]
        System.out.println(Arrays.toString(results));
    });
```

**Blocking:**
```java
Batch transaction = new Batch(true);  // Atomic (transaction)
transaction.set("counter", "0");
transaction.incr("counter");
transaction.get("counter");

Object[] results = client.exec(transaction, true).get();
// results: [OK, 1, 1]
```

### Key Points

- Constructor: `new Batch(boolean isAtomic)` - positional parameter, not named
- Execution: `client.exec(batch, raiseOnError)` - camelCase parameter
- Returns: `CompletableFuture<Object[]>` - need casting for specific types
- `raiseOnError=true`: Throws first error as exception
- `raiseOnError=false`: Returns errors in result array
- See SKILL.md for retry strategy decision matrix

### Retry Strategies (Cluster Only)

**Retry on server errors:**
```java
import glide.api.models.commands.batch.ClusterBatchOptions;
import glide.api.models.commands.batch.ClusterBatchRetryStrategy;

ClusterBatchOptions options = ClusterBatchOptions.builder()
    .retryStrategy(ClusterBatchRetryStrategy.builder()
        .retryServerError(true)
        .retryConnectionError(false)
        .build())
    .build();

Object[] results = client.exec(batch, true, options).get();
```

**Retry on connection errors:**
```java
ClusterBatchOptions options = ClusterBatchOptions.builder()
    .retryStrategy(ClusterBatchRetryStrategy.builder()
        .retryServerError(false)
        .retryConnectionError(true)
        .build())
    .build();

Object[] results = client.exec(batch, true, options).get();
```

**Retry on both:**
```java
ClusterBatchOptions options = ClusterBatchOptions.builder()
    .retryStrategy(ClusterBatchRetryStrategy.builder()
        .retryServerError(true)
        .retryConnectionError(true)
        .build())
    .build();

Object[] results = client.exec(batch, true, options).get();
```

**No retries:**
```java
ClusterBatchOptions options = ClusterBatchOptions.builder()
    .retryStrategy(ClusterBatchRetryStrategy.builder()
        .retryServerError(false)
        .retryConnectionError(false)
        .build())
    .build();

Object[] results = client.exec(batch, true, options).get();
```

---

## Error Handling

### Exception Types
```java
import glide.api.models.exceptions.RequestException;      // Command errors (WRONGTYPE, etc.)
import glide.api.models.exceptions.TimeoutException;      // Request timeout
import glide.api.models.exceptions.ConnectionException;   // Connection issues
```

## Common Pitfalls

### 1. Using Jedis/Lettuce When Working with GLIDE applications
**Problem:** Using legacy Redis clients
**Solution:** Always use `valkey-glide` package

### 2. Missing Platform Classifier
**Problem:** Build fails with missing native library
**Solution:** Use `os-maven-plugin` or `osdetector` for automatic platform detection

### 3. Blocking in Production Code
**Problem:** Using `.get()` or `.join()` blocks threads, kills concurrency
**Solution:** Use async chaining with `.thenCompose()`, `.thenAccept()`, etc.

### 4. Forgetting .get() in Blocking Code
**Problem:** Operations return `CompletableFuture`, not values
**Solution:** Call `.get()` or `.join()` when blocking is acceptable

### 5. Default Timeout Too Short
**Problem:** Connection timeouts on first request
**Solution:** Set explicit `requestTimeout()` in configuration (e.g., 10000ms)

### 6. Wrong Batch Import
**Problem:** Trying to import from `glide.api.models.commands.batch.Batch`
**Solution:** Import from `glide.api.models.Batch`

### 7. Broad Exception Catching
**Problem:** Catching `Exception` instead of specific GLIDE exceptions
**Solution:** Catch `RequestException`, `TimeoutException`, `ConnectionException`

### 8. Not Unwrapping Exceptions in Blocking Code
**Problem:** Catching `ExecutionException` but not checking `getCause()`
**Solution:** Use `e.getCause()` to get actual GLIDE exception, or prefer async

### 9. Using String for Binary Vector Data
**Problem:** Converting vector bytes to String corrupts the data
**Solution:** Use `GlideString.of(byte[])` for binary data like vectors

### 10. Not Checking FT.search Results Array Length
**Problem:** Accessing `results[1]` when count is 0 causes IndexOutOfBoundsException
**Solution:** Check `results.length > 1` before accessing documents map

### 11. CROSSSLOT Errors in Cluster Mode
**Problem:** Atomic batch or multi-key command with keys in different slots
**Solution:** Use hash tags `{tag}` to ensure keys map to same slot, or use non-atomic batch

---

## Vector Search (FT Module)

### Imports
```java
import glide.api.commands.servermodules.FT;
import glide.api.models.commands.FT.FTCreateOptions;
import glide.api.models.commands.FT.FTCreateOptions.FieldInfo;
import glide.api.models.commands.FT.FTCreateOptions.VectorFieldFlat;
import glide.api.models.commands.FT.FTCreateOptions.DistanceMetric;
import glide.api.models.commands.FT.FTSearchOptions;
import glide.api.models.GlideString;
```

### Index Creation
```java
FieldInfo[] schema = new FieldInfo[] {
    new FieldInfo("embedding", 
        VectorFieldFlat.builder(DistanceMetric.COSINE, 768).build())
};
FT.create(client, "my_idx", schema).get();
```

### Store Documents with Vectors
```java
// CRITICAL: Use GlideString for binary vector data
Map<GlideString, GlideString> doc = Map.of(
    GlideString.of("embedding"), GlideString.of(vectorBytes),
    GlideString.of("text"), GlideString.of("content")
);
client.hset(GlideString.of("doc:1"), doc).get();
```

### Vector Search

**⚠️ SECURITY:** The `=>` token in FT.SEARCH syntax separates a filter from a KNN clause. If user-controlled input (e.g., a filter parameter) contains `=>`, an attacker can inject a KNN query that bypasses all filters and returns all documents. Reject `=>` in any user-supplied filter or field name before interpolating into query strings:
```java
if (userFilter != null && userFilter.contains("=>")) {
    throw new IllegalArgumentException("Filter must not contain '=>'");
}
```

```java
String query = "*=>[KNN 5 @embedding $vector AS score]";
FTSearchOptions opts = FTSearchOptions.builder()
    .params(Map.of(GlideString.of("vector"), GlideString.of(queryVectorBytes)))
    .build();

Object[] results = FT.search(client, "my_idx", query, opts).get();
Long count = (Long) results[0];
if (results.length > 1) {
    Map<GlideString, Map<GlideString, GlideString>> docs = 
        (Map<GlideString, Map<GlideString, GlideString>>) results[1];
}
```

### Index Management
```java
// Drop index
FT.dropindex(client, "my_idx").get();

// Get info
Map<String, Object> info = FT.info(client, "my_idx").get();

// List indexes
GlideString[] indexes = FT.list(client).get();
```

### Vector Encoding Helper
```java
private static byte[] floatArrayToBytes(float[] array) {
    ByteBuffer buffer = ByteBuffer.allocate(array.length * 4)
        .order(ByteOrder.LITTLE_ENDIAN);
    for (float f : array) {
        buffer.putFloat(f);
    }
    return buffer.array();
}
```

**Key Points:**
- FT methods are static on `FT` class, not client methods
- Use `GlideString.of()` factory method for binary data
- **CRITICAL:** Binary vectors MUST use `GlideString`, NOT `String` - converting bytes to String corrupts data
- Search returns `Object[]`: `[count, documents_map]`
- Documents map only present if count > 0 - check `results.length > 1`
- Use `ByteOrder.LITTLE_ENDIAN` for vector encoding

---

## Cluster Operations

### Client Selection
```java
// Standalone
import glide.api.GlideClient;
import glide.api.models.configuration.GlideClientConfiguration;
import glide.api.models.Batch;

// Cluster
import glide.api.GlideClusterClient;
import glide.api.models.configuration.GlideClusterClientConfiguration;
import glide.api.models.ClusterBatch;
```

### Hash Slots and CROSSSLOT Errors

In cluster mode, data is distributed across 16384 hash slots. Each key hashes to a specific slot, and slots are distributed across nodes.

**Atomic batches require same slot:**
```java
// Success - hash tags ensure same slot
ClusterBatch batch = new ClusterBatch(true);
batch.set("{user}:1", "Alice");
batch.set("{user}:2", "Bob");
client.exec(batch, true).get();

// Fails - different slots
ClusterBatch batch = new ClusterBatch(true);
batch.set("key1", "value1");  // Slot A
batch.set("key2", "value2");  // Slot B
client.exec(batch, true).get();  // RequestException: CROSSSLOT
```

**Non-atomic batches span slots:**
```java
ClusterBatch pipeline = new ClusterBatch(false);
pipeline.set("key1", "value1");  // Slot A
pipeline.set("key2", "value2");  // Slot B
client.exec(pipeline, true).get();  // Success
```

**Multi-key operations:**
```java
// Same slot - OK
client.del(new String[]{"{user}:1", "{user}:2"}).get();

// Different slots - CROSSSLOT error
client.del(new String[]{"key1", "key2"}).get();  // Fails

// Use non-atomic batch for multi-slot delete
ClusterBatch cleanup = new ClusterBatch(false);
cleanup.del(new String[]{"{user}:1", "{user}:2"});
cleanup.del(new String[]{"key1"});
cleanup.del(new String[]{"key2"});
Object[] results = client.exec(cleanup, true).get();  // [2, 1, 1]
```

**Key Points:**
- Use hash tags `{tag}` to control slot assignment
- Atomic operations require all keys in same slot
- Non-atomic batches automatically route to multiple nodes
- GLIDE splits pipelines per node and reassembles responses

---

## Best Practices

## Client Lifecycle Management

**Spring Boot:**
```java
@Bean(destroyMethod = "close")
public GlideClient glideClient() throws ExecutionException, InterruptedException {
    return GlideClient.createClient(config).get();
}
```

**Plain Java (shutdown hook):**
```java
GlideClient client = GlideClient.createClient(config).get();
Runtime.getRuntime().addShutdownHook(new Thread(client::close));
```

---

# Performance Optimization

Config templates: `../assets/java-config.java`

## AZ Affinity

```java
import glide.api.GlideClusterClient;
import glide.api.models.configuration.GlideClusterClientConfiguration;
import glide.api.models.configuration.ReadFrom;

GlideClusterClientConfiguration config = GlideClusterClientConfiguration.builder()
    .address(NodeAddress.builder()
        .host("cluster.endpoint.cache.amazonaws.com")
        .port(6379)
        .build())
    .readFrom(ReadFrom.AZ_AFFINITY)
    .clientAZ("us-east-1a")
    .requestTimeout(500)
    .build();

GlideClusterClient client = GlideClusterClient.createClient(config).get();
```

## Throughput Tuning

```java
GlideClientConfiguration config = GlideClientConfiguration.builder()
    .address(NodeAddress.builder().host("localhost").port(6379).build())
    .inflightRequestsLimit(2000) // Default: 1000
    .requestTimeout(500)
    .build();
```

## Serverless / Lambda

```java
GlideClientConfiguration config = GlideClientConfiguration.builder()
    .address(NodeAddress.builder().host("localhost").port(6379).build())
    .lazyConnect(true) // Defer connection until first command
    .requestTimeout(500)
    .build();
```

## Retry Strategy

```java
import glide.api.models.configuration.BackoffStrategy;

GlideClientConfiguration config = GlideClientConfiguration.builder()
    .address(NodeAddress.builder().host("localhost").port(6379).build())
    .reconnectStrategy(BackoffStrategy.builder()
        .numberOfRetries(10)
        .factor(500)
        .exponentBase(2)
        .build())
    .requestTimeout(500)
    .build();
```

## Dedicated Blocking Client

```java
GlideClient blockingClient = GlideClient.createClient(
    GlideClientConfiguration.builder()
        .address(NodeAddress.builder().host("localhost").port(6379).build())
        .requestTimeout(30000)
        .clientName("queue-worker")
        .build()
).get();

String[] item = blockingClient.blpop(new String[]{"queue"}, 30).get();
```

---

## Concurrent Operations

```java
CompletableFuture<String> userFuture = client.get("user:123");
CompletableFuture<String[]> postsFuture = client.lrange("posts:123", 0, -1);
CompletableFuture.allOf(userFuture, postsFuture).join();
String user = userFuture.get();
```

## Thread Safety

```java
// ✅ Client shared across threads
private static final GlideClient client = createClient();

// ✅ Batch created per thread (because Batch objects are NOT thread-safe)
Batch batch = new Batch(false);
batch.get("key1");
client.exec(batch, true).get();
```

## Monitoring

### OpenTelemetry

```java
import glide.api.OpenTelemetry;

OpenTelemetry.init(
    OpenTelemetry.OpenTelemetryConfig.builder()
        .traces(OpenTelemetry.TracesConfig.builder()
            .endpoint("http://localhost:4318/v1/traces")
            .samplePercentage(1)
            .build())
        .metrics(OpenTelemetry.MetricsConfig.builder()
            .endpoint("http://localhost:4318/v1/metrics")
            .build())
        .build()
);
```

### Logging

```java
import glide.api.logging.Logger;

Logger.setLoggerConfig(Logger.Level.WARN, "glide.log");  // Production
Logger.setLoggerConfig(Logger.Level.ERROR);               // Max performance
```

Server-side config: `server-configuration-guide.md`
