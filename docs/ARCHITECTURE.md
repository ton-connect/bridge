# Architecture

This repository contains two bridge engines for TON Connect 2.0:

- **Bridge v1** (`./cmd/bridge`) - Original implementation, production-proven but not horizontally scalable
- **Bridge v3** (`./cmd/bridge3`) - Modern implementation with pub/sub architecture and horizontal scaling support

## Bridge v1

Bridge v1 was the original implementation. All clients subscribe and send messages through a single bridge application instance.

### How It Works

**Message Storage:**
- Messages are stored in **memory** for fast access
- Messages are simultaneously pushed to **PostgreSQL** for persistent storage

**Client Subscription Flow:**
1. Client subscribes to messages via SSE (`GET /bridge/events`)
2. Bridge reads all pending messages from PostgreSQL first
3. Bridge pushes these messages to the client
4. Bridge continues serving new messages via SSE in real-time

**Message Sending Flow:**
1. Client sends message via `POST /bridge/message`
2. Bridge stores message in memory
3. Bridge immediately sends message to the recipient client if connected (via SSE)
4. Bridge writes message to PostgreSQL for persistence

### Architecture

```
┌─────────┐         ┌─────────────┐         ┌────────────┐
│  Wallet │ ◄─SSE── │  Bridge v1  │ ◄─────► │ PostgreSQL │
└─────────┘         │  (single)   │         │ (persist)  │
     │              │             │         └────────────┘
     │              │   Memory    │
     │              │   (cache)   │
     │              └─────────────┘
     │ POST /bridge/message
     ▼
┌─────────┐
│   dApp  │
└─────────┘
```

### Fundamental Limitation

**Bridge v1 cannot be horizontally scaled.** Since messages are stored in the memory of a single application instance, running multiple bridge instances would result in:
- Messages sent to instance A not visible to clients connected to instance B
- No way to synchronize in-memory state across instances
- Clients unable to receive messages if connected to different instances

This limitation led to the development of Bridge v3.

### Storage Options

- **PostgreSQL** (required for production)
- **Memory only** (development/testing, no persistence)

## Bridge v3

Bridge v3 is designed for **horizontal scaling**. It uses Redis-compatible storage (Redis, Valkey, AWS ElastiCache, etc.) as the primary storage and can run multiple bridge instances simultaneously.

### How It Works

**Deployment:**
- Run 1, 3, 10, or more bridge instances simultaneously
- User setup required: DNS, load balancing, Kubernetes, etc. to present multiple instances as a single endpoint
- All instances share state through Redis pub/sub + sorted sets

**Message Storage & Sharing:**
- **Pub/Sub**: Real-time message delivery across all bridge instances
- **Sorted Sets (ZSET)**: Persistent message storage with TTL-based expiration
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

### Architecture

```
                    Load Balancer / DNS
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
   ┌─────────┐        ┌─────────┐       ┌─────────┐
   │Bridge v3│        │Bridge v3│       │Bridge v3│
   │Instance │        │Instance │       │Instance │
   └────┬────┘        └────┬────┘       └────┬────┘
        │                  │                  │
        └──────────────────┼──────────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │Redis/Valkey │
                    │  Pub/Sub +  │
                    │   ZSETs     │
                    └─────────────┘
```

### Scaling Requirements

**Redis Version:**
- **Redis 7.0+** (or Valkey) is **required** for production deployments
- Uses [Sharded Pub/Sub](https://valkey.io/topics/pubsub/) introduced in Redis 7.0
- Shard channels are assigned to slots by the same algorithm used for keys
- This ensures pub/sub messages are properly distributed in cluster mode

**Redis Deployment Options:**
- Single-node Redis (small deployments)
- Redis Cluster (high availability and scale)
- Managed services: AWS ElastiCache, GCP Memorystore, Azure Cache for Redis

**Bridge Instances:**
- Run any number of instances (1, 3, 10+)
- Each instance handles its own SSE connections
- All instances share state through Redis

### Storage Options

- **Redis/Valkey** (required for production, supports clustering)
- **Memory only** (development/testing, single instance only)

# Storage Backends

Bridge supports multiple storage backends for different use cases and performance requirements.

## Memory Storage

Supported for both BridgeV1 engine and BridgeV3 engine. Do not use it in your production environment.

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

### When to Use Bridge v1 or v3

### Message Flow

**Bridge v1:**
1. Client connects to `/bridge/events`
2. Bridge queries PostgreSQL every `HEARTBEAT_INTERVAL`
3. New messages returned via SSE
4. Messages marked as delivered in database

**Bridge v3:**
1. Client connects to `/bridge/events`
2. Bridge subscribes to Valkey pub/sub channels, dublicate message to SET
3. Messages pushed instantly via SSE when published
4. No database queries for delivery
