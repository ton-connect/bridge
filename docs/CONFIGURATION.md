# Configuration

Complete reference for all environment variables supported by TON Connect Bridge.

## Quick Reference

| Variable | Bridge v1 | Bridge v3 | Default | Description |
|----------|:---------:|:---------:|---------|-------------|
| `STORAGE` | - | ✅ | `memory` | Storage backend: `memory`, `valkey`, `postgres` |
| `PORT` | ✅ | ✅ | `8081` | HTTP server port |
| `METRICS_PORT` | ✅ | ✅ | `9103` | Metrics and health endpoint port |
| `LOG_LEVEL` | ✅ | ✅ | `info` | Logging level |
| `POSTGRES_URI` | ✅ | ✅ | - | PostgreSQL connection string |
| `VALKEY_URI` | - | ✅ | - | Valkey/Redis connection string |

See sections below for complete details.

## Core Settings

### `LOG_LEVEL`
**Type:** `string`  
**Default:** `info`  
**Applies to:** Bridge v1, v3

Logging verbosity level.

**Valid values:**
- `panic` - System is unusable
- `fatal` - Critical errors
- `error` - Error conditions
- `warn` - Warning conditions
- `info` - Informational messages (recommended)
- `debug` - Debug-level messages
- `trace` - Very verbose, trace-level messages

**Example:**
```bash
LOG_LEVEL=debug ./bridge3
```

### `PORT`
**Type:** `integer`  
**Default:** `8081`  
**Applies to:** Bridge v1, v3

HTTP server port for bridge endpoints.

**Example:**
```bash
PORT=8080 ./bridge
```

### `METRICS_PORT`
**Type:** `integer`  
**Default:** `9103`  
**Applies to:** Bridge v1, v3

Port for metrics, health checks, and profiling endpoints.

**Endpoints available:**
- `/health` - Health check
- `/ready` - Readiness check
- `/version` - Version information
- `/metrics` - Prometheus metrics
- `/debug/pprof/*` - Go profiling (if `PPROF_ENABLED=true`)

**Example:**
```bash
METRICS_PORT=9090 ./bridge3
```

## Storage Configuration

### `STORAGE`
**Type:** `string`  
**Default:** `memory`  
**Applies to:** Bridge v3 only

Storage backend selection.

**Valid values:**
- `memory` - In-memory storage (no persistence, fastest)
- `valkey` - Valkey/Redis pub/sub storage (recommended for production)
- `postgres` - PostgreSQL storage (limited support, no pub/sub yet)

**Example:**
```bash
STORAGE=valkey VALKEY_URI="valkey://localhost:6379" ./bridge3
```

**Note:** Bridge v1 automatically selects storage based on `POSTGRES_URI` presence:
- If `POSTGRES_URI` is set → PostgreSQL storage
- If not set → Memory storage

### `POSTGRES_URI`
**Type:** `string`  
**Default:** _(empty)_  
**Applies to:** Bridge v1 (required for production), Bridge v3 (limited)

PostgreSQL connection string.

**Format:**
```
postgres://username:password@host:port/database?options
```

**Example:**
```bash
POSTGRES_URI="postgres://bridge:secret@localhost:5432/bridge_db?sslmode=disable"
```

**Connection Pool Settings:**

#### `POSTGRES_MAX_CONNS`
**Type:** `integer`  
**Default:** `25`

Maximum number of connections in the pool.

#### `POSTGRES_MIN_CONNS`
**Type:** `integer`  
**Default:** `0`

Minimum number of connections kept open.

#### `POSTGRES_MAX_CONN_LIFETIME`
**Type:** `duration`  
**Default:** `1h`

Maximum lifetime of a connection. Format: `1h`, `30m`, `90s`, etc.

#### `POSTGRES_MAX_CONN_LIFETIME_JITTER`
**Type:** `duration`  
**Default:** `10m`

Random jitter added to connection lifetime to prevent thundering herd.

#### `POSTGRES_MAX_CONN_IDLE_TIME`
**Type:** `duration`  
**Default:** `30m`

