# C# GLIDE Skill

## External Resources
- `../assets/csharp-config.cs` - Client connection config templates, TLS/SSL, authentication (password, username, AWS IAM), cluster, standalone, database selection, protocol version, etc.
- `csharp-anti-patterns.md` - Anti-patterns to avoid in C# GLIDE development including CROSSSLOT error patterns, Hash vs JSON performance, and more.

> **Status:** Preview - C# GLIDE is available on NuGet but still has features being implemented before GA. See [official documentation](https://valkey.io/valkey-glide/) for latest updates.

## Package Selection

```csharp
// ✅ Correct
using Valkey.Glide;
using Valkey.Glide.Pipeline;
using static Valkey.Glide.ConnectionConfiguration;

// ❌ Wrong
using StackExchange.Redis;  // Different library (though Valkey.Glide provides compatibility layer)
```

**Note:** Current NuGet version is 0.9.0. Features like IAM authentication and insecure TLS mode require version 2.0+ (not yet released).

## Async Patterns

All operations return `Task<T>`. Use async/await:

```csharp
async Task Example()
{
    var value = await client.StringGetAsync("key");
    await client.StringSetAsync("key", "value");
}
```

**Key Point:** Unlike Java's `CompletableFuture.get()`, C# uses `await` - no blocking calls needed.

## Batch/Pipeline Operations

### Standalone Batch
```csharp
// Atomic batch (transaction)
var batch = new Batch(isAtomic: true);
batch.StringSet("key1", "value1");
batch.StringGet("key1");
var results = await client.Exec(batch, raiseOnError: true);

// Non-atomic pipeline
var pipeline = new Batch(isAtomic: false);
pipeline.StringSet("key1", "value1");
pipeline.StringSet("key2", "value2");
var results = await client.Exec(pipeline, raiseOnError: true);
```

### Cluster Batch
```csharp
// Atomic batch (requires same slot)
var batch = new ClusterBatch(isAtomic: true);
batch.StringSet("{user}:1", "Alice");
batch.StringGet("{user}:1");
var results = await client.Exec(batch, raiseOnError: true);

// Non-atomic pipeline (can span slots)
var pipeline = new ClusterBatch(isAtomic: false);
pipeline.StringSet("key1", "value1");
pipeline.StringSet("key2", "value2");
var results = await client.Exec(pipeline, raiseOnError: true);
```

**Key Points:**
- Use named parameter `isAtomic:` for clarity
- Results are `object?[]?` - nullable array of nullable objects
- Cluster atomic batches require same hash slot

### Retry Strategies

**Note:** C# GLIDE v0.9.0 does not support batch retry strategies. This feature may be added in future versions.

For production resilience, implement retry logic at the application level:

```csharp
async Task<T> ExecuteWithRetryAsync<T>(
    Func<Task<T>> operation,
    int maxRetries = 3,
    int baseDelayMs = 100)
{
    for (int attempt = 0; attempt < maxRetries; attempt++)
    {
        try
        {
            return await operation();
        }
        catch (Exception ex) when (attempt < maxRetries - 1)
        {
            await Task.Delay(baseDelayMs * (int)Math.Pow(2, attempt));
        }
    }
    throw new InvalidOperationException("Max retries exceeded");
}

// Usage
var results = await ExecuteWithRetryAsync(async () =>
{
    var batch = new ClusterBatch(isAtomic: false);
    batch.StringSetAsync("key", "value");
    return await client.Exec(batch, raiseOnError: true);
});
```

See SKILL.md for retry strategy decision matrix (applicable when feature becomes available).

## Cluster Operations

### Hash Tags for Slot Control
```csharp
// Use {tag} to ensure keys map to same slot
await client.StringSetAsync("{user}:1:name", "Alice");
await client.StringSetAsync("{user}:1:email", "alice@example.com");

// Atomic batch requires same slot
var batch = new ClusterBatch(isAtomic: true);
batch.StringSetAsync("{order}:100:status", "pending");
batch.StringSet("{order}:100:total", "99.99");
await client.Exec(batch, raiseOnError: true);
```

