# Configuration

Complete reference for all environment variables supported by TON Connect Bridge.

## Core Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_LEVEL` | string | `info` | Logging level<br>`panic` `fatal` `error` `warn` `info` `debug` `trace` |
| `PORT` | int | `8081` | HTTP server port for bridge endpoints |
| `METRICS_PORT` | int | `9103` | Metrics port: `/health` `/ready` `/metrics` `/version` `/debug/pprof/*` |
| `PPROF_ENABLED` | bool | `true` | See [pprof docs](https://pkg.go.dev/net/http/pprof) |

## Storage

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `STORAGE` | string | `memory` | `valkey` (cluster required) or `memory` (dev only) |
| `VALKEY_URI` | string | - | Cluster format: `rediss://default:@clustercfg.example.com:6379?skip_verify=true` |

## Performance & Limits

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HEARTBEAT_INTERVAL` | int | `10` | SSE heartbeat interval (seconds) |
| `RPS_LIMIT` | int | `1` | Requests/sec per IP for `/bridge/message` |
| `CONNECTIONS_LIMIT` | int | `50` | Max concurrent SSE connections per IP |
| `MAX_BODY_SIZE` | int | `10485760` | Max HTTP request body size (bytes) for `/bridge/message` |
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

## Webhooks

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `WEBHOOK_URL` | string | - | URL for bridge event notifications |
| `COPY_TO_URL` | string | - | Mirror all messages to URL (debugging/analytics) |

## TON Analytics

TODO where to read more about it?

| Variable                       |  Type  | Default | Description                                                  |
|--------------------------------|--------|---------|--------------------------------------------------------------|
| `TON_ANALYTICS_ENABLED`        | bool   | `false` | Enable TonConnect analytics                                  |
| `TON_ANALYTICS_URL`            | string | `https://analytics.ton.org/events` | TON Analytics endpoint URL |
| `TON_ANALYTICS_BRIDGE_VERSION` | string | `1.0.0` | Bridge version for analytics tracking (auto-set during build) |
| `TON_ANALYTICS_BRIDGE_URL`     | string | `localhost` | Public bridge URL for analytics                          |
| `TON_ANALYTICS_NETWORK_ID`     | string | `-239`  | TON network: `-239` (mainnet), `-3` (testnet)                |

## NTP Time Synchronization

Bridge v3 supports NTP time synchronization for consistent `event_id` generation across multiple instances. This ensures monotonic event ordering even when bridge instances run on different servers.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `NTP_ENABLED` | bool | `true` | Enable NTP time synchronization |
| `NTP_SERVERS` | string | `time.google.com,time.cloudflare.com,pool.ntp.org` | Comma-separated NTP server list |
| `NTP_SYNC_INTERVAL` | int | `300` | NTP sync interval (seconds) |
| `NTP_QUERY_TIMEOUT` | int | `5` | NTP query timeout (seconds) |

**Note:** NTP synchronization is only available in bridge v3. Bridge v1 uses local system time.

## Configuration Presets

### 🧪 Development (Memory)

```bash
LOG_LEVEL=debug
STORAGE=memory
CORS_ENABLE=true
HEARTBEAT_INTERVAL=10
RPS_LIMIT=50
CONNECTIONS_LIMIT=50
NTP_ENABLED=true
```

### 🚀 Production (Redis/Valkey Cluster)

```bash
LOG_LEVEL=info
STORAGE=valkey
VALKEY_URI="rediss://username:yourpassword@clustercfg.example.com:6379?skip_verify=true"
CORS_ENABLE=true
RPS_LIMIT=100000
CONNECTIONS_LIMIT=500000
CONNECT_CACHE_SIZE=500000
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,{use_your_own_please}"
ENVIRONMENT=production
BRIDGE_URL="https://use-your-own-bridge.myapp.com"
NTP_ENABLED=true
NTP_SERVERS=time.google.com,time.cloudflare.com,pool.ntp.org
NTP_SYNC_INTERVAL=300
```

## Using Environment Files

<details>
<summary><b>💾 .env file</b></summary>

```bash
# .env
LOG_LEVEL=info
PORT=8081
STORAGE=valkey
VALKEY_URI=rediss://clustercfg.example.com:6379
CORS_ENABLE=true
RPS_LIMIT=100
CONNECTIONS_LIMIT=200
```

**Load:**
```bash
export $(cat .env | xargs) && ./bridge3
```

</details>