Maximum idle time before a connection is closed.

#### `POSTGRES_HEALTH_CHECK_PERIOD`
**Type:** `duration`  
**Default:** `1m`

How often to check connection health.

#### `POSTGRES_LAZY_CONNECT`
**Type:** `boolean`  
**Default:** `false`

Enable lazy connection initialization. If `true`, connections are created on-demand.

**Example:**
```bash
POSTGRES_URI="postgres://user:pass@localhost/db" \
POSTGRES_MAX_CONNS=50 \
POSTGRES_MIN_CONNS=5 \
POSTGRES_MAX_CONN_LIFETIME=2h \
./bridge
```

### `VALKEY_URI`
**Type:** `string`  
**Default:** _(empty)_  
**Applies to:** Bridge v3 with `STORAGE=valkey`

Valkey (Redis fork) connection string for pub/sub storage.

**Format:**
```
valkey://[:password@]host:port[/database]
redis://[:password@]host:port[/database]
```

**Example:**
```bash
STORAGE=valkey VALKEY_URI="valkey://localhost:6379/0" ./bridge3
```

**Cluster Example:**
```bash
STORAGE=valkey VALKEY_URI="valkey://node1:6379,node2:6379,node3:6379" ./bridge3
```

## Performance & Limits

### `HEARTBEAT_INTERVAL`
**Type:** `integer` (seconds)  
**Default:** `10`  
**Applies to:** Bridge v1, v3

Interval for sending heartbeat messages to keep SSE connections alive.

**Example:**
```bash
HEARTBEAT_INTERVAL=5 ./bridge
```

**Recommendation:**
- Development: `5-10` seconds
- Production: `10-15` seconds

### `RPS_LIMIT`
**Type:** `integer`  
**Default:** `1`  
**Applies to:** Bridge v1, v3

Rate limit for requests per second per IP address to `/bridge/message` endpoint.

**Example:**
```bash
RPS_LIMIT=100 ./bridge3
```

**Note:** Set to `0` to disable rate limiting (not recommended in production).

### `RATE_LIMITS_BY_PASS_TOKEN`
**Type:** `[]string` (comma-separated)  
**Default:** _(empty)_  
**Applies to:** Bridge v1, v3

Bearer tokens that bypass rate limiting. Useful for trusted clients.

**Example:**
```bash
RATE_LIMITS_BY_PASS_TOKEN="secret-token-1,secret-token-2" ./bridge
```

**Usage:**
```bash
curl -H "Authorization: Bearer secret-token-1" \
  -X POST http://localhost:8081/bridge/message
```

### `CONNECTIONS_LIMIT`
**Type:** `integer`  
**Default:** `50`  
**Applies to:** Bridge v1, v3

Maximum number of concurrent SSE connections per IP address.

**Example:**
```bash
CONNECTIONS_LIMIT=200 ./bridge3
```

**Recommendation:**
- Small deployment: `50-100`
- Medium deployment: `200-500`
- Large deployment: `1000+` (requires tuning OS limits)

### `MAX_BODY_SIZE`
**Type:** `integer` (bytes)  
**Default:** `10485760` (10 MB)  
**Applies to:** Bridge v1, v3

Maximum HTTP request body size.

**Example:**
```bash
MAX_BODY_SIZE=5242880 ./bridge  # 5 MB
```

## CORS & Security

### `CORS_ENABLE`
**Type:** `boolean`  
**Default:** `false`  
**Applies to:** Bridge v1, v3

Enable Cross-Origin Resource Sharing (CORS) headers.

**Example:**
```bash
CORS_ENABLE=true ./bridge3
```

**When enabled, allows:**
- All origins (`*`)
- Methods: `GET`, `POST`, `OPTIONS`
- Credentials: `true`
- Max age: `86400` seconds (24 hours)

### `TRUSTED_PROXY_RANGES`
**Type:** `[]string` (comma-separated CIDR ranges)  
**Default:** `0.0.0.0/0`  
**Applies to:** Bridge v1, v3

