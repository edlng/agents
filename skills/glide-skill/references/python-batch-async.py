from glide import Batch, ClusterBatch, BatchOptions, ClusterBatchOptions
from glide_shared.commands.server_modules.batch_options import BatchRetryStrategy

# Standalone atomic batch (transaction)
batch = Batch(True)
batch.set("key", "value")
batch.get("key")
result = await client.exec(batch, raise_on_error=True)

# Cluster pipeline with retry strategy
batch = ClusterBatch(False)
batch.set("{user}:1", "data1")
batch.get("{user}:1")

retry_strategy = BatchRetryStrategy(
    retry_server_error=True,
    retry_connection_error=False
)
options = ClusterBatchOptions(
    timeout=2000,
    retry_strategy=retry_strategy
)
result = await client.exec(batch, raise_on_error=False, options=options)
