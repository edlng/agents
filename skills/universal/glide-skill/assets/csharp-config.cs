// C# GLIDE Configuration Template
// Optimized for production web applications
// See: https://github.com/valkey-io/valkey-glide-csharp
//
// This file contains all client connection patterns:
//   - Standalone and cluster configs
//   - Password and username/password authentication
//   - TLS configuration
//   - AWS ElastiCache/MemoryDB IAM authentication (v2.0+)
//   - Production tuning (timeouts, retry, throughput, AZ affinity)
//   - Database selection, protocol version, client name
//
// Note: Current NuGet version is 0.9.0. IAM authentication and insecure
// TLS mode require Valkey.Glide 2.0+ (not yet released).

using Valkey.Glide;
using static Valkey.Glide.ConnectionConfiguration;

public static class GlideConfig
{
    // =====================================================================
    // Standalone Client Configuration
    // =====================================================================
    public static StandaloneClientConfiguration StandaloneConfig()
    {
        return new StandaloneClientConfigurationBuilder()
            .WithAddress("localhost", 6379)
            // Request timeout (500ms recommended for web apps)
            .WithRequestTimeout(TimeSpan.FromMilliseconds(500))
            // Connection retry strategy
            .WithConnectionRetryStrategy(
                numberOfRetries: 10,
                factor: 500,        // Base delay in ms
                exponentBase: 2     // Exponential backoff
            )
            // Client name for debugging
            .WithClientName("my-app-client")
            // Lazy connect for serverless/Lambda
            .WithLazyConnect(false)  // Set to true for Lambda
            .Build();
    }

    // =====================================================================
    // Cluster Client Configuration
    // =====================================================================
    public static ClusterClientConfiguration ClusterConfig()
    {
        return new ClusterClientConfigurationBuilder()
            .WithAddress("cluster.endpoint.cache.amazonaws.com", 6379)
            .WithRequestTimeout(TimeSpan.FromMilliseconds(500))
            // AZ Affinity for cost optimization (read-heavy workloads)
            .WithReadFrom(new ReadFrom(ReadFromStrategy.AzAffinity, "us-east-1a"))
            .WithConnectionRetryStrategy(
                numberOfRetries: 10,
                factor: 500,
                exponentBase: 2
            )
            .WithClientName("my-app-cluster-client")
            .Build();
    }

    // =====================================================================
    // Create / close clients (do this once at application startup)
    // =====================================================================
    public static async Task<(GlideClient Standalone, GlideClusterClient Cluster)> CreateClientsAsync()
    {
        var standalone = await GlideClient.CreateClient(StandaloneConfig());
        var cluster = await GlideClusterClient.CreateClient(ClusterConfig());
        return (standalone, cluster);
    }

    public static async ValueTask CloseClientsAsync(
        GlideClient standalone, GlideClusterClient cluster)
    {
        await standalone.DisposeAsync();
        await cluster.DisposeAsync();
    }

    // =====================================================================
    // Password authentication
    // =====================================================================
    public static StandaloneClientConfiguration PasswordConfig()
    {
        return new StandaloneClientConfigurationBuilder()
            .WithAddress("localhost", 6379)
            .WithAuthentication("password")
            .WithRequestTimeout(TimeSpan.FromSeconds(5))
            .Build();
    }

    // =====================================================================
    // Username + password authentication
    // =====================================================================
    public static StandaloneClientConfiguration UsernamePasswordConfig()
    {
        return new StandaloneClientConfigurationBuilder()
            .WithAddress("localhost", 6379)
            .WithAuthentication("username", "password")
            .WithRequestTimeout(TimeSpan.FromSeconds(5))
            .Build();
    }

    // =====================================================================
    // TLS (production, CA-signed certificates)
    // =====================================================================
    public static StandaloneClientConfiguration TlsConfig()
    {
        return new StandaloneClientConfigurationBuilder()
            .WithAddress("localhost", 6379)
            .WithAuthentication("username", "password")
            .WithTls()
            .WithRequestTimeout(TimeSpan.FromSeconds(5))
            .Build();
    }

    // =====================================================================
    // AWS ElastiCache / MemoryDB IAM Authentication (v2.0+)
    // Not yet available in NuGet 0.9.0.
    // =====================================================================
    public static ClusterClientConfiguration IamAuthConfig()
    {
        var iamAuthConfig = new IamAuthConfig(
            "cluster-name", ServiceType.ElastiCache, "us-east-1");

        return new ClusterClientConfigurationBuilder()
            .WithAddress("host", 6379)
            .WithAuthentication("username", iamAuthConfig)
            .WithTls(true)
            .WithRequestTimeout(TimeSpan.FromSeconds(5))
            .Build();
    }

    // =====================================================================
    // Database selection (standalone only)
    // =====================================================================
    public static StandaloneClientConfiguration WithDatabaseConfig()
    {
        return new StandaloneClientConfigurationBuilder()
            .WithAddress("localhost", 6379)
            .WithDataBaseId(1)
            .Build();
    }

    // =====================================================================
    // Protocol version
    // =====================================================================
    public static StandaloneClientConfiguration Resp2Config()
    {
        return new StandaloneClientConfigurationBuilder()
            .WithAddress("localhost", 6379)
            .WithProtocolVersion(ConnectionConfiguration.Protocol.RESP2)
            .Build();
    }
}
