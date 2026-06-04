from glide_sync import Batch, ClusterBatch, BatchOptions, ClusterBatchOptions
from glide_shared.commands.server_modules.batch_options import BatchRetryStrategy

# Standalone atomic batch (transaction)
batch = Batch(True)
batch.set("key", "value")
batch.get("key")
result = client.exec(batch, raise_on_error=True)

# Standalone pipeline
batch = Batch(False)
batch.set("key1", "value1")
batch.set("key2", "value2")
result = client.exec(batch, raise_on_error=False)

# Cluster pipeline with options
batch = ClusterBatch(False)
batch.set("{user}:1", "data1")
batch.set("{user}:2", "data2")

retry_strategy = BatchRetryStrategy(
    retry_server_error=True,
    retry_connection_error=False
)
options = ClusterBatchOptions(
    timeout=2000,
    retry_strategy=retry_strategy
)
result = client.exec(batch, raise_on_error=False, options=options)
