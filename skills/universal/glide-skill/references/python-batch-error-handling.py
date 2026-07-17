# raise_on_error=False - errors in result array
batch = ClusterBatch(False)
batch.set("key", "hello")
batch.lpop("key")  # WRONGTYPE error
batch.delete(["key"])

result = client.exec(batch, raise_on_error=False)
# Result: ['OK', RequestError('WRONGTYPE...'), 1]

# raise_on_error=True - raises first error
batch = Batch(True)
batch.set("key", "hello")
batch.lpop("key")  # WRONGTYPE error

try:
    result = client.exec(batch, raise_on_error=True)
except RequestError as e:
    print(f"Batch failed: {e}")