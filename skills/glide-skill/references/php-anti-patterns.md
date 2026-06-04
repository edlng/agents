# PHP GLIDE Anti-Patterns

This document contains anti-patterns specific to PHP GLIDE development. These patterns should be avoided in production code.

---

## Common Pitfalls

### ❌ INCORRECT: Forgetting exec()
```php
// ❌ Wrong - commands queued but not executed
$client->multi();
$client->set('key', 'value');
```

### ✅ CORRECT: Always call exec()
```php
// ✅ Correct
$client->multi();
$client->set('key', 'value');
$results = $client->exec();
```

**Why:** Commands are queued but not executed until `exec()` is called.

---

### ❌ INCORRECT: CROSSSLOT in Transactions
```php
// ❌ Wrong - different slots
$client->multi();
$client->set('key1', 'value1');
$client->set('key2', 'value2');
$client->exec();  // Error
```

### ✅ CORRECT: Use hash tags
```php
// ✅ Correct - use hash tags
$client->multi();
$client->set('{user}:1', 'value1');
$client->set('{user}:2', 'value2');
$client->exec();
```

**Why:** Atomic operations in cluster mode require all keys in the same slot.

---

### ❌ INCORRECT: Assuming extension is loaded
```php
// ❌ Wrong - assuming extension is loaded
$client = new ValkeyGlide();
```

### ✅ CORRECT: Check extension first
```php
// ✅ Correct - check first
if (!extension_loaded('valkey_glide')) {
    die('valkey_glide extension not loaded. Add extension=valkey_glide to php.ini');
}
$client = new ValkeyGlide();
```

**Why:** Provides clear error message if extension is not loaded.

---

### ❌ INCORRECT: Wrong Client for Cluster
```php
// ❌ Wrong
$client = new ValkeyGlide();
$client->connect(addresses: [['host' => 'localhost', 'port' => 7000]]);
```

### ✅ CORRECT: Use ValkeyGlideCluster
```php
// ✅ Correct
$client = new ValkeyGlideCluster(
    addresses: [['host' => 'localhost', 'port' => 7000]]
);
```

**Why:** Cluster endpoints require ValkeyGlideCluster for proper slot routing.

---

## Performance Optimization

### ❌ INCORRECT: Fetching entire JSON object
```php
// ❌ Inefficient — must fetch/parse entire object
$client->set('user:123', json_encode($userData));
$email = json_decode($client->get('user:123'), true)['email'];
```

### ✅ CORRECT: Use Hash for structured data
```php
// ✅ Efficient — fetch only needed fields
$client->hSet('user:123', 'name', 'John', 'email', 'john@example.com', 'age', '30');
$email = $client->hGet('user:123', 'email');
```

**Why:** Fetching only needed fields reduces network transfer and parsing overhead.

---

## Architecture Patterns

### ❌ INCORRECT: God Object
```php
class ValkeyManager {
    // User management
    public function createUser($userId, $data) { }
    
    // Session management
    public function createSession($sessionId, $userId) { }
    
    // Cache management
    public function cacheData($key, $value, $ttl) { }
    
    // Analytics
    public function trackEvent($event) { }
}
```

### ✅ CORRECT: Single Responsibility Principle
```php
class UserRepository {
    private $client;
    
    public function __construct($client) {
        $this->client = $client;
    }
    
    public function createUser($userId, $data) {
        $this->client->set("user:$userId", json_encode($data));
    }
}

class SessionRepository {
    private $client;
    
    public function __construct($client) {
        $this->client = $client;
    }
    
    public function createSession($sessionId, $userId) {
        $this->client->set("session:$sessionId", $userId);
    }
}
```

**Why:** God objects violate Single Responsibility Principle, making code hard to maintain and test.

---

### ❌ INCORRECT: If-Else Chains
```php
class CacheManager {
    public function cache($key, $value, $strategy) {
        if ($strategy === 'short') {
            $this->client->setex($key, 60, $value);
        } elseif ($strategy === 'medium') {
            $this->client->setex($key, 3600, $value);
        }
        // Must modify this method to add new strategies!
    }
}
```

### ✅ CORRECT: Strategy Pattern (Open-Closed Principle)
```php
interface CacheStrategy {
    public function cache($client, $key, $value);
}

class ShortCacheStrategy implements CacheStrategy {
    public function cache($client, $key, $value) {
        $client->setex($key, 60, $value);
    }
}

class CacheManager {
    public function __construct($client, CacheStrategy $strategy) {
        $this->strategy = $strategy;
    }
}
```

**Why:** If-else chains violate Open-Closed Principle - must modify code to extend behavior.

---

### ❌ INCORRECT: Tight Coupling
```php
class UserService {
    public function __construct() {
        // Tightly coupled to ValkeyGlide
        $this->client = new ValkeyGlide();
        $this->client->connect(...);
    }
}
```

### ✅ CORRECT: Dependency Injection (Dependency Inversion Principle)
```php
interface CacheClient {
    public function get($key);
    public function set($key, $value);
}

class ValkeyGlideAdapter implements CacheClient {
    private $client;
    
    public function __construct() {
        $this->client = new ValkeyGlide();
        $this->client->connect(...);
    }
    
    public function get($key) {
        return $this->client->get($key);
    }
}

class UserService {
    public function __construct(CacheClient $cache) {
        $this->cache = $cache;
    }
}
```

**Why:** Tight coupling makes testing difficult and prevents swapping implementations.

---

---

## FT.SEARCH Query Injection via `=>` Delimiter

The `=>` token in FT.SEARCH syntax separates a filter from a KNN vector clause. If user-controlled input is interpolated into query strings without sanitization, an attacker can inject a KNN clause that bypasses all filters and returns all documents.

### ❌ INCORRECT: Interpolating user input without sanitization
```php
// ❌ VULNERABLE — $userFilter could contain "=>[KNN ...]"
$query = "({$userFilter}) {$searchText}";
```

### ✅ CORRECT: Reject `=>` before query construction
```php
if ($userFilter && str_contains($userFilter, '=>')) {
    throw new \InvalidArgumentException("Filter must not contain '=>'");
}
$query = "({$userFilter}) {$searchText}";
```

**Why:** There is no built-in escaping in Valkey Search syntax. Treat `=>` as a reserved delimiter and reject it in all user-controlled query fragments.