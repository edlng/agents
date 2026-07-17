# Valkey/ElastiCache Server Configuration Guide

## Cluster Architecture Selection

### When to Use Cluster Mode

Skill analyzes your code patterns to determine if cluster mode would benefit your application:

**Indicators for Cluster Mode:**

- **Multi-key operations across different keys**: Code shows operations on many unrelated keys
- **High throughput requirements**: Code patterns suggest >100K ops/sec
- **Horizontal scaling needs**: Single-node memory limits are insufficient
- **Hash tag usage**: Code already uses `{tag}` syntax indicating cluster-aware design
- **Large dataset**: Code accesses thousands of unique keys

### When to Use Standalone Mode

**Indicators for Standalone Mode:**

- **Single-key or related-key operations**: Most operations on same key or related keys
- **Low to moderate throughput**: <50K ops/sec
- **Simple deployment requirements**: No need for horizontal scaling
- **Transactions across multiple keys**: Heavy use of MULTI/EXEC without hash tags

## Read/Write Workload Analysis

| Workload | Detected Commands | Read Strategy | Replicas |
|----------|------------------|---------------|----------|
| Read-heavy (>80%) | GET, MGET, HGET, HGETALL, SMEMBERS, LRANGE, ZRANGE | Prefer Replica or AZ Affinity | 2-3/shard |
| Write-heavy (>50%) | SET, MSET, HSET, SADD, LPUSH, ZADD, INCR, DEL | Primary | 1/shard (HA only) |
| Balanced (40-60%) | Mixed | Primary | 1-2/shard |

**Read-heavy additional recommendations**: Enable AZ Affinity to reduce cross-AZ costs.
**Write-heavy additional recommendations**: Increase primary node size, use batching to reduce roundtrips.

## Server-Side Configuration Tuning

Based on detected code patterns, the skill recommends server-side Valkey configuration parameters.

### Memory Eviction Policy (`maxmemory-policy`)

| Pattern | Indicator | Policy |
|---------|-----------|--------|
| Cache | SET with TTL, GET, EXPIRE | `allkeys-lru` or `allkeys-lfu` |
| Persistent | SET without TTL, critical data | `noeviction` |
| Mixed TTL | Both persistent and TTL keys | `volatile-lru` or `volatile-lfu` |

### Connection Timeout (`timeout`)

**Long-Lived Connection Patterns:**

Detected: Persistent connections, infrequent operations, connection reuse

**Recommended**: `timeout = 300` (5 minutes) or higher

**Short-Lived Connection Patterns:**

Detected: Frequent reconnections, serverless/Lambda usage

**Recommended**: `timeout = 60` (1 minute)

### TCP Keep-Alive (`tcp-keepalive`)

**Detected Long Idle Periods:**

**Recommended**: `tcp-keepalive = 60` (seconds)

**Benefit**: Prevents firewall/NAT timeout on idle connections

### Max Clients (`maxclients`)

**Detected Concurrency Configuration:**

**Note**: In ElastiCache, `maxclients` has a value of 65,000. For self-managed Valkey/Redis, the default is 10,000 and can be adjusted. When planning capacity, ensure your total connection count across all application instances stays well below this limit.

**Calculation** (for capacity planning): `total_connections = number_of_app_instances × connections_per_instance`

**Example**:
- 10 app instances
- 100 connections per instance
- Total: 1,000 connections (well within the 65,000 limit)

## ElastiCache-Specific Recommendations

### Node Type Selection

Based on detected memory usage patterns and throughput requirements. These recommendations are starting points - actual node type selection should be based on load testing and cost analysis for your specific workload.

**Memory-Intensive Patterns:**

**Recommended Node Types** (AWS ElastiCache):
- **r7g.large**: 13.07 GiB memory
- **r7g.xlarge**: 26.32 GiB memory
- **r7g.2xlarge**: 52.82 GiB memory

**Compute-Intensive Patterns:**

**Recommended Node Types** (AWS ElastiCache):
- **m7g.large**: 6.38 GiB memory
- **m7g.xlarge**: 12.93 GiB memory
- **m7g.2xlarge**: 26.04 GiB memory

**Important**: Throughput varies significantly based on operation types (simple vs complex), payload sizes, and network latency. Conduct load testing to validate node type selection.

### Multi-AZ Deployment

**High Availability Requirements Detected:**

**Recommended**: Enable Multi-AZ replication for automatic failover

**Non-Critical Usage Detected:**

**Recommended**: Single-AZ deployment to reduce costs

### Parameter Group Optimization

**Read-Heavy Workload:**

```
# ElastiCache Parameter Group for Read-Heavy Workloads
maxmemory-policy: allkeys-lru
timeout: 300
tcp-keepalive: 60
```

**Write-Heavy Workload:**

```
# ElastiCache Parameter Group for Write-Heavy Workloads
maxmemory-policy: noeviction
timeout: 60
tcp-keepalive: 30
```

**Note**: In ElastiCache, the `maxclients` parameter is fixed at 65,000 and persistence is managed internally.

**Cache Workload:**

```
# ElastiCache Parameter Group for Cache Workloads
maxmemory-policy: allkeys-lfu
timeout: 300
tcp-keepalive: 60
```

### Cluster Mode Configuration

**Shard Count Recommendations:**

Based on detected key distribution and throughput:

**Shard Count Guidelines**:
- **1-10K keys**: 1 shard (standalone) - sufficient for most small applications
- **10K-100K keys**: 2-3 shards - provides horizontal scaling
- **100K-1M keys**: 3-6 shards - balances distribution and management overhead
- **1M+ keys**: 6-15 shards - for large-scale deployments

**Note**: These are starting points. Actual shard count should consider throughput requirements, data distribution patterns, and operational complexity. More shards increase management overhead but improve horizontal scalability.

**Replicas Per Shard:**

Based on detected read/write ratio:

- **Read-heavy (>80% reads)**: 2-3 replicas per shard
- **Balanced (40-60% reads)**: 1-2 replicas per shard
- **Write-heavy (>50% writes)**: 1 replica per shard (HA only)

## Configuration Examples

### Example 1: High-Traffic Web Application (Read-Heavy)

**Detected Code Pattern:**
```python
# 90% reads, 10% writes, 50K ops/sec
await client.get(f'user:{user_id}')
await client.hgetall(f'profile:{user_id}')
await client.smembers(f'friends:{user_id}')
```

**Recommended ElastiCache Configuration:**

```yaml
Cluster Mode: Enabled
Node Type: r7g.large
Shards: 3
Replicas per Shard: 2
Multi-AZ: Enabled
Parameter Group:
  maxmemory-policy: allkeys-lru
  timeout: 300
  tcp-keepalive: 60
```

**Client Configuration:**
```python
client = await GlideClusterClient.create(
    addresses=[NodeAddress("cluster.endpoint", 6379)],
    request_timeout=500,
    read_from=ReadFrom.AZ_AFFINITY,
    client_az="us-east-1a"
)
```

### Example 2: Real-Time Analytics (Write-Heavy)

**Detected Code Pattern:**
```java
// 70% writes, 30% reads, 100K ops/sec
client.incr("counter:views:" + pageId);
client.zadd("leaderboard", score, userId);
client.lpush("events:stream", event);
```

**Recommended ElastiCache Configuration:**

```yaml
Cluster Mode: Enabled
Node Type: m7g.xlarge
Shards: 6
Replicas per Shard: 1
Multi-AZ: Enabled
Parameter Group:
  maxmemory-policy: noeviction
  timeout: 60
  tcp-keepalive: 30
```

**Client Configuration:**
```java
GlideClusterClient client = GlideClusterClient.createClient(
    GlideClusterClientConfiguration.builder()
        .address(NodeAddress.builder()
            .host("cluster.endpoint")
            .port(6379)
            .build())
        .requestTimeout(500)
        .readFrom(ReadFrom.PRIMARY)  // Avoid replication lag
        .inflightRequestsLimit(2000)
        .build()
).get();
```

