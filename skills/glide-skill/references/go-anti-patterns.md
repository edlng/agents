# Go GLIDE Anti-Patterns

This document contains anti-patterns specific to Go GLIDE development. These patterns should be avoided in production code.

---

## Package Selection

### ❌ INCORRECT: Don't use go-redis
```go
// NEVER use these
import "github.com/redis/go-redis/v9"
```

### ✅ CORRECT: Use GLIDE
```go
import (
	"context"
	glide "github.com/valkey-io/valkey-glide/go/v2"
	"github.com/valkey-io/valkey-glide/go/v2/config"
	"github.com/valkey-io/valkey-glide/go/v2/pipeline"
)
```

**Why:** go-redis is a Redis client. GLIDE is the official recommended client with better performance and active development.

---

## Type Assertions

### ❌ INCORRECT: Unsafe type assertion
```go
// ❌ Unsafe - can panic if wrong type
strVal := results[0].(string)

// ❌ Wrong - panics if not string
str := results[0].(string)
```

### ✅ CORRECT: Safe type assertion
```go
// ✅ Safe type assertion (recommended)
if str, ok := results[0].(string); ok {
	fmt.Println("String result:", str)
} else {
	// Handle unexpected type
}

// ✅ Correct - safe type assertion
if str, ok := results[0].(string); ok {
	fmt.Println("Result:", str)
} else {
	// Handle unexpected type
	fmt.Println("Unexpected type")
}
```

**Why:** Direct assertions can panic if the type is wrong. Always use the two-value form for safety.

---

## Error Handling

### ❌ INCORRECT: Hiding error handling
```go
// ❌ Don't try to hide error handling
// No "smart" wrappers or generic error handlers
```

### ✅ CORRECT: Explicit error handling
```go
// ✅ Explicit error handling (Go way)
value, err := client.Get(ctx, "key")
if err != nil {
	return fmt.Errorf("failed to get key: %w", err)
}
```

**Why:** Go GLIDE follows Go's philosophy of explicit, obvious code.

---

### ❌ INCORRECT: Losing error context
```go
// ❌ Don't lose error context
if err != nil {
	return err  // What failed? Which key?
}
```

### ✅ CORRECT: Wrap errors with context
```go
// ✅ Wrap errors with context
value, err := client.Get(ctx, userKey)
if err != nil {
	return fmt.Errorf("failed to fetch user %s: %w", userID, err)
}
```

**Why:** Error wrapping provides context for debugging.

---

## Configuration

### ❌ INCORRECT: Struct tag magic
```go
// ❌ Not like this (anti-pattern from other ORMs)
// type Config struct {
//     Host string `valkey:"host"`
//     Port int    `valkey:"port"`
// }
```

### ✅ CORRECT: Explicit configuration
```go
// ✅ Explicit configuration
cfg := config.NewClientConfiguration().
	WithAddress(&config.NodeAddress{Host: "localhost", Port: 6379}).
	WithRequestTimeout(10000)
```

**Why:** Go GLIDE doesn't use struct tags for configuration. Be explicit.

---

## Model Design

### ❌ INCORRECT: Combining responsibilities
```go
// ❌ Don't combine responsibilities
// type User struct {
//     ID   int    `json:"id" cache:"id"`
//     Name string `json:"name" cache:"name"`
// }
```

### ✅ CORRECT: Separate models
```go
// ✅ Separate models
type UserAPIResponse struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
}

type UserCacheModel struct {
	ID        int
	Name      string
	UpdatedAt time.Time
}

// Convert between them explicitly
func toAPIResponse(cache UserCacheModel) UserAPIResponse {
	return UserAPIResponse{
		ID:   cache.ID,
		Name: cache.Name,
	}
}
```

**Why:** Separate concerns for different layers of your application.

---

## Context Usage

### ✅ CORRECT: Production timeout context
```go
// ✅ Production: timeout context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

value, err := client.Get(ctx, "key")
if err != nil {
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("operation timed out")
	}
	return err
}
```

### ✅ CORRECT: Simple demos background context
```go
// ✅ Simple demos: background context
ctx := context.Background()
value, err := client.Get(ctx, "key")
```

**Why:** Use timeout contexts in production for proper cancellation handling.

---

## Performance Optimization

### ❌ INCORRECT: Fetching entire JSON object
```go
// ❌ Inefficient — must fetch/parse entire object
data, _ := json.Marshal(user)
client.Set(ctx, "user:123", string(data))
```

### ✅ CORRECT: Use Hash for structured data
```go
// ✅ Efficient — fetch only needed fields
client.HSet(ctx, "user:123", map[string]string{"name": "John", "email": "john@example.com"})
name, _ := client.HGet(ctx, "user:123", "name")
```

**Why:** Fetching only needed fields reduces network transfer and parsing overhead.

---

---

## FT.SEARCH Query Injection via `=>` Delimiter

The `=>` token in FT.SEARCH syntax separates a filter from a KNN vector clause. If user-controlled input is interpolated into query strings without sanitization, an attacker can inject a KNN clause that bypasses all filters and returns all documents.

### ❌ INCORRECT: Interpolating user input without sanitization
```go
// ❌ VULNERABLE — userFilter could contain "=>[KNN ...]"
query := fmt.Sprintf("(%s) %s", userFilter, searchText)
```

### ✅ CORRECT: Reject `=>` before query construction
```go
if strings.Contains(userFilter, "=>") {
    return nil, fmt.Errorf("filter must not contain '=>'")
}
query := fmt.Sprintf("(%s) %s", userFilter, searchText)
```

**Why:** There is no built-in escaping in Valkey Search syntax. Treat `=>` as a reserved delimiter and reject it in all user-controlled query fragments.