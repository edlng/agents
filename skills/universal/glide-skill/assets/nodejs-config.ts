// Node.js/TypeScript GLIDE Configuration Template
// Optimized for production web applications
//
// This file contains all client connection patterns:
//   - Standalone and cluster configs
//   - Password and username/password authentication
//   - TLS with CA certificates and self-signed certificates
//   - AWS ElastiCache/MemoryDB IAM authentication (GLIDE 2.2+)
//   - Production tuning (timeouts, retry, throughput, AZ affinity)

import { GlideClient, GlideClusterClient, ReadFrom, ServiceType } from '@valkey/valkey-glide';
import * as fs from 'fs';

// ===========================================================================
// Standalone Client Configuration
// ===========================================================================
export const standaloneConfig = {
  addresses: [{ host: 'localhost', port: 6379 }],

  // Request timeout (500ms recommended for web apps)
  requestTimeout: 500,

  // Connection retry strategy
  connectionBackoff: {
    numberOfRetries: 10,
    factor: 500,        // Base delay in ms
    exponentBase: 2,    // Exponential backoff
  },

  // Client name for debugging
  clientName: 'my-app-client',

  // Lazy connect for serverless/Lambda
  lazyConnect: false,  // Set to true for Lambda

  // High-throughput configuration
  inflightRequestsLimit: 2000,  // Default: 1000
};

// ===========================================================================
// Cluster Client Configuration
// ===========================================================================
export const clusterConfig = {
  addresses: [{ host: 'cluster.endpoint.cache.amazonaws.com', port: 6379 }],

  // Request timeout
  requestTimeout: 500,

  // AZ Affinity for cost optimization (read-heavy workloads)
  readFrom: 'AZAffinity' as ReadFrom,
  clientAz: 'us-east-1a',  // Your application's AZ

  // Connection retry strategy
  connectionBackoff: {
    numberOfRetries: 10,
    factor: 500,
    exponentBase: 2,
  },

  // Client name
  clientName: 'my-app-cluster-client',

  // High-throughput configuration
  inflightRequestsLimit: 2000,
};

// ===========================================================================
// Create / close clients (do this once at application startup)
// ===========================================================================
export async function createClients() {
  const standalone = await GlideClient.createClient(standaloneConfig);
  const cluster = await GlideClusterClient.createClient(clusterConfig);
  return { standalone, cluster };
}

export function closeClients(standalone: GlideClient, cluster: GlideClusterClient) {
  standalone.close();
  cluster.close();
}

// ===========================================================================
// Authentication Patterns
// ===========================================================================

// Password-only authentication
export const passwordConfig = {
  addresses: [{ host: 'localhost', port: 6379 }],
  credentials: { password: 'mypassword' },
  requestTimeout: 5000,
};

// Username + password authentication
export const usernamePasswordConfig = {
  addresses: [{ host: 'localhost', port: 6379 }],
  credentials: { username: 'myuser', password: 'mypassword' },
  requestTimeout: 5000,
};

// ===========================================================================
// TLS / SSL Patterns
// ===========================================================================

// TLS with CA-signed certificate (production)
export const tlsProductionConfig = {
  addresses: [{ host: 'localhost', port: 6379 }],
  useTLS: true,
  credentials: { password: 'mypassword' },
  advancedClientConfiguration: {
    tlsAdvancedConfiguration: {
      rootCertificates: fs.readFileSync('ca.crt'),
    },
  },
  requestTimeout: 5000,
};

// TLS with self-signed certificate (testing only)
// WARNING: insecure mode may not work in all Node.js GLIDE versions.
// If certificate validation is not bypassed, use rootCertificates instead.
export const tlsInsecureConfig = {
  addresses: [{ host: 'localhost', port: 6379 }],
  useTLS: true,
  credentials: { password: 'mypassword' },
  advancedClientConfiguration: {
    tlsAdvancedConfiguration: {
      insecure: true,
    },
  },
  requestTimeout: 5000,
};

// ===========================================================================
// AWS ElastiCache / MemoryDB IAM Authentication (GLIDE 2.2+)
// ===========================================================================
// Use ServiceType.Elasticache or ServiceType.MemoryDB (not strings).
// IAM requires username in credentials and TLS enabled.
// Always call client.close() when done.
export const iamAuthConfig = {
  addresses: [{ host: 'my-cluster.cache.amazonaws.com', port: 6379 }],
  useTLS: true,  // IAM auth requires TLS
  credentials: {
    username: 'myUser',
    iamConfig: {
      cluster_name: 'my-cluster',
      service: ServiceType.Elasticache,  // or ServiceType.MemoryDB
      region: 'us-east-1',
    },
  },
  requestTimeout: 5000,
};
