# Valkey Failover Testing

Test Bridge behavior when Valkey primary/replica fail.

> **Note**: These tests are not running in CI.

## Start

```bash
cd test/failover
docker-compose up -d
docker-compose up benchmark  # runs 5min load test
```

## Test Primary Failure

```bash
# Terminal 1: watch logs
docker logs -f bridge

# Terminal 2: kill primary during load test
docker stop valkey-primary
```

**Expected**: Bridge will have connection errors until you manually reconnect.

## Test Replica Failure

```bash
docker stop valkey-replica
```

**Expected**: Primary keeps working, no issues.

## Check Status

```bash
docker exec valkey-primary valkey-cli INFO replication
docker exec valkey-replica valkey-cli INFO replication
```

## Cleanup

```bash
docker-compose down
```
