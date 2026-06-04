# PHP GLIDE Skill

## External Resources
- `../assets/php-config.php` - Client connection config templates, TLS/SSL, authentication (password, username, AWS IAM), cluster, standalone, PHPRedis compatibility, etc.
- `php-anti-patterns.md` - Anti-patterns to avoid in PHP GLIDE development including Hash vs JSON performance patterns, and more

## Package Selection

```php
// ✅ Correct - PHP Extension
extension=valkey_glide

// Check if loaded
if (!extension_loaded('valkey_glide')) {
    die('valkey_glide extension not loaded');
}

// ❌ Wrong - Don't use PHPRedis directly
// extension=redis
```

**Installation**: PHP GLIDE is a C extension, not a Composer package. Install via PECL, pie, or build from source. See [Packagist](https://packagist.org/packages/valkey-io/valkey-glide-php) for details.

## Synchronous API

All operations are synchronous (blocking):

```php
$value = $client->get('key');
$client->set('key', 'value');
$client->del(['key']);
```

## Batch/Pipeline Operations

### Non-Atomic Pipeline
```php
$pipeline = $client->pipeline();
$pipeline->set('key1', 'value1');
$pipeline->set('key2', 'value2');
$pipeline->get('key1');
$pipeline->get('key2');
$results = $client->exec();
// Returns: [true, true, "value1", "value2"]
```

### Atomic Transaction
```php
$client->multi();
$client->set('counter', '0');
$client->incr('counter');
$client->incr('counter');
$client->get('counter');
$results = $client->exec();
// Returns: [true, 1, 2, "2"]
```

### Retry Strategies

**Note:** PHP GLIDE v1.0.0 does not support batch retry strategies (`retryServerError`, `retryConnectionError`). This feature may be added in future versions.

For production resilience, implement retry logic at the application level:

```php
function executeWithRetry($client, callable $operation, int $maxRetries = 3): mixed {
    $attempt = 0;
    while ($attempt < $maxRetries) {
        try {
            return $operation($client);
        } catch (Exception $e) {
            $attempt++;
            if ($attempt >= $maxRetries) {
                throw $e;
            }
            usleep(100000 * $attempt); // Exponential backoff
        }
    }
}

// Usage
$results = executeWithRetry($client, function($c) {
    $c->multi();
    $c->set('key', 'value');
    $c->get('key');
    return $c->exec();
});
```

See SKILL.md for retry strategy decision matrix (applicable when feature becomes available).

## Cluster Operations

### Hash Tags for Slot Control
```php
// Keys with same hash tag go to same slot
$client->set('{user}:1:name', 'Alice');
$client->set('{user}:1:email', 'alice@example.com');

// Atomic transaction works
$client->multi();
$client->get('{user}:1:name');
$client->get('{user}:1:email');
$results = $client->exec();
```

### Multi-Slot Operations
```php
// Non-atomic pipeline for different slots
$pipeline = $client->pipeline();
$pipeline->set('key1', 'value1');
$pipeline->set('key2', 'value2');
$pipeline->get('key1');
$results = $client->exec();
```

## Error Handling

```php
// Some errors are printed to stderr
$client->lpush('string_key', 'value');
// Prints: "Error executing command: WRONGTYPE..."

// Some errors throw exceptions
try {
    $client->multi();
    $client->set('key1', 'value1');  // Different slots
    $client->set('key2', 'value2');
    $client->exec();
} catch (Exception $e) {
    echo "CROSSSLOT error: " . $e->getMessage();
}
```

---

## Common Pitfalls

### 1. Forgetting exec()
**Problem:** Commands queued but not executed
**Solution:** Always call `exec()` after `multi()`

### 2. CROSSSLOT in Transactions
**Problem:** Atomic operations with keys in different slots
**Solution:** Use hash tags `{tag}` to ensure same slot

### 3. Extension Not Loaded
**Problem:** Assuming extension is loaded
**Solution:** Check with `extension_loaded('valkey_glide')` first

### 4. Wrong Client for Cluster
**Problem:** Using `ValkeyGlide` for cluster endpoints
**Solution:** Use `ValkeyGlideCluster` for cluster mode

---

## Client Lifecycle Management

One client per PHP-FPM worker (not per request). Use a static property or global:

```php
class Cache {
    private static ?ValkeyGlide $client = null;
    public static function client(): ValkeyGlide {
        if (self::$client === null) {
            self::$client = new ValkeyGlide();
            self::$client->connect(addresses: [['host' => 'localhost', 'port' => 6379]], request_timeout: 500);
        }
        return self::$client;
    }
}
```

---

# Performance Optimization

```php
use ValkeyGlide\OpenTelemetry\OpenTelemetryConfig;
use ValkeyGlide\OpenTelemetry\TracesConfig;
use ValkeyGlide\OpenTelemetry\MetricsConfig;

$otelConfig = OpenTelemetryConfig::builder()
    ->traces(TracesConfig::builder()
        ->endpoint('http://localhost:4318/v1/traces')
        ->samplePercentage(1)
        ->build())
    ->metrics(MetricsConfig::builder()
        ->endpoint('http://localhost:4318/v1/metrics')
        ->build())
    ->build();

// Adjust sampling at runtime:
ValkeyGlide::setOtelSamplePercentage(10);
```

Recommended sampling: 1-10% production, 25-50% staging, 100% development.

Server-side config: `server-configuration-guide.md`
