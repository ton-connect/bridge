# Architecture

TON Connect Bridge uses pub/sub architecture to synchronize state across multiple instances, enabling true high-availability deployments. It is designed for horizontal scaling with Redis-compatible storage.

## How It Works

**Deployment:**
- Run 1, 3, 10, or more bridge instances simultaneously
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
2. Bridge publishes message to Redis pub/sub channel (instant delivery to all instances)
3. Bridge stores message in Redis sorted set (for offline clients)
4. All bridge instances with subscribed clients receive the message via pub/sub
5. Bridge instances deliver message to their connected clients via SSE

## Architecture

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

## Scaling Requirements

**Redis Version:**
- Redis 7.0+ (or Valkey, or any Redis-compatible database) for production deployments

**Redis Deployment Options:**
- Single-node Redis (small deployments)
- Redis Cluster (high availability and scale)
- Managed services: AWS ElastiCache, GCP Memorystore, Azure Cache for Redis

**Bridge Instances:**
- Run any number of instances (1, 3, 10+)
- Each instance handles its own SSE connections
- All instances share state through Redis

## Storage Options

Bridge supports:

- **Redis/Valkey** (recommended for production) - Full pub/sub support, horizontal scaling
- **Memory** (development/testing) - No persistence, single instance only

For production deployments, **always use Redis or Valkey** to enable high availability.