### CROSSSLOT Error

Atomic batches require all keys in the same slot. Non-atomic batches can span slots.

## Error Handling

```csharp
using static Valkey.Glide.Errors;
// Note: alias Valkey.Glide.Errors.TimeoutException to avoid conflict with System.TimeoutException
using TimeoutException = Valkey.Glide.Errors.TimeoutException;

try
{
    await client.StringSetAsync("key", "value");
    var result = await client.StringGetAsync("key");
}
catch (ConnectionException ex)
{
    Console.WriteLine($"Connection error: {ex.Message}");
}
catch (TimeoutException ex)
{
    Console.WriteLine($"Timeout: {ex.Message}");
}
catch (RequestException ex)
{
    Console.WriteLine($"Request error: {ex.Message}");
}
```

**Key Point:** Direct exception access (no wrapping like Java's `ExecutionException`). Exceptions are nested in `Valkey.Glide.Errors` — use `using static Valkey.Glide.Errors;` for convenience.

## PubSub Operations

### Configuration-Time Subscriptions
```csharp
var config = new StandaloneClientConfigurationBuilder()
    .WithAddress("localhost", 6379)
    .WithPubSubReconciliationInterval(TimeSpan.FromSeconds(1))
    .WithPubSubSubscriptionConfig(new StandalonePubSubSubscriptionConfig()
        .WithChannel("alerts")
        .WithPattern("log:*")
        .WithCallback((msg, ctx) => {
            Console.WriteLine($"Received: {msg.Message}");
        }))
    .Build();

await using var client = await GlideClient.CreateClient(config);
```

### Dynamic Subscribe/Unsubscribe
```csharp
await client.PSubscribeAsync("news*");
await client.UnsubscribeAsync("alerts");
```

### Publishing
```csharp
await client.PublishAsync("channel", "message");
```

## StackExchange.Redis Compatibility

Valkey.Glide provides compatibility layer:

```csharp
// Compatible with StackExchange.Redis API
var connection = await ConnectionMultiplexer.ConnectAsync("localhost:6379");
var db = connection.GetDatabase();

await db.StringSetAsync("key", "value");
var value = await db.StringGetAsync("key");
```

**Key Point:** Eases migration from StackExchange.Redis to Valkey.Glide

## Client Lifecycle Management

**ASP.NET Core:**

Setup client:
```csharp
// Program.cs
builder.Services.AddSingleton<GlideClient>(_ =>
    GlideClient.CreateClient(config).GetAwaiter().GetResult());

// Shutdown via IHostedService.StopAsync or await using for scripts
await using var client = await GlideClient.CreateClient(config);
```

Clean-up resources:
```csharp
await using var client = await GlideClient.CreateClient(config);
try
{
    // Test operations
    await client.StringSetAsync("test:key", "value");
}
finally
{
    await client.Del(["test:key"]);
}
```

---

# Performance Optimization

Config templates: `../assets/csharp-config.cs`

`inflightRequestsLimit` not exposed in C# — managed at Rust core level (default: 1000). Focus on batching and `Task.WhenAll`.

## AZ Affinity

```csharp
using static Valkey.Glide.ConnectionConfiguration;

var config = new ClusterClientConfigurationBuilder()
    .WithAddress("cluster.endpoint.cache.amazonaws.com", 6379)
    .WithReadFrom(new ReadFrom(ReadFromStrategy.AzAffinity, "us-east-1a"))
    .WithRequestTimeout(TimeSpan.FromMilliseconds(500))
    .WithConnectionRetryStrategy(numberOfRetries: 10, factor: 500, exponentBase: 2)
    .WithClientName("my-app-cluster")
    .Build();

await using var client = await GlideClusterClient.CreateClient(config);
```

## Serverless / Lambda

```csharp
var config = new StandaloneClientConfigurationBuilder()
    .WithAddress(Environment.GetEnvironmentVariable("CACHE_ENDPOINT")!, 6379)
    .WithRequestTimeout(TimeSpan.FromMilliseconds(500))
    .WithLazyConnect(true)  // Defer TCP+TLS handshake until first command
    .WithClientName("lambda-handler")
    .WithConnectionRetryStrategy(numberOfRetries: 3, factor: 500, exponentBase: 2)
    .Build();
```

## Retry Strategy

```csharp
var config = new StandaloneClientConfigurationBuilder()
    .WithAddress("localhost", 6379)
    .WithRequestTimeout(TimeSpan.FromMilliseconds(500))
    .WithConnectionRetryStrategy(
        numberOfRetries: 10,
        factor: 500,        // Base delay in ms
        exponentBase: 2     // Exponential backoff
    )
    .Build();
```

## Dedicated Blocking Client

```csharp
var blockingConfig = new StandaloneClientConfigurationBuilder()
    .WithAddress("localhost", 6379)
    .WithRequestTimeout(TimeSpan.FromSeconds(30))
    .WithClientName("queue-worker")
    .Build();

await using var blockingClient = await GlideClient.CreateClient(blockingConfig);
var item = await blockingClient.ListBlockingLeftPopAsync(
    new ValkeyKey[] { "queue" }, TimeSpan.FromSeconds(30));
```

## Typed Error Handling

```csharp
using static Valkey.Glide.Errors;

try
{
    var value = await client.StringGetAsync("key");
}
catch (TimeoutException ex)
{
    // Retry with exponential backoff
}
catch (ConnectionException ex)
{
    // Transient — client will auto-reconnect; use circuit breaker pattern
}
catch (RequestException ex)
{
    // Server-side error (WRONGTYPE, etc.)
}
catch (ExecAbortException ex)
{
    // Transaction aborted
}
catch (ConfigurationError ex)
{
    // Invalid configuration — fix config and recreate client
}
```

## Hash vs JSON for Structured Data

## Concurrent Operations

```csharp
// Task.WhenAll for concurrent independent operations
var userTask = client.StringGetAsync("user:123");
var settingsTask = client.StringGetAsync("settings:123");
var statsTask = client.StringGetAsync("stats:123");

await Task.WhenAll(userTask, settingsTask, statsTask);

var user = userTask.Result;
var settings = settingsTask.Result;
var stats = statsTask.Result;
```

## Thread Safety

```csharp
// ✅ Batch created per scope (because Batch objects are NOT thread-safe — create one per operation scope.)
async Task ProcessAsync(GlideClient client)
{
    var batch = new Batch(isAtomic: false);
    batch.StringSet("key1", "value1");
    batch.StringGet("key1");
    await client.Exec(batch, raiseOnError: true);
}
```

## ASP.NET Core Integration

```csharp
// Program.cs — register as singleton
builder.Services.AddSingleton<GlideClient>(sp =>
    GlideClient.CreateClient(config).GetAwaiter().GetResult());

// Graceful shutdown via IHostedService
public class GlideShutdownService : IHostedService
{
    private readonly GlideClient _client;
    public GlideShutdownService(GlideClient client) => _client = client;
    public Task StartAsync(CancellationToken ct) => Task.CompletedTask;
    public async Task StopAsync(CancellationToken ct) => await _client.DisposeAsync();
}
```

## Monitoring

### OpenTelemetry

```csharp
// OpenTelemetry integration — check latest Valkey.Glide docs for C# API
// The Rust core emits traces/metrics; configure the OTel exporter at startup
```

### Logging

```csharp
// Set log level for production (reduce noise)
// Valkey.Glide uses the Rust core logger — configure via environment or API
```

Server-side config: `server-configuration-guide.md`
