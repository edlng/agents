// Go GLIDE Configuration Template
// Optimized for production web applications
//
// This file contains all client connection patterns:
//   - Standalone and cluster configs
//   - Password and username/password authentication
//   - TLS with CA certificates and self-signed certificates
//   - AWS ElastiCache/MemoryDB IAM authentication (GLIDE 2.2+)
//   - Production tuning (timeouts, retry, throughput, AZ affinity)
//
// Key notes:
//   - WithAdvancedConfiguration() must be called last in the builder chain
//   - Always check err != nil and use defer client.Close()

package config

import (
	"os"
	"time"

	glide "github.com/valkey-io/valkey-glide/go/v2"
	"github.com/valkey-io/valkey-glide/go/v2/config"
)

// =========================================================================
// Standalone Client Configuration
// =========================================================================
func StandaloneConfig() *config.ClientConfiguration {
	return config.NewClientConfiguration().
		WithAddress(&config.NodeAddress{Host: "localhost", Port: 6379}).
		// Request timeout (500ms recommended for web apps)
		WithRequestTimeout(500 * time.Millisecond).
		// Connection retry strategy
		WithReconnectStrategy(config.NewBackoffStrategy(
			10,  // numberOfRetries
			500, // factor (base delay in ms)
			2,   // exponentBase
		)).
		// Client name for debugging
		WithClientName("my-app-client")
}

// Note: inflightRequestsLimit is currently not exposed in the Go config builder.
// It is managed at the Rust core level (default: 1000).

// =========================================================================
// Cluster Client Configuration
// =========================================================================
func ClusterConfig() *config.ClusterClientConfiguration {
	return config.NewClusterClientConfiguration().
		WithAddress(&config.NodeAddress{Host: "cluster.endpoint.cache.amazonaws.com", Port: 6379}).
		WithRequestTimeout(500 * time.Millisecond).
		// AZ Affinity for cost optimization (read-heavy workloads)
		WithReadFrom(config.AzAffinity).
		WithClientAZ("us-east-1a"). // Your application's AZ
		WithReconnectStrategy(config.NewBackoffStrategy(
			10, 500, 2,
		)).
		WithClientName("my-app-cluster-client")
}

// =========================================================================
// Create / close clients (do this once at application startup)
// =========================================================================
type Clients struct {
	Standalone *glide.Client
	Cluster    *glide.ClusterClient
}

func CreateClients() (*Clients, error) {
	standalone, err := glide.NewClient(StandaloneConfig())
	if err != nil {
		return nil, err
	}
	cluster, err := glide.NewClusterClient(ClusterConfig())
	if err != nil {
		standalone.Close()
		return nil, err
	}
	return &Clients{Standalone: standalone, Cluster: cluster}, nil
}

func (c *Clients) Close() {
	if c.Standalone != nil {
		c.Standalone.Close()
	}
	if c.Cluster != nil {
		c.Cluster.Close()
	}
}

// =========================================================================
// Password-only authentication
// =========================================================================
func PasswordConfig() *config.ClientConfiguration {
	return config.NewClientConfiguration().
		WithAddress(&config.NodeAddress{Host: "localhost", Port: 6379}).
		WithCredentials(config.NewServerCredentials("", "mypassword")).
		WithRequestTimeout(5000 * time.Millisecond)
}

// =========================================================================
// Username + password authentication
// =========================================================================
func UsernamePasswordConfig() *config.ClientConfiguration {
	return config.NewClientConfiguration().
		WithAddress(&config.NodeAddress{Host: "localhost", Port: 6379}).
		WithCredentials(config.NewServerCredentials("myuser", "mypassword")).
		WithRequestTimeout(5000 * time.Millisecond)
}

// =========================================================================
// TLS with CA-signed certificate (production)
// =========================================================================
func TlsProductionConfig() (*config.ClientConfiguration, error) {
	caCert, err := os.ReadFile("ca.crt")
	if err != nil {
		return nil, err
	}
	tlsConfig := config.NewTlsConfiguration().WithRootCertificates(caCert)
	advancedConfig := config.NewAdvancedClientConfiguration().WithTlsConfiguration(tlsConfig)

	return config.NewClientConfiguration().
		WithAddress(&config.NodeAddress{Host: "localhost", Port: 6379}).
		WithUseTLS(true).
		WithCredentials(config.NewServerCredentials("", "mypassword")).
		WithRequestTimeout(5000 * time.Millisecond).
		// WithAdvancedConfiguration must be called last in the chain
		WithAdvancedConfiguration(advancedConfig), nil
}

// =========================================================================
// TLS with self-signed certificate (testing only)
// WARNING: WithInsecureTLS disables certificate verification — NOT for production
// =========================================================================
func TlsInsecureConfig() *config.ClientConfiguration {
	tlsConfig := config.NewTlsConfiguration().WithInsecureTLS(true)
	advancedConfig := config.NewAdvancedClientConfiguration().WithTlsConfiguration(tlsConfig)

	return config.NewClientConfiguration().
		WithAddress(&config.NodeAddress{Host: "localhost", Port: 6379}).
		WithUseTLS(true).
		WithCredentials(config.NewServerCredentials("", "mypassword")).
		WithRequestTimeout(5000 * time.Millisecond).
		WithAdvancedConfiguration(advancedConfig)
}

// =========================================================================
// AWS ElastiCache / MemoryDB IAM Authentication (GLIDE 2.2+)
// Use config.ElastiCache or config.MemoryDB for service type.
// IAM requires username in credentials and TLS enabled.
// Always use defer client.Close() for cleanup.
// =========================================================================
func IamAuthConfig() (*config.ClientConfiguration, error) {
	iamConfig := config.NewIamAuthConfig("my-cluster", config.ElastiCache, "us-east-1")
	credentials, err := config.NewServerCredentialsWithIam("myUser", iamConfig)
	if err != nil {
		return nil, err
	}

	return config.NewClientConfiguration().
		WithAddress(&config.NodeAddress{Host: "my-cluster.cache.amazonaws.com", Port: 6379}).
		WithUseTLS(true). // IAM auth requires TLS
		WithCredentials(credentials).
		WithRequestTimeout(5000 * time.Millisecond), nil
}
