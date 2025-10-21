# Configuration

Complete reference for all environment variables supported by TON Connect Bridge.

## Core Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_LEVEL` | string | `info` | **Logging level**<br>`panic` `fatal` `error` `warn` `info` `debug` `trace` |
| `PORT` | int | `8081` | HTTP server port for bridge endpoints |
| `METRICS_PORT` | int | `9103` | Metrics port: `/health` `/ready` `/metrics` `/version` `/debug/pprof/*` |
| `PPROF_ENABLED` | bool | `true` | See [pprof docs](https://pkg.go.dev/net/http/pprof) |

```bash
# Example
LOG_LEVEL=debug PORT=8080 METRICS_PORT=9090 ./bridge3
```

## Storage

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `STORAGE` | string | `memory` | **Bridge v3 only**<br>`memory` - No persistence, fastest<br>`valkey` - Pub/sub, production recommended<br>`postgres` - Limited support, no pub/sub |
| `POSTGRES_URI` | string | - | **Format:** `postgres://user:pass@host:port/db?sslmode=require`<br>Bridge v1: Required for production<br>Bridge v3: Limited support |
| `VALKEY_URI` | string | - | **Format:** `valkey://[:pass@]host:port[/db]`<br>**Cluster:** `valkey://node1:6379,node2:6379,node3:6379` |

### PostgreSQL Pool Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `POSTGRES_MAX_CONNS` | int | `25` | Max connections in pool |
| `POSTGRES_MIN_CONNS` | int | `0` | Min idle connections |
| `POSTGRES_MAX_CONN_LIFETIME` | duration | `1h` | Connection lifetime (`1h`, `30m`, `90s`) |
| `POSTGRES_MAX_CONN_LIFETIME_JITTER` | duration | `10m` | Random jitter to prevent thundering herd |
| `POSTGRES_MAX_CONN_IDLE_TIME` | duration | `30m` | Max idle time before closing |
| `POSTGRES_HEALTH_CHECK_PERIOD` | duration | `1m` | Health check interval |
| `POSTGRES_LAZY_CONNECT` | bool | `false` | Create connections on-demand |

### Redis settings

TODO

## Performance & Limits

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HEARTBEAT_INTERVAL` | int | `10` | SSE heartbeat interval (seconds)<br>Dev: `5-10s` · Prod: `10-15s` |
| `RPS_LIMIT` | int | `1` | Requests/sec per IP for `/bridge/message`<br>`0` = disabled (not recommended) |
| `CONNECTIONS_LIMIT` | int | `50` | Max concurrent SSE connections per IP<br>Small: `50-100` · Medium: `200-500` · Large: `1000+` |
| `MAX_BODY_SIZE` | int | `10485760` | Max HTTP request body size (bytes)<br>Default: 10 MB |
| `RATE_LIMITS_BY_PASS_TOKEN` | string | - | Bypass tokens (comma-separated)<br>Use with `Authorization: Bearer <token>` |

```bash
# Example
HEARTBEAT_INTERVAL=10 \
RPS_LIMIT=100 \
CONNECTIONS_LIMIT=500 \
RATE_LIMITS_BY_PASS_TOKEN="secret-token-1,secret-token-2" \
./bridge3
```

## Security

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CORS_ENABLE` | bool | `false` | Enable CORS: origins `*`, methods `GET/POST/OPTIONS`, credentials `true` |
| `TRUSTED_PROXY_RANGES` | string | `0.0.0.0/0` | Trusted proxy CIDRs for `X-Forwarded-For` (comma-separated)<br>Example: `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` |
| `SELF_SIGNED_TLS` | bool | `false` | ⚠️ **Dev only**: Self-signed TLS cert. Use nginx/Cloudflare in prod |

```bash
# Example
CORS_ENABLE=true \
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12" \
./bridge
```

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

---

## Configuration Presets

### 🧪 Development (Memory)

```bash
LOG_LEVEL=debug
STORAGE=memory
CORS_ENABLE=true
HEARTBEAT_INTERVAL=10
CONNECTIONS_LIMIT=50
```

### 🚀 Production: Bridge v1 + PostgreSQL

```bash
LOG_LEVEL=info
POSTGRES_URI="postgres://bridge:${PASSWORD}@db.internal:5432/bridge?sslmode=require"
POSTGRES_MAX_CONNS=100
POSTGRES_MIN_CONNS=10
CORS_ENABLE=true
RPS_LIMIT=100000
CONNECTIONS_LIMIT=50000
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12"
PPROF_ENABLED=false
ENVIRONMENT=production
BRIDGE_URL="https://bridge.myapp.com"
```

### 🚀 Production: Bridge v3 + Valkey

```bash
LOG_LEVEL=info
STORAGE=valkey
VALKEY_URI="valkey://valkey.internal:6379/0"
CORS_ENABLE=true
RPS_LIMIT=100000
CONNECTIONS_LIMIT=50000
CONNECT_CACHE_SIZE=5000000
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12"
PPROF_ENABLED=false
ENVIRONMENT=production
BRIDGE_URL="https://bridge-v3.myapp.com"
```

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

**Docker Compose:**
```yaml
services:
  bridge:
    image: bridge3
    env_file: .env
```

</details>
