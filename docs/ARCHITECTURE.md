# Architecture

Complete technical reference for TON Connect Bridge architecture and storage backends.

# Bridge Architecture

This repository contains two bridge implementations for TON Connect 2.0:

- **Bridge v1** (`./cmd/bridge`) - The original, production-proven implementation
- **Bridge v3** (`./cmd/bridge3`) - Next-generation implementation with pub/sub architecture

## Bridge v1

### Design Philosophy

Bridge v1 uses a traditional request-response model with Server-Sent Events (SSE) for real-time communication. It was designed for reliability and persistence, making it the go-to choice for production environments.

### Architecture

```
┌─────────┐         ┌─────────────┐         ┌────────────┐
│  Wallet │ ◄─SSE── │  Bridge v1  │ ◄─────► │ PostgreSQL │
└─────────┘         │  (polling)  │         └────────────┘
     │              └─────────────┘
     │ POST /bridge/message
     ▼
┌─────────┐
│   dApp  │
└─────────┘
```

**Key characteristics:**
- Messages stored in PostgreSQL for durability
- Long polling pattern for message retrieval
- Simple, predictable behavior
- Battle-tested in production

### Storage Options

- **PostgreSQL** (required for production)
- **Memory** (development/testing only)

---

## Bridge v3

### Design Philosophy

Bridge v3 introduces a pub/sub architecture optimized for high-throughput, real-time messaging. It's designed to scale horizontally and handle thousands of concurrent connections efficiently.

### Architecture

```
┌─────────┐         ┌─────────────┐         ┌─────────┐
│  Wallet │ ◄─SSE── │  Bridge v3  │ ◄─────► │ Valkey  │
└─────────┘         │  (pub/sub)  │         │ Pub/Sub │
     │              └─────────────┘         └─────────┘
     │ POST /bridge/message                      │
     ▼              Pub ─────────────────────────┘
┌─────────┐
│   dApp  │
└─────────┘
```

**Key characteristics:**
- Native pub/sub using Valkey (Redis fork)
- Zero database queries for message delivery
- Horizontal scaling with multiple bridge instances
- Sub-second message latency
- Memory-efficient connection handling



### Storage Options

- **Valkey** (recommended for production)
- **Memory** (development/testing)

# Storage Backends

Bridge supports multiple storage backends for different use cases and performance requirements.

## Memory Storage

In-memory storage with no persistence. Fast and simple, but data is lost on restart.

**Features:**
- ✅ Zero configuration
- ✅ Instant message delivery
- ✅ No external dependencies
- ❌ No persistence across restarts
- ❌ Single instance only
- ❌ Limited by available RAM

**Configuration:**

**Bridge v1:**
```bash
# Memory used automatically when POSTGRES_URI is not set
./bridge
```

**Bridge v3:**
```bash
STORAGE=memory ./bridge3
```

**Best for:**
- Local development
- Testing and CI/CD
- Proof of concepts
- Short-lived deployments

## PostgreSQL Storage

Relational database with full ACID guarantees and persistent message storage.

**Features:**
- ✅ Full message persistence
- ✅ ACID transactions
- ✅ Proven reliability
- ✅ Backup/restore support
- ⚠️ Requires polling (Bridge v1)
- ⚠️ Limited pub/sub (Bridge v3)
- ❌ Vertical scaling only

**Bridge v1 Support:** ✅ **Full support** (recommended)  

**Best for:**
- Production (Bridge v1)
- Applications requiring message persistence
- Compliance/audit requirements
- Moderate traffic (<1,000 concurrent connections)

---

## Valkey Storage

High-performance pub/sub storage using Valkey (Redis fork). Designed for real-time, high-throughput messaging.

**Features:**
- ✅ Native pub/sub architecture
- ✅ Sub-second message latency
- ✅ Horizontal scaling
- ✅ Cluster support
- ✅ Optional persistence (AOF/RDB)
- ⚠️ Eventual consistency
- ❌ No ACID guarantees

**Bridge v1 Support:** ❌ Not supported  
**Bridge v3 Support:** ✅ **Recommended for production**

### Configuration

```bash
STORAGE=valkey
VALKEY_URI="valkey://[:password@]host:port[/database]"
```

**Single instance:**
```bash
STORAGE=valkey \
VALKEY_URI="valkey://localhost:6379/0" \
./bridge3
```

**With password:**
```bash
STORAGE=valkey \
VALKEY_URI="valkey://:my_strong_password@localhost:6379/0" \
./bridge3
```

**Cluster:**
```bash
STORAGE=valkey \
VALKEY_URI="valkey://node1:6379,node2:6379,node3:6379" \
./bridge3
```

