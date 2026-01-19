# TON Connect Bridge v1 (Legacy)

> **⚠️ Warning:** Bridge v1 is deprecated and will be removed in future versions. 
> 
> **Please use [TON Connect Bridge v3](../../README.md) for all new deployments.**
>
> Bridge v1 is the original implementation of TON Connect Bridge. It was production-proven but has a fundamental limitation: **it cannot be horizontally scaled** due to its in-memory message storage architecture.

---

## Architecture

Bridge v1 uses a single application instance with in-memory caching backed by PostgreSQL for persistence.

### How It Works

**Message Storage:**
- Messages are pushed directly between clients in real-time via SSE
- Messages are pushed to PostgreSQL for persistent storage

**Client Subscription Flow:**
1. Client subscribes to messages via SSE (`GET /bridge/events`)
2. Bridge reads all pending messages from PostgreSQL first
3. Bridge pushes these messages to the client
4. Bridge continues serving new messages via SSE in real-time

**Message Sending Flow:**
1. Client sends message via `POST /bridge/message`
2. Bridge immediately pushes message to all subscribed clients via SSE
3. Bridge writes message to PostgreSQL for persistence

### Architecture Diagram

```
  Internet
     │
     ├── TLS Termination (Cloudflare/nginx)
     │
     ▼
┌────────┐
│ Bridge │──── PostgreSQL
│   v1   │
└────────┘
```

### Fundamental Limitation

**Bridge v1 cannot be horizontally scaled.** Since messages are stored in the memory of a single application instance, running multiple bridge instances would result in:
- Messages sent to instance A not visible to clients connected to instance B
- No way to synchronize in-memory state across instances
- Clients unable to receive messages if connected to different instances

This limitation led to the development of Bridge v3.

## Building & Running

### Build

```bash
make build
./bridge
```

### Storage Options

- **PostgreSQL** (required for production)
- **Memory only** (development/testing, no persistence)

## Configuration

### PostgreSQL Settings

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `POSTGRES_URI` | string | - | **Required for production**<br>Format: `postgres://user:pass@host:port/db?sslmode=require` |
| `POSTGRES_MAX_CONNS` | int | `25` | Max connections in pool |
| `POSTGRES_MIN_CONNS` | int | `0` | Min idle connections |
| `POSTGRES_MAX_CONN_LIFETIME` | duration | `1h` | Connection lifetime (`1h`, `30m`, `90s`) |
| `POSTGRES_MAX_CONN_LIFETIME_JITTER` | duration | `10m` | Random jitter to prevent thundering herd |
| `POSTGRES_MAX_CONN_IDLE_TIME` | duration | `30m` | Max idle time before closing |
| `POSTGRES_HEALTH_CHECK_PERIOD` | duration | `1m` | Health check interval |
| `POSTGRES_LAZY_CONNECT` | bool | `false` | Create connections on-demand |

### Production Configuration Example

```bash
LOG_LEVEL=info
POSTGRES_URI="postgres://bridge:${PASSWORD}@db.internal:5432/bridge?sslmode=require"
POSTGRES_MAX_CONNS=100
POSTGRES_MIN_CONNS=10
CORS_ENABLE=true
RPS_LIMIT=10000
CONNECTIONS_LIMIT=50000
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12,{use_your_own}"
ENVIRONMENT=production
BRIDGE_URL="https://use-your-own-bridge.myapp.com"
```

## Deployment

### Docker Deployment

```bash
docker compose -f docker/docker-compose.postgres.yml up -d
```

### Environment File

```bash
# .env
LOG_LEVEL=info
PORT=8081
POSTGRES_URI=postgres://bridge:password@postgres:5432/bridge
CORS_ENABLE=true
RPS_LIMIT=1000
CONNECTIONS_LIMIT=5000
```

## Support

Bridge v1 receives security updates only. For new features and improvements, please use Bridge v3.

**Questions?** See the [main documentation](../../docs/) or open an issue.
