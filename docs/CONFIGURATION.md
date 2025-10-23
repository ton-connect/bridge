# Configuration

Complete reference for all environment variables supported by TON Connect Bridge v3.

> **Note:** Looking for Bridge v1 (PostgreSQL) configuration? See [`cmd/bridge/README.md`](../cmd/bridge/README.md) (deprecated).

## Core Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_LEVEL` | string | `info` | **Logging level**<br>`panic` `fatal` `error` `warn` `info` `debug` `trace` |
| `PORT` | int | `8081` | HTTP server port for bridge endpoints |
| `METRICS_PORT` | int | `9103` | Metrics port: `/health` `/ready` `/metrics` `/version` `/debug/pprof/*` |
| `PPROF_ENABLED` | bool | `true` | See [pprof docs](https://pkg.go.dev/net/http/pprof) |

## Storage

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `STORAGE` | string | `memory` | Storage backend<br>`valkey` - **Recommended for production**, pub/sub support, horizontal scaling<br>`memory` - No persistence, development only<br>`postgres` - Limited support, no pub/sub, see [Bridge v1 docs](../cmd/bridge/README.md) |
| `VALKEY_URI` | string | - | **Format:** `valkey://[:pass@]host:port[/db]`<br>**Cluster:** `valkey://node1:6379,node2:6379,node3:6379`<br>**Required for production deployments** |

### Redis/Valkey Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `VALKEY_URI` | string | - | Redis/Valkey connection string |
| `VALKEY_MAX_RETRIES` | int | `3` | Max retry attempts for failed operations |
| `VALKEY_POOL_SIZE` | int | `10` | Connection pool size per instance |
| `VALKEY_MIN_IDLE_CONNS` | int | `5` | Minimum idle connections in pool |
| `VALKEY_MAX_CONN_AGE` | duration | `0` | Max connection lifetime (0 = unlimited) |
| `VALKEY_POOL_TIMEOUT` | duration | `4s` | Timeout for getting connection from pool |
| `VALKEY_IDLE_TIMEOUT` | duration | `5m` | Close idle connections after this time |
| `VALKEY_DIAL_TIMEOUT` | duration | `5s` | Timeout for establishing new connections |
| `VALKEY_READ_TIMEOUT` | duration | `3s` | Timeout for socket reads |
| `VALKEY_WRITE_TIMEOUT` | duration | `3s` | Timeout for socket writes |

## Performance & Limits

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HEARTBEAT_INTERVAL` | int | `10` | SSE heartbeat interval (seconds) |
| `RPS_LIMIT` | int | `1` | Requests/sec per IP for `/bridge/message` |
| `CONNECTIONS_LIMIT` | int | `50` | Max concurrent SSE connections per IP |
| `MAX_BODY_SIZE` | int | `10485760` | Max HTTP request body size (bytes) |
| `RATE_LIMITS_BY_PASS_TOKEN` | string | - | Bypass tokens (comma-separated) |

## Security

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CORS_ENABLE` | bool | `false` | Enable CORS: origins `*`, methods `GET/POST/OPTIONS`, credentials `true` |
| `TRUSTED_PROXY_RANGES` | string | `0.0.0.0/0` | Trusted proxy CIDRs for `X-Forwarded-For` (comma-separated)<br>Example: `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` |
| `SELF_SIGNED_TLS` | bool | `false` | ⚠️ **Dev only**: Self-signed TLS cert. Use nginx/Cloudflare in prod |

## Caching

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONNECT_CACHE_SIZE` | int | `2000000` | Max entries in connect client cache |
| `CONNECT_CACHE_TTL` | int | `300` | Cache TTL (seconds) |
| `ENABLE_TRANSFERED_CACHE` | bool | `true` | Cache transferred messages (prevents duplicates) |
| `ENABLE_EXPIRED_CACHE` | bool | `true` | Cache expired messages (improves performance) |

## Events & Webhooks

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DISCONNECT_EVENTS_TTL` | int | `3600` | Disconnect events retention (seconds) |
| `DISCONNECT_EVENT_MAX_SIZE` | int | `512` | Max disconnect event size (bytes) |
| `WEBHOOK_URL` | string | - | URL for bridge event notifications |
| `COPY_TO_URL` | string | - | Mirror all messages to URL (debugging/analytics) |

## TON Analytics

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TF_ANALYTICS_ENABLED` | bool | `false` | Enable TonConnect analytics |
| `BRIDGE_NAME` | string | `ton-connect-bridge` | Instance name for metrics/logging |
| `BRIDGE_VERSION` | string | `1.0.0` | Version (auto-set during build) |
| `BRIDGE_URL` | string | `localhost` | Public bridge URL |
| `ENVIRONMENT` | string | `production` | Environment name (`dev`, `staging`, `production`) |
| `NETWORK_ID` | string | `-239` | TON network: `-239` (mainnet), `-3` (testnet) |

## Configuration Presets

### 🧪 Development (Memory)

```bash
LOG_LEVEL=debug
STORAGE=memory
CORS_ENABLE=true
HEARTBEAT_INTERVAL=10
RPS_LIMIT=50
CONNECTIONS_LIMIT=50
```

### 🚀 Production: Bridge v3 + Redis/Valkey

```bash
LOG_LEVEL=info
STORAGE=valkey
VALKEY_URI="valkey://valkey.internal:6379/0"
VALKEY_POOL_SIZE=50
VALKEY_MIN_IDLE_CONNS=10
CORS_ENABLE=true
RPS_LIMIT=100000
CONNECTIONS_LIMIT=500000
CONNECT_CACHE_SIZE=500000
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
ENVIRONMENT=production
BRIDGE_URL="https://bridge.myapp.com"
```

### 🚀 Production: Redis Cluster

```bash
LOG_LEVEL=info
STORAGE=valkey
VALKEY_URI="valkey://node1:6379,node2:6379,node3:6379"
VALKEY_POOL_SIZE=100
VALKEY_MIN_IDLE_CONNS=20
CORS_ENABLE=true
RPS_LIMIT=500000
CONNECTIONS_LIMIT=1000000
CONNECT_CACHE_SIZE=1000000
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12"
ENVIRONMENT=production
BRIDGE_URL="https://bridge.myapp.com"
```

> **Note:** For Bridge v1 (PostgreSQL) configuration examples, see [`cmd/bridge/README.md`](../cmd/bridge/README.md)

## Using Environment Files

<details>
<summary><b>💾 .env file</b></summary>

```bash
# .env
LOG_LEVEL=info
PORT=8081
STORAGE=valkey
VALKEY_URI=valkey://localhost:6379
CORS_ENABLE=true
RPS_LIMIT=100
CONNECTIONS_LIMIT=200
```

**Load:**
```bash
export $(cat .env | xargs) && ./bridge3
```

</details>
