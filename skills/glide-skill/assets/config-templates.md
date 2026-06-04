# GLIDE Configuration Templates

Production-ready configuration templates for Valkey GLIDE clients across all supported languages.

## Available Templates

- `nodejs-config.ts` - Node.js/TypeScript configuration
- `python-config.py` - Python async/sync configuration
- `java-config.java` - Java configuration
- `go-config.go` - Go configuration
- `php-config.php` - PHP configuration
- `csharp-config.cs` - C# configuration

## Configuration Features

All templates include:

- **Request Timeout**: 500ms (recommended for web apps)
- **Connection Retry Strategy**: Exponential backoff (10 retries, 500ms base, 2x multiplier)
- **Client Name**: Descriptive name for debugging
- **Lazy Connect**: Optional for serverless/Lambda deployments
- **High-Throughput**: `inflightRequestsLimit` set to 2000 (Node.js, Python, Java; not available in Go, PHP, C#)

Cluster templates additionally include:

- **AZ Affinity**: Cost optimization for read-heavy workloads
- **Read Strategy**: Configured for same-AZ reads

## Key Parameter Names Across Languages

| Setting | Node.js | Python | Java | Go | PHP | C# |
|---------|---------|--------|------|----|-----|-----|
| Retry strategy | `connectionBackoff` | `reconnect_strategy` | `reconnectStrategy` | `WithReconnectStrategy` | `reconnect_strategy` | `WithConnectionRetryStrategy` |
| Inflight limit | `inflightRequestsLimit` | `inflight_requests_limit` | `inflightRequestsLimit` | N/A | N/A | N/A |
| Lazy connect | `lazyConnect` | `lazy_connect` | `lazyConnect` | `WithLazyConnect` | `lazy_connect` | `WithLazyConnect` |

## Additional Resources

- [GLIDE Wiki](https://glide.valkey.io/)
- [AZ Affinity Blog](https://valkey.io/blog/az-affinity-strategy/)