IP ranges to trust for `X-Forwarded-For` header (for accurate rate limiting behind proxies).

**Example:**
```bash
# Trust internal network and Cloudflare IPs
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,173.245.48.0/20" ./bridge
```

### `SELF_SIGNED_TLS`
**Type:** `boolean`  
**Default:** `false`  
**Applies to:** Bridge v1, v3

Enable self-signed TLS certificate for HTTPS (development only).

**Example:**
```bash
SELF_SIGNED_TLS=true ./bridge
```

**⚠️ Warning:** Do not use in production. Use proper TLS termination (nginx, Cloudflare, etc.).

## Caching

### `CONNECT_CACHE_SIZE`
**Type:** `integer`  
**Default:** `2000000`  
**Applies to:** Bridge v1, v3

Maximum number of entries in the connect client cache.

**Example:**
```bash
CONNECT_CACHE_SIZE=5000000 ./bridge
```

### `CONNECT_CACHE_TTL`
**Type:** `integer` (seconds)  
**Default:** `300` (5 minutes)  
**Applies to:** Bridge v1, v3

Time-to-live for connect client cache entries.

**Example:**
```bash
CONNECT_CACHE_TTL=600 ./bridge  # 10 minutes
```

### `ENABLE_TRANSFERED_CACHE`
**Type:** `boolean`  
**Default:** `true`  
**Applies to:** Bridge v1, v3

Enable in-memory cache for transferred messages (prevents duplicates).

**Example:**
```bash
ENABLE_TRANSFERED_CACHE=false ./bridge
```

### `ENABLE_EXPIRED_CACHE`
**Type:** `boolean`  
**Default:** `true`  
**Applies to:** Bridge v1, v3

Enable in-memory cache for expired messages (improves performance).

**Example:**
```bash
ENABLE_EXPIRED_CACHE=false ./bridge
```

## Event Management

### `DISCONNECT_EVENTS_TTL`
**Type:** `integer` (seconds)  
**Default:** `3600` (1 hour)  
**Applies to:** Bridge v1, v3

How long to keep disconnect events before deletion.

**Example:**
```bash
DISCONNECT_EVENTS_TTL=7200 ./bridge  # 2 hours
```

### `DISCONNECT_EVENT_MAX_SIZE`
**Type:** `integer` (bytes)  
**Default:** `512`  
**Applies to:** Bridge v1, v3

Maximum size of a disconnect event message.

**Example:**
```bash
DISCONNECT_EVENT_MAX_SIZE=1024 ./bridge
```

## Webhooks & Integration

### `WEBHOOK_URL`
**Type:** `string`  
**Default:** _(empty)_  
**Applies to:** Bridge v1, v3

URL to send webhook notifications for bridge events.

**Example:**
```bash
WEBHOOK_URL="https://myapp.com/bridge-webhook" ./bridge
```

### `COPY_TO_URL`
**Type:** `string`  
**Default:** _(empty)_  
**Applies to:** Bridge v1, v3

URL to copy/mirror all messages to (for debugging or analytics).

**Example:**
```bash
COPY_TO_URL="https://analytics.myapp.com/bridge-messages" ./bridge
```

## Monitoring & Profiling

### `PPROF_ENABLED`
**Type:** `boolean`  
**Default:** `true`  
**Applies to:** Bridge v1, v3

Enable Go profiling endpoints at `/debug/pprof/*`.

**Example:**
```bash
PPROF_ENABLED=false ./bridge
```

**Available profiles:**
- `/debug/pprof/` - Index
- `/debug/pprof/heap` - Heap memory
- `/debug/pprof/goroutine` - Goroutines
- `/debug/pprof/profile` - CPU profile

