// Java GLIDE Configuration Template
// Optimized for production web applications
//
// This file contains all client connection patterns:
//   - Standalone and cluster configs
//   - Password and username/password authentication
//   - TLS with CA certificates and self-signed certificates
//   - AWS ElastiCache/MemoryDB IAM authentication (GLIDE 2.2+)
//   - Production tuning (timeouts, retry, throughput, AZ affinity)
//
// Key method names (case-sensitive):
//   - useTLS(true)                  — capital TLS, not useTls()
//   - .tlsAdvancedConfiguration()   — not .tlsAdvancedConfig()
//   - .advancedConfiguration()      — not .advancedConfig()

import glide.api.GlideClient;
import glide.api.GlideClusterClient;
import glide.api.models.configuration.AdvancedGlideClientConfiguration;
import glide.api.models.configuration.BackoffStrategy;
import glide.api.models.configuration.GlideClientConfiguration;
import glide.api.models.configuration.GlideClusterClientConfiguration;
import glide.api.models.configuration.IamAuthConfig;
import glide.api.models.configuration.NodeAddress;
import glide.api.models.configuration.ReadFrom;
import glide.api.models.configuration.ServerCredentials;
import glide.api.models.configuration.ServiceType;
import glide.api.models.configuration.TlsAdvancedConfiguration;

import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.concurrent.ExecutionException;

public class GlideConfig {

    // ======================================================================
    // Standalone Client Configuration
    // ======================================================================
    public static GlideClientConfiguration standaloneConfig() {
        return GlideClientConfiguration.builder()
            .address(NodeAddress.builder()
                .host("localhost")
                .port(6379)
                .build())
            // Request timeout (500ms recommended for web apps)
            .requestTimeout(500)
            // Connection retry strategy
            .reconnectStrategy(BackoffStrategy.builder()
                .numberOfRetries(10)
                .factor(500)        // Base delay in ms
                .exponentBase(2)    // Exponential backoff
                .build())
            // Client name for debugging
            .clientName("my-app-client")
            // Lazy connect for serverless/Lambda
            .lazyConnect(false)  // Set to true for Lambda
            // High-throughput configuration
            .inflightRequestsLimit(2000)  // Default: 1000
            .build();
    }

    // ======================================================================
    // Cluster Client Configuration
    // ======================================================================
    public static GlideClusterClientConfiguration clusterConfig() {
        return GlideClusterClientConfiguration.builder()
            .address(NodeAddress.builder()
                .host("cluster.endpoint.cache.amazonaws.com")
                .port(6379)
                .build())
            .requestTimeout(500)
            // AZ Affinity for cost optimization (read-heavy workloads)
            .readFrom(ReadFrom.AZ_AFFINITY)
            .clientAZ("us-east-1a")  // Your application's AZ
            .reconnectStrategy(BackoffStrategy.builder()
                .numberOfRetries(10)
                .factor(500)
                .exponentBase(2)
                .build())
            .clientName("my-app-cluster-client")
            .inflightRequestsLimit(2000)
            .build();
    }

    // ======================================================================
    // Password-only authentication
    // ======================================================================
    public static GlideClientConfiguration passwordConfig() {
        return GlideClientConfiguration.builder()
            .address(NodeAddress.builder().host("localhost").port(6379).build())
            .credentials(ServerCredentials.builder()
                .password("mypassword")
                .build())
            .requestTimeout(5000)
            .build();
    }

    // ======================================================================
    // Username + password authentication
    // ======================================================================
    public static GlideClientConfiguration usernamePasswordConfig() {
        return GlideClientConfiguration.builder()
            .address(NodeAddress.builder().host("localhost").port(6379).build())
            .credentials(ServerCredentials.builder()
                .username("user")
                .password("mypassword")
                .build())
            .requestTimeout(5000)
            .build();
    }

    // ======================================================================
    // TLS with CA-signed certificate (production)
    // ======================================================================
    public static GlideClientConfiguration tlsProductionConfig() throws Exception {
        byte[] caCert = Files.readAllBytes(Paths.get("ca.crt"));

        return GlideClientConfiguration.builder()
            .address(NodeAddress.builder().host("localhost").port(6379).build())
            .useTLS(true)
            .credentials(ServerCredentials.builder().password("mypassword").build())
            .advancedConfiguration(AdvancedGlideClientConfiguration.builder()
                .tlsAdvancedConfiguration(TlsAdvancedConfiguration.builder()
                    .rootCertificates(caCert)
                    .build())
                .build())
            .requestTimeout(5000)
            .build();
    }

    // ======================================================================
    // TLS with self-signed certificate (testing only)
    // WARNING: useInsecureTLS disables certificate verification — NOT for production
    // ======================================================================
    public static GlideClientConfiguration tlsInsecureConfig() {
        return GlideClientConfiguration.builder()
            .address(NodeAddress.builder().host("localhost").port(6379).build())
            .useTLS(true)
            .credentials(ServerCredentials.builder().password("mypassword").build())
            .advancedConfiguration(AdvancedGlideClientConfiguration.builder()
                .tlsAdvancedConfiguration(TlsAdvancedConfiguration.builder()
                    .useInsecureTLS(true)
                    .build())
                .build())
            .requestTimeout(5000)
            .build();
    }

    // ======================================================================
    // AWS ElastiCache / MemoryDB IAM Authentication (GLIDE 2.2+)
    // IAM requires username in credentials and TLS enabled.
    // Always use try-with-resources for automatic client cleanup.
    // ======================================================================
    public static GlideClientConfiguration iamAuthConfig() {
        IamAuthConfig iamConfig = IamAuthConfig.builder()
            .clusterName("my-cluster")
            .service(ServiceType.ELASTICACHE)  // or ServiceType.MEMORYDB
            .region("us-east-1")
            .build();

        return GlideClientConfiguration.builder()
            .address(NodeAddress.builder()
                .host("my-cluster.cache.amazonaws.com")
                .port(6379)
                .build())
            .useTLS(true)  // IAM auth requires TLS
            .credentials(ServerCredentials.builder()
                .username("myUser")
                .iamConfig(iamConfig)
                .build())
            .requestTimeout(5000)
            .build();
    }

    // ======================================================================
    // Create / close clients (do this once at application startup)
    // ======================================================================
    public static class Clients implements AutoCloseable {
        private final GlideClient standalone;
        private final GlideClusterClient cluster;

        public Clients() throws ExecutionException, InterruptedException {
            this.standalone = GlideClient.createClient(standaloneConfig()).get();
            this.cluster = GlideClusterClient.createClient(clusterConfig()).get();
        }

        public GlideClient getStandalone() { return standalone; }
        public GlideClusterClient getCluster() { return cluster; }

        @Override
        public void close() {
            try { standalone.close(); } catch (Exception e) { /* log */ }
            try { cluster.close(); } catch (Exception e) { /* log */ }
        }
    }
}