### Persistence Options

Valkey supports optional persistence:

**RDB (Snapshot):**
```conf
# valkey.conf
save 900 1      # Save after 900s if 1 key changed
save 300 10     # Save after 300s if 10 keys changed
save 60 10000   # Save after 60s if 10000 keys changed
```

**AOF (Append-Only File):**
```conf
# valkey.conf
appendonly yes
appendfsync everysec
```

**Best for:**
- Production (Bridge v3)
- High-throughput applications (>1,000 msg/s)
- Real-time messaging requirements
- Horizontal scaling needs
- Low-latency requirements (<100ms)

---

## Storage Comparison

### Performance

| Storage | Avg Latency | P99 Latency | Throughput | Concurrent Connections |
|---------|-------------|-------------|------------|------------------------|
| Memory | <1ms | <5ms | ~10,000 msg/s | 1,000 |
| PostgreSQL (v1) | 100-500ms | 1-3s | ~200 msg/s | 500 |
| Valkey (v3) | <10ms | <50ms | ~50,000 msg/s | 10,000+ |

### Resource Usage

**Memory per 1,000 connections:**
- Memory storage: ~50 MB
- PostgreSQL: ~100 MB (+ database)
- Valkey: ~30 MB (+ Valkey instance)

**CPU usage:**
- Memory: Negligible
- PostgreSQL: Medium (polling overhead)
- Valkey: Low (event-driven)

### Selection Guide

**Choose Memory if:**
- ✅ Local development
- ✅ Testing/CI/CD
- ✅ Don't need persistence

**Choose PostgreSQL if:**
- ✅ Need guaranteed persistence
- ✅ Existing PostgreSQL infrastructure
- ✅ Moderate traffic (<1,000 connections)
- ✅ Using Bridge v1

**Choose Valkey if:**
- ✅ High performance required
- ✅ Low latency critical (<100ms)
- ✅ High concurrent connections (>1,000)
- ✅ Using Bridge v3

---

## See Also

- [API Reference](API.md) - HTTP endpoints and usage examples
- [Configuration](CONFIGURATION.md) - Environment variables and tuning
- [Deployment](DEPLOYMENT.md) - Production deployment patterns
- [Monitoring](MONITORING.md) - Metrics and observability


### When to Use Bridge v1 or v3

### When to Use Bridge v3

✅ **Choose Bridge v3 when:**
- High throughput and low latency are critical
- You expect thousands of concurrent connections
- You need horizontal scalability
- You're comfortable with Valkey/Redis infrastructure
- Message persistence is less critical (or handled elsewhere)
- Building a new deployment

## Choosing the Right Bridge

### Comparison

| Feature | Bridge v1 | Bridge v3 |
|---------|-----------|-----------|
| **Protocol** | HTTP Long Polling + SSE | Pub/Sub with SSE |
| **Storage** | PostgreSQL, Memory | Memory, Valkey, PostgreSQL* |
| **Latency** | ~1-10 seconds | <100ms |
| **Throughput** | ~1,000 connections | ~10,000+ connections |
| **Scaling** | Vertical | Horizontal |
| **Persistence** | ✅ PostgreSQL | ⚠️ Optional (Valkey AOF/RDB) |
| **Maturity** | Stable, production-proven | Stable |

\* PostgreSQL support for v3 is limited (no pub/sub yet)

### Message Flow

**Bridge v1:**
1. Client connects to `/bridge/events`
2. Bridge queries PostgreSQL every `HEARTBEAT_INTERVAL`
3. New messages returned via SSE
4. Messages marked as delivered in database

**Bridge v3:**
1. Client connects to `/bridge/events`
2. Bridge subscribes to Valkey pub/sub channels
3. Messages pushed instantly via SSE when published
4. No database queries for delivery

### Decision Matrix

| Requirement | Recommendation |
|-------------|----------------|
| Production stability | Bridge v1 |
| High performance | Bridge v3 |
| Message persistence required | Bridge v1 |
| Real-time (<1s latency) | Bridge v3 |
| Simple deployment | Bridge v1 (memory) or v3 (memory) |
| Enterprise scale | Bridge v3 (Valkey cluster) |

### Migration from v1 to v3

Both bridges implement the same HTTP API, making migration straightforward:

1. **Test in parallel**: Run both bridges side-by-side with different endpoints
2. **Update client configuration**: Point clients to the new bridge URL
3. **Monitor metrics**: Compare performance and error rates
4. **Gradual rollout**: Use load balancer to shift traffic gradually
