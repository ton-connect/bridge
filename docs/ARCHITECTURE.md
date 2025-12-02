# Architecture

TON Connect Bridge uses pub/sub architecture to synchronize state across multiple instances, enabling true high-availability deployments. It is designed for horizontal scaling with Redis-compatible storage.

```
                Load Balancer / DNS
                        │
        ┌───────────────┼───────────────┐
        │               │               │
        ▼               ▼               ▼
   ┌────────┐      ┌────────┐      ┌────────┐
   │BridgeV3│      │BridgeV3│      │BridgeV3│
   │Instance│      │Instance│      │Instance│
   └───┬────┘      └────┬───┘      └────┬───┘
       │                │               │
       └────────────────┼───────────────┘
                        │
                        ▼
        ┌───────────────────────────────┐
        │ Clustered/Not Clustered Redis │
        └───────────────────────────────┘
```

## How It Works

**Deployment:**
- Run any number of bridge instances simultaneously
- User setup required: DNS, load balancing, Kubernetes, etc. to present multiple instances as a single endpoint
- All instances share state through Redis pub/sub + sorted sets

**Message Storage & Sharing:**
- Pub/Sub: Real-time message delivery across all bridge instances
- Sorted Sets (ZSET): Persistent message storage with TTL-based expiration
- All bridge instances subscribe to the same Redis channels
- Messages published to Redis are instantly visible to all instances

**Client Subscription Flow:**
1. Client subscribes to messages via SSE (`GET /bridge/events`)
2. Bridge subscribes to Redis pub/sub channel for that client
3. Bridge reads pending messages from Redis sorted set (ZRANGE)
4. Bridge pushes historical messages to the client
5. Bridge continues serving new messages via pub/sub in real-time

**Message Sending Flow:**
1. Client sends message via `POST /bridge/message`
2. Bridge generates a monotonic event ID using time-based generation
3. Bridge publishes message to Redis pub/sub channel (instant delivery to all instances)
4. Bridge stores message in Redis sorted set (for offline clients)
5. All bridge instances with subscribed clients receive the message via pub/sub
6. Bridge instances deliver message to their connected clients via SSE

## Time Synchronization

**Event ID Generation:**
- Bridge uses time-based event IDs to ensure monotonic ordering across instances
- Format: `(timestamp_ms << 11) | local_counter` (53 bits total for JavaScript compatibility)
- 42 bits for timestamp (supports dates up to year 2100), 11 bits for counter
- Provides ~2K events per millisecond per instance

**NTP Synchronization (Optional):**
- When enabled, all bridge instances synchronize their clocks with NTP servers
- Improves event ordering consistency across distributed instances
- Fallback to local system time if NTP is unavailable
- Configuration: `NTP_ENABLED`, `NTP_SERVERS`, `NTP_SYNC_INTERVAL`

**Time Provider Architecture:**
- Uses `TimeProvider` interface for clock abstraction
- `ntp.Client`: NTP-synchronized time (recommended for multi-instance deployments)
- `ntp.LocalTimeProvider`: Local system time (single instance or testing)

## Scaling Requirements

**Redis Version:**
- Redis 7.0+ (or Valkey, or any Redis-compatible database) for production deployments

**Redis Deployment Options:**
- Redis Cluster (required for Bridge v3 - high availability and scale)
- Managed services: AWS ElastiCache, GCP Memorystore, Azure Cache for Redis

**Bridge Instances:**
- Run any number of instances simultaneously
- Each instance handles its own SSE connections
- All instances share state through Redis

## Storage Options

Bridge v3 supports:

- **Redis/Valkey Cluster** (required for production) - Full pub/sub support, horizontal scaling
- **Memory** (development/testing only) - No persistence, single instance only

For production deployments, **always use Redis or Valkey in cluster mode** to enable high availability.