See [net/http/pprof documentation](https://pkg.go.dev/net/http/pprof).

### `TF_ANALYTICS_ENABLED`
**Type:** `boolean`  
**Default:** `false`  
**Applies to:** Bridge v1, v3

Enable TonConnect analytics integration.

**Example:**
```bash
TF_ANALYTICS_ENABLED=true ./bridge
```

## Metadata

### `BRIDGE_NAME`
**Type:** `string`  
**Default:** `ton-connect-bridge`  
**Applies to:** Bridge v1, v3

Bridge instance name for metrics and logging.

**Example:**
```bash
BRIDGE_NAME="my-bridge-prod-1" ./bridge
```

### `BRIDGE_VERSION`
**Type:** `string`  
**Default:** `1.0.0`  
**Applies to:** Bridge v1, v3

Bridge version (automatically set during build).

### `BRIDGE_URL`
**Type:** `string`  
**Default:** `localhost`  
**Applies to:** Bridge v1, v3

Public URL of the bridge for analytics/reporting.

**Example:**
```bash
BRIDGE_URL="https://bridge.myapp.com" ./bridge
```

### `ENVIRONMENT`
**Type:** `string`  
**Default:** `production`  
**Applies to:** Bridge v1, v3

Environment name for logging and monitoring.

**Example:**
```bash
ENVIRONMENT="staging" ./bridge
```

### `NETWORK_ID`
**Type:** `string`  
**Default:** `-239`  
**Applies to:** Bridge v1, v3

TON network identifier.

**Valid values:**
- `-239` - Mainnet
- `-3` - Testnet

**Example:**
```bash
NETWORK_ID="-3" ./bridge  # Testnet
```

## Configuration Examples

### Development (Memory)

```bash
LOG_LEVEL=debug
PORT=8081
METRICS_PORT=9103
STORAGE=memory
CORS_ENABLE=true
HEARTBEAT_INTERVAL=5
CONNECTIONS_LIMIT=50
```

### Production (Bridge v1 + PostgreSQL)

```bash
LOG_LEVEL=info
PORT=8081
METRICS_PORT=9103
POSTGRES_URI="postgres://bridge:strong_password@db.internal:5432/bridge?sslmode=require"
POSTGRES_MAX_CONNS=100
POSTGRES_MIN_CONNS=10
POSTGRES_MAX_CONN_LIFETIME=2h
CORS_ENABLE=true
HEARTBEAT_INTERVAL=10
RPS_LIMIT=100
CONNECTIONS_LIMIT=500
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12"
PPROF_ENABLED=false
ENVIRONMENT=production
BRIDGE_URL="https://bridge.myapp.com"
```

### Production (Bridge v3 + Valkey)

```bash
LOG_LEVEL=info
PORT=8081
METRICS_PORT=9103
STORAGE=valkey
VALKEY_URI="valkey://valkey.internal:6379/0"
CORS_ENABLE=true
HEARTBEAT_INTERVAL=10
RPS_LIMIT=1000
CONNECTIONS_LIMIT=2000
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12"
CONNECT_CACHE_SIZE=5000000
CONNECT_CACHE_TTL=300
PPROF_ENABLED=false
ENVIRONMENT=production
BRIDGE_URL="https://bridge-v3.myapp.com"
```

### High-Performance Tuning

```bash
# For handling 10k+ concurrent connections
STORAGE=valkey
VALKEY_URI="valkey://cluster-node-1:6379,cluster-node-2:6379,cluster-node-3:6379"
CONNECTIONS_LIMIT=10000
RPS_LIMIT=10000
HEARTBEAT_INTERVAL=15
CONNECT_CACHE_SIZE=10000000
CONNECT_CACHE_TTL=600
POSTGRES_MAX_CONNS=200
```

## Environment File

Create a `.env` file for easier configuration:

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

**Load with:**
```bash
export $(cat .env | xargs) && ./bridge3
```

Or use docker-compose:
```yaml
version: '3.8'
services:
  bridge:
    image: bridge3
    env_file: .env
```

## Validation

The bridge validates configuration on startup and logs warnings for:
- Invalid `LOG_LEVEL` values
- Missing required `POSTGRES_URI` or `VALKEY_URI`
- Invalid duration formats
- Invalid CIDR ranges in `TRUSTED_PROXY_RANGES`

Check logs on startup for configuration issues.
