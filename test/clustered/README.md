# Valkey Cluster Scalability Tests

Find the breaking point where the 3-node cluster experiences performance degradation (high latency, errors, or dropped messages) while the 6-node cluster continues to handle the same workload successfully. This demonstrates the practical value of horizontal scaling.

> **Note**: These tests are not running in CI.

## Architecture

Both clusters use Valkey 7.2 in cluster mode:

### 3-Node Cluster
- **3 shards** × 0.05 CPU = **0.45 CPU total**
- No replicas (master-only configuration)
- 256MB memory per shard
- Network: `172.21.0.0/16`

### 6-Node Cluster  
- **6 shards** × 0.05 CPU = **0.90 CPU total**
- No replicas (master-only configuration)
- 256MB memory per shard
- Network: `172.22.0.0/16` with automatic slot distribution.


## Start

```bash
# Run a specific scenario on 3-node cluster
make test-3node-write-heavy
make test-3node-read-heavy
make test-3node-mixed

# Run a specific scenario on 6-node cluster
make test-6node-write-heavy
make test-6node-read-heavy
make test-6node-mixed
```

Or use the shell scripts directly:

```bash
./run-cluster-3-nodes.sh mixed
./run-cluster-6-nodes.sh write-heavy
```
