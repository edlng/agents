<?php
// PHP GLIDE Configuration Template
// Optimized for production web applications
//
// This file contains all client connection patterns:
//   - Standalone and cluster configs
//   - PHPRedis compatibility aliases
//   - Password and username/password authentication
//   - TLS with CA-signed certificates
//   - AWS ElastiCache/MemoryDB IAM authentication (GLIDE 2.2+)
//   - Production tuning (timeouts, retry, throughput, AZ affinity)

// ===========================================================================
// Standalone Client Configuration
// ===========================================================================
// ValkeyGlide uses a two-step pattern: construct (no args), then connect()
// connect() supports both PHPRedis-style positional params and GLIDE-style named params.
function createStandaloneClient(): ValkeyGlide {
    $client = new ValkeyGlide();
    $client->connect(
        // PHPRedis-compatible positional parameters (positions 0-5):
        //   $host, $port, $timeout, $persistent_id, $retry_interval, $read_timeout
        // These are available for drop-in PHPRedis compatibility.
        // For new code, prefer the GLIDE-style named parameters below.

        addresses: [
            ['host' => 'localhost', 'port' => 6379]
        ],

        // Request timeout in milliseconds (500ms recommended for web apps)
        request_timeout: 500,

        // Connection retry strategy
        reconnect_strategy: [
            'num_of_retries' => 10,
            'factor' => 2,           // Delay multiplier
            'exponent_base' => 2,    // Exponential backoff base
            'jitter_percent' => 15,  // Random jitter to avoid thundering herd
        ],

        // Client name for debugging (visible in CLIENT LIST)
        client_name: 'my-app-client',

        // Database index (0-15 for standalone)
        database_id: 0,

        // Lazy connect for serverless/PHP-FPM (defers connection until first command)
        lazy_connect: true,
    );
    return $client;
}

// ===========================================================================
// Cluster Client Configuration
// ===========================================================================
// ValkeyGlideCluster uses a one-step pattern: all config in constructor.
// The constructor supports both PHPRedis RedisCluster-style positional params
// ($name, $seeds, $timeout, $read_timeout, $persistent, $auth, $context)
// and GLIDE-style named params. Cannot mix the two styles.
function createClusterClient(): ValkeyGlideCluster {
    return new ValkeyGlideCluster(
        addresses: [
            ['host' => 'cluster.endpoint.cache.amazonaws.com', 'port' => 6379]
        ],

        // TLS configuration
        use_tls: false,

        // Request timeout in milliseconds
        request_timeout: 500,

        // AZ Affinity for cost optimization (read-heavy workloads)
        // Constants are defined on ValkeyGlide, used for both standalone and cluster
        read_from: ValkeyGlide::READ_FROM_AZ_AFFINITY,
        client_az: 'us-east-1a',  // Your application's AZ

        // Connection retry strategy
        reconnect_strategy: [
            'num_of_retries' => 10,
            'factor' => 2,
            'exponent_base' => 2,
            'jitter_percent' => 15,
        ],

        // Client name
        client_name: 'my-app-cluster-client',

        // Periodic topology checks (cluster-specific)
        // ValkeyGlideCluster::PERIODIC_CHECK_ENABLED_DEFAULT_CONFIGS (0) = enabled with defaults
        // ValkeyGlideCluster::PERIODIC_CHECK_DISABLED (1) = disabled
        periodic_checks: ValkeyGlideCluster::PERIODIC_CHECK_ENABLED_DEFAULT_CONFIGS,

        // Lazy connect
        lazy_connect: true,
    );
}

// ===========================================================================
// PHPRedis Compatibility (PHP 8.3+)
// ===========================================================================
// Register aliases so PHPRedis class names (Redis, RedisCluster) map to GLIDE.
// ValkeyGlide::registerPHPRedisAliases();
// $client = new Redis();
// $client->connect(addresses: [['host' => 'localhost', 'port' => 6379]]);

// ===========================================================================
// Authentication Patterns
// ===========================================================================

// Password-only authentication
function createClientWithPassword(): ValkeyGlide {
    $client = new ValkeyGlide();
    $client->connect(
        addresses: [['host' => 'localhost', 'port' => 6379]],
        credentials: ['password' => 'mypassword'],
    );
    return $client;
}

// Username + password authentication
function createClientWithUsernamePassword(): ValkeyGlide {
    $client = new ValkeyGlide();
    $client->connect(
        addresses: [['host' => 'localhost', 'port' => 6379]],
        credentials: ['username' => 'myuser', 'password' => 'mypassword'],
    );
    return $client;
}

// ===========================================================================
// TLS / SSL Patterns
// ===========================================================================

// TLS with CA-signed certificate (production)
// NOTE: PHP GLIDE v1.0.0 may not support insecure TLS for self-signed certs.
// Use proper CA-signed certificates in production.
function createClientWithTls(): ValkeyGlide {
    $client = new ValkeyGlide();
    $client->connect(
        addresses: [['host' => 'localhost', 'port' => 6379]],
        use_tls: true,
        credentials: ['password' => 'mypassword'],
    );
    return $client;
}

// ===========================================================================
// AWS ElastiCache / MemoryDB IAM Authentication (GLIDE 2.2+)
// ===========================================================================
// Use ValkeyGlide::IAM_SERVICE_ELASTICACHE or ValkeyGlide::IAM_SERVICE_MEMORYDB
// IAM requires username in credentials and TLS enabled.
// Always call $client->close() when done.
function createClientWithIam(): ValkeyGlide {
    $client = new ValkeyGlide();
    $client->connect(
        addresses: [['host' => 'my-cluster.cache.amazonaws.com', 'port' => 6379]],
        use_tls: true,
        credentials: [
            'username' => 'myUser',
            'iamConfig' => [
                ValkeyGlide::IAM_CONFIG_CLUSTER_NAME => 'my-cluster',
                ValkeyGlide::IAM_CONFIG_REGION => 'us-east-1',
                ValkeyGlide::IAM_CONFIG_SERVICE => ValkeyGlide::IAM_SERVICE_ELASTICACHE,
            ],
        ],
    );
    return $client;
}

// ===========================================================================
// Global client instances (reuse across requests in PHP-FPM)
// ===========================================================================
global $glide_standalone, $glide_cluster;

if (!isset($glide_standalone)) {
    $glide_standalone = createStandaloneClient();
}

if (!isset($glide_cluster)) {
    $glide_cluster = createClusterClient();
}

// Usage in request handlers
function handleRequest() {
    global $glide_standalone;

    try {
        $value = $glide_standalone->get('key');
        return $value;
    } catch (ValkeyGlideException $e) {
        error_log("Valkey error: " . $e->getMessage());
        throw $e;
    }
}
