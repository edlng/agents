# Python GLIDE Configuration Template
# Optimized for production web applications
#
# This file contains all client connection patterns:
#   - Standalone and cluster configs (async + sync)
#   - Password and username/password authentication
#   - TLS with CA certificates and self-signed certificates
#   - AWS ElastiCache/MemoryDB IAM authentication (GLIDE 2.2+)
#   - Production tuning (timeouts, retry, throughput, AZ affinity)

from glide import (
    AdvancedGlideClientConfiguration,
    GlideClient,
    GlideClusterClient,
    GlideClientConfiguration,
    GlideClusterClientConfiguration,
    NodeAddress,
    BackoffStrategy,
    ReadFrom,
    ServerCredentials,
)

# ---------------------------------------------------------------------------
# Standalone Client Configuration (Async)
# ---------------------------------------------------------------------------
async_standalone_config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],

    # Request timeout (500ms recommended for web apps)
    request_timeout=500,

    # Connection retry strategy
    reconnect_strategy=BackoffStrategy(
        num_of_retries=10,
        factor=500,        # Base delay in ms
        exponent_base=2,   # Exponential backoff
    ),

    # Client name for debugging
    client_name="my-app-client",

    # Lazy connect for serverless/Lambda
    lazy_connect=False,  # Set to True for Lambda

    # High-throughput configuration
    inflight_requests_limit=2000,  # Default: 1000
)

# ---------------------------------------------------------------------------
# Cluster Client Configuration (Async)
# ---------------------------------------------------------------------------
async_cluster_config = GlideClusterClientConfiguration(
    addresses=[NodeAddress("cluster.endpoint.cache.amazonaws.com", 6379)],

    # Request timeout
    request_timeout=500,

    # AZ Affinity for cost optimization (read-heavy workloads)
    read_from=ReadFrom.AZ_AFFINITY,
    client_az="us-east-1a",  # Your application's AZ

    # Connection retry strategy
    reconnect_strategy=BackoffStrategy(
        num_of_retries=10,
        factor=500,
        exponent_base=2,
    ),

    # Client name
    client_name="my-app-cluster-client",

    # High-throughput configuration
    inflight_requests_limit=2000,
)


# ---------------------------------------------------------------------------
# Create / close clients (do this once at application startup)
# ---------------------------------------------------------------------------
async def create_async_clients():
    standalone = await GlideClient.create(async_standalone_config)
    cluster = await GlideClusterClient.create(async_cluster_config)
    return standalone, cluster


async def close_async_clients(standalone, cluster):
    await standalone.close()
    await cluster.close()


# ---------------------------------------------------------------------------
# Sync Client Configuration
# The sync client is a separate package: pip install valkey-glide-sync
# Note: Use either async (glide) or sync (glide_sync), not both.
# Both are shown here for reference. The glide_sync module exports the same
# class names, aliased here to avoid collision.
# ---------------------------------------------------------------------------
from glide_sync import GlideClient as GlideClientSync

sync_standalone_config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    request_timeout=500,
    reconnect_strategy=BackoffStrategy(
        num_of_retries=10,
        factor=500,
        exponent_base=2,
    ),
    client_name="my-app-sync-client",
)


def create_sync_client():
    return GlideClientSync.create(sync_standalone_config)


def close_sync_client(standalone):
    standalone.close()


# ===========================================================================
# Authentication Patterns
# ===========================================================================

# ---------------------------------------------------------------------------
# Password-only authentication
# ---------------------------------------------------------------------------
password_config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    credentials=ServerCredentials("password"),
    request_timeout=5000,
)

# ---------------------------------------------------------------------------
# Username + password authentication
# ---------------------------------------------------------------------------
username_password_config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    credentials=ServerCredentials("password", "username"),
    request_timeout=5000,
)


# ===========================================================================
# TLS / SSL Patterns
# ===========================================================================

# ---------------------------------------------------------------------------
# TLS with CA-signed certificate (production)
# ---------------------------------------------------------------------------
from glide_shared.config import TlsAdvancedConfiguration

with open("ca.crt", "rb") as f:
    ca_cert = f.read()

tls_production_config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    use_tls=True,
    credentials=ServerCredentials("password"),
    advanced_config=AdvancedGlideClientConfiguration(
        tls_config=TlsAdvancedConfiguration(root_pem_cacerts=ca_cert),
    ),
    request_timeout=5000,
)

# ---------------------------------------------------------------------------
# TLS with self-signed certificate (testing only)
# WARNING: Disables certificate verification — do NOT use in production
# ---------------------------------------------------------------------------
tls_insecure_config = GlideClientConfiguration(
    addresses=[NodeAddress("localhost", 6379)],
    use_tls=True,
    credentials=ServerCredentials("password"),
    advanced_config=AdvancedGlideClientConfiguration(
        tls_config=TlsAdvancedConfiguration(use_insecure_tls=True),
    ),
    request_timeout=5000,
)


# ===========================================================================
# AWS ElastiCache / MemoryDB IAM Authentication (GLIDE 2.2+)
# Requires: pip install --upgrade valkey-glide
# ===========================================================================
from glide import IamAuthConfig, ServiceType

iam_config = IamAuthConfig(
    cluster_name="my-cluster",
    service=ServiceType.ELASTICACHE,  # or ServiceType.MEMORYDB
    region="us-east-1",
)

iam_auth_config = GlideClientConfiguration(
    addresses=[NodeAddress("my-cluster.cache.amazonaws.com", 6379)],
    use_tls=True,  # IAM auth requires TLS
    credentials=ServerCredentials(username="myUser", iam_config=iam_config),
    request_timeout=5000,
)