### Example 3: Session Store (Balanced)

**Detected Code Pattern:**
```typescript
// 50% reads, 50% writes, 20K ops/sec
await client.set(`session:${sessionId}`, data, { expiryMode: 'EX', expiry: 3600 });
await client.get(`session:${sessionId}`);
```

**Recommended ElastiCache Configuration:**

```yaml
Cluster Mode: Disabled (Standalone)
Node Type: r7g.large
Replicas: 1
Multi-AZ: Enabled
Parameter Group:
  maxmemory-policy: volatile-lru
  timeout: 300
  tcp-keepalive: 60
```

**Client Configuration:**
```typescript
const client = await GlideClient.createClient({
    addresses: [{ host: 'primary.endpoint', port: 6379 }],
    requestTimeout: 500,
    readFrom: "primary"
});
```

### Example 4: Serverless/Lambda Cache

**Detected Code Pattern:**
```go
// Lambda function, infrequent access, lazy connection
cfg := config.NewClientConfiguration().
    WithAddress(&config.NodeAddress{Host: "endpoint", Port: 6379}).
    WithLazyConnect(true)

client, err := glide.NewClient(cfg)
```

**Recommended ElastiCache Configuration:**

```yaml
Cluster Mode: Disabled (Standalone)
Node Type: t4g.small (cost-optimized)
Replicas: 0 (optional: 1 for HA)
Multi-AZ: Disabled (cost optimization)
Parameter Group:
  maxmemory-policy: allkeys-lru
  timeout: 60
  tcp-keepalive: 30
```

**Client Configuration:**
```go
client, err := glide.NewClient(
    config.NewClientConfiguration().
        WithAddress(&config.NodeAddress{Host: "endpoint", Port: 6379}).
        WithRequestTimeout(500).
        WithReconnectStrategy(config.NewBackoffStrategy(
            3,   // numberOfRetries
            200, // factor (base delay in ms)
            2,   // exponentBase
        )),
)
```

## Monitoring Recommendations

After applying server configuration changes, monitor these metrics:

### Key Metrics to Track

1. **Memory Usage**: Should stay below 80% of `maxmemory`
2. **Evictions**: Should be minimal for `noeviction`, expected for LRU/LFU
3. **Connection Count**: Should stay well below 65,000 (ElastiCache fixed `maxclients`)
4. **CPU Utilization**: Should stay below 90% of available CPU (for single-threaded Valkey/Redis, calculate as 90 / number_of_cores for the `CPUUtilization` metric; use `EngineCPUUtilization` directly on 4+ vCPU nodes)
5. **Network Throughput**: Should not saturate node capacity
6. **Replication Lag**: Should be <1 second for read replicas

### CloudWatch Alarms (ElastiCache)

```yaml
Alarms:
  - Metric: DatabaseMemoryUsagePercentage
    Threshold: 80%
    Action: Alert + consider scaling up
  
  - Metric: CPUUtilization
    Threshold: 90% / number_of_cores (e.g., 45% for 2-vCPU nodes)
    Action: Alert + consider scaling up
  
  - Metric: CurrConnections
    Threshold: 80% of 65,000 (ElastiCache default maxclients)
    Action: Alert + investigate connection leaks or scale out
  
  - Metric: Evictions
    Threshold: >0 (for noeviction policy); set application-appropriate threshold for LRU/LFU policies
    Action: Alert + increase memory
  
  - Metric: ReplicationLag
    Threshold: >1 (seconds)
    Action: Alert + investigate replication issues
```

## Additional Resources

- [ElastiCache Best Practices](https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/BestPractices.html)
- [Valkey Configuration](https://valkey.io/topics/config/)
- [AZ Affinity Blog](https://valkey.io/blog/az-affinity-strategy/)
