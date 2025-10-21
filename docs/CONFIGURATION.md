# Configuration

Complete reference for all environment variables supported by TON Connect Bridge.

## Quick Reference

| Variable | v1 | v3 | Default | Description |
|----------|:--:|:--:|---------|-------------|
| **Core** |
| `LOG_LEVEL` | ✅ | ✅ | `info` | Logging level: `trace`, `debug`, `info`, `warn`, `error` |
| `PORT` | ✅ | ✅ | `8081` | HTTP server port |
| `METRICS_PORT` | ✅ | ✅ | `9103` | Metrics/health port (`/health`, `/metrics`, `/ready`) |
| **Storage** |
| `STORAGE` | - | ✅ | `memory` | Storage backend: `memory`, `valkey`, `postgres` |
| `POSTGRES_URI` | ✅ | ✅ | - | `postgres://user:pass@host:port/db?options` |
| `VALKEY_URI` | - | ✅ | - | `valkey://[:pass@]host:port[/db]` |
| **Performance** |
| `RPS_LIMIT` | ✅ | ✅ | `1` | Requests/sec per IP |
| `CONNECTIONS_LIMIT` | ✅ | ✅ | `50` | Max concurrent connections per IP |
| `HEARTBEAT_INTERVAL` | ✅ | ✅ | `10` | SSE heartbeat interval (seconds) |
| **Security** |
| `CORS_ENABLE` | ✅ | ✅ | `false` | Enable CORS headers |
| `TRUSTED_PROXY_RANGES` | ✅ | ✅ | `0.0.0.0/0` | Trusted proxy CIDRs (comma-separated) |
| `RATE_LIMITS_BY_PASS_TOKEN` | ✅ | ✅ | - | Bypass tokens (comma-separated) |

💡 **Tip:** Bridge v1 auto-selects storage: PostgreSQL if `POSTGRES_URI` is set, Memory otherwise.

---

## Core Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_LEVEL` | string | `info` | **Logging level**<br>`panic` `fatal` `error` `warn` `info` `debug` `trace` |
| `PORT` | int | `8081` | HTTP server port for bridge endpoints |
| `METRICS_PORT` | int | `9103` | Metrics port: `/health` `/ready` `/metrics` `/version` `/debug/pprof/*` |

```bash
# Example
LOG_LEVEL=debug PORT=8080 METRICS_PORT=9090 ./bridge3
```

---

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

<details>
<summary><b>📖 Tuning Guidelines</b></summary>

| Workload | Max Conns | Min Conns | Lifetime |
|----------|-----------|-----------|----------|
| Light (<100 msg/s) | 25 | 5 | 1h |
| Medium (100-500 msg/s) | 50 | 10 | 2h |
| Heavy (500+ msg/s) | 100-200 | 20 | 4h |

</details>

```bash
# PostgreSQL example
POSTGRES_URI="postgres://bridge:pass@db:5432/bridge?sslmode=require" \
POSTGRES_MAX_CONNS=50 \
POSTGRES_MIN_CONNS=10 \
./bridge

# Valkey example
STORAGE=valkey \
VALKEY_URI="valkey://localhost:6379/0" \
./bridge3

# Valkey cluster
STORAGE=valkey \
VALKEY_URI="valkey://node1:6379,node2:6379,node3:6379" \
./bridge3
```

---

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

---

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

---

## Caching

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONNECT_CACHE_SIZE` | int | `2000000` | Max entries in connect client cache |
| `CONNECT_CACHE_TTL` | int | `300` | Cache TTL (seconds) |
| `ENABLE_TRANSFERED_CACHE` | bool | `true` | Cache transferred messages (prevents duplicates) |
| `ENABLE_EXPIRED_CACHE` | bool | `true` | Cache expired messages (improves performance) |

---

## Events & Webhooks

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DISCONNECT_EVENTS_TTL` | int | `3600` | Disconnect events retention (seconds) |
| `DISCONNECT_EVENT_MAX_SIZE` | int | `512` | Max disconnect event size (bytes) |
| `WEBHOOK_URL` | string | - | URL for bridge event notifications |
| `COPY_TO_URL` | string | - | Mirror all messages to URL (debugging/analytics) |

---

## Monitoring

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PPROF_ENABLED` | bool | `true` | Enable `/debug/pprof/*` endpoints<br>⚠️ Disable or restrict in production |
| `TF_ANALYTICS_ENABLED` | bool | `false` | Enable TonConnect analytics |

<details>
<summary><b>📊 pprof Endpoints</b></summary>

- `/debug/pprof/` - Index
- `/debug/pprof/heap` - Memory allocation
- `/debug/pprof/goroutine` - Goroutine traces
- `/debug/pprof/profile` - CPU profile (30s)

See [pprof docs](https://pkg.go.dev/net/http/pprof)

</details>

---

## Metadata

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
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
HEARTBEAT_INTERVAL=5
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
