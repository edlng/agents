# Java GLIDE Anti-Patterns

This document contains anti-patterns specific to Java GLIDE development. These patterns should be avoided in production code.

---

## Package Selection

### ❌ INCORRECT: Don't use Jedis or Lettuce
```java
// NEVER use these
import redis.clients.jedis.*;
import io.lettuce.core.*;
```

**Why:** Jedis and Lettuce are Redis clients. GLIDE is the official recommended client with better performance and active development.

---

## Resource Management

### ❌ INCORRECT: Resource leak
```java
GlideClient client = GlideClient.createClient(config).get();
client.set("key", "value").get();
// Client never closed!
```

### ✅ CORRECT: Use try-with-resources
```java
try (GlideClient client = GlideClient.createClient(config).get()) {
    client.set("key", "value").get();
}
```

**Why:** Without try-with-resources, connections leak and exhaust the connection pool.

---

## Concurrency

### ❌ INCORRECT: Swallow InterruptedException
```java
try {
    String value = future.get();
} catch (InterruptedException e) {
    // Ignoring - thread can't be stopped!
}
```

### ✅ CORRECT: Restore interrupt status
```java
try {
    String value = future.get();
} catch (InterruptedException e) {
    Thread.currentThread().interrupt();
    // Handle interruption
}
```

**Why:** Swallowing interruption breaks thread cancellation and prevents graceful shutdown.

---

## Exception Handling (Blocking Calls)

### ❌ INCORRECT: Catch wrapped exception directly
```java
try {
    client.lpop("string:key").get();
} catch (RequestException e) {
    // Never reached - wrapped in ExecutionException!
}
```

### ✅ CORRECT: Unwrap ExecutionException
```java
try {
    client.lpop("string:key").get();
} catch (ExecutionException e) {
    if (e.getCause() instanceof RequestException) {
        System.out.println("Error: " + e.getCause().getMessage());
    }
}
```

**Why:** CompletableFuture wraps exceptions in ExecutionException for blocking calls.

---

## Exception Handling (Async Chains)

### ✅ CORRECT: Direct exception access
```java
client.lpop("string:key")
    .exceptionally(e -> {
        if (e instanceof RequestException) {
            System.out.println("Error: " + e.getMessage());
        }
        return null;
    });
```

**Why:** Async chains provide direct exception access - no unwrapping needed.

---

## Exception Handling (General)

### ❌ INCORRECT: Broad Exception Handling
```java
// DON'T catch Exception - too broad
try {
    client.get("key").get();
} catch (Exception e) {
    // Too vague
}
```

### ✅ CORRECT: Specific Exception Handling

**Async:**
```java
client.lpop("string_key")
    .exceptionally(e -> {
        Throwable cause = (e instanceof CompletionException && e.getCause() != null) 
            ? e.getCause() : e;
        
        if (cause instanceof RequestException) {
            System.err.println("Request error: " + cause.getMessage());
        } else if (cause instanceof TimeoutException) {
            System.err.println("Timeout: " + cause.getMessage());
        }
        return null;
    });
```

**Blocking:**
```java
try {
    client.lpop("string_key").get();
} catch (ExecutionException e) {
    if (e.getCause() instanceof RequestException) {
        System.err.println("Request error: " + e.getCause().getMessage());
    }
}
```

**Why:** Broad catches hide bugs and make debugging difficult.

---

## Performance Optimization

### ❌ INCORRECT: Fetching entire JSON object
```java
// ❌ Inefficient — must fetch/parse entire object
client.set("user:123", mapper.writeValueAsString(user)).get();
Map<String, Object> parsed = mapper.readValue(client.get("user:123").get(), Map.class);
```

### ✅ CORRECT: Use Hash for structured data
```java
// ✅ Efficient — fetch only needed fields
client.hset("user:123", Map.of("name", "John", "email", "john@example.com", "age", "30")).get();
String name = client.hget("user:123", "name").get();
```

**Why:** Fetching only needed fields reduces network transfer and parsing overhead.

---

## Design Patterns

### ❌ INCORRECT: Primitive obsession
```java
void storeUser(GlideClient client, String userId, String sessionId) {
    // Easy to swap parameters - compiles but wrong!
}
```

### ✅ CORRECT: Type-safe value objects
```java
class UserId {
    private final String value;
    public UserId(String value) {
        if (!value.startsWith("user:")) {
            throw new IllegalArgumentException("Invalid user ID");
        }
        this.value = value;
    }
    public String getValue() { return value; }
}

void storeUser(GlideClient client, UserId userId, SessionId sessionId) {
    // Compiler prevents parameter swap
}
```

**Why:** Primitives lack type safety and allow parameter order mistakes.

---

## Testing

### ❌ INCORRECT: Depend on external Valkey
```java
@Test
void testUserService() {
    GlideClient client = GlideClient.createClient(config).get();
    // Flaky - depends on external state
}
```

### ✅ CORRECT: Use test doubles
```java
@Test
void testUserService() {
    GlideClient mockClient = mock(GlideClient.class);
    when(mockClient.get("user:1")).thenReturn(CompletableFuture.completedFuture("Alice"));
    // Test with mock
}
```

**Why:** Tests depending on external services are non-deterministic and flaky.

---
