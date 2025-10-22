# Deployment Guide

Production-ready deployment patterns and best practices for TON Connect Bridge.

> **Note:** Bridge has two engines:
> - **v1 engine** - Battle-proven, works only with PostgreSQL
> - **v3 engine** - Modern, works only with Redis-compatible databases (Redis, Valkey, AWS ElastiCache, GCP Memorystore, Azure Cache for Redis, etc.)

## Quick Deployment Checklist

- [ ] Choose Bridge version (v1 or v3) based on requirements
- [ ] Select storage backend
- [ ] Configure environment variables
- [ ] Set up TLS termination
- [ ] Configure load balancer (if multi-instance)
- [ ] Enable monitoring and alerts
- [ ] Test health endpoints
- [ ] Set up rate limiting
- [ ] Run load tests with built-in [`benchmark/`](../benchmark/) tools

## Architecture Patterns

### Pattern 1: Single Instance (Small Scale)

BridgeV1 + PostgreSQL or BridgeV3 + Redis

**Best for:** <1,000 concurrent connections, low traffic

```
  Internet
     │
     ├── TLS Termination (Cloudflare/nginx)
     │
     ▼
┌────────┐
│ Bridge │──── PostgreSQL or Redis
└────────┘
```

**Pros:**
- Simple setup
- Easy to manage
- Low cost

**Cons:**
- Single point of failure
- Limited scaling
- No high availability

### Pattern 2: Multi-Instance with Load Balancer

**Best for:** >1,000 concurrent connections, high availability

```
Internet
  │
  ├── TLS Termination & Load Balancer
  │
  ├──────┬──────┬──────┐
  │      │      │      │
  ▼      ▼      ▼      ▼
┌───┐  ┌───┐  ┌───┐  ┌───┐
│Bv3│  │Bv3│  │Bv3│  │Bv3│ Bridge Instances
└───┘  └───┘  └───┘  └───┘
   │      │      │      │
   └──────┴──────┴──────┘
            │
            ▼
   ┌─────────────────┐
   │  Redis Cluster  │
   └─────────────────┘
```

**Pros:**
- High availability
- Horizontal scaling
- Load distribution

**Cons:**
- More complex setup
- Requires shared storage
- Higher cost

### Pattern 3: Multi-Region (Global)

**Best for:** Global audience, ultra-low latency requirements

```
     Region 1              Region 2              Region 3
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│ Load Balancer │      │ Load Balancer │      │ Load Balancer │
└───────┬───────┘      └───────┬───────┘      └───────┬────────┘
        │                      │                      │
     ┌──┴──┐                ┌──┴──┐                ┌──┴──┐
     │ Bv3 │                │ Bv3 │                │ Bv3 │
     └──┬──┘                └──┬──┘                └──┬──┘
        │                      │                      │
        └──────────────────────┴──────────────────────┘
                               │
                         Redis Cluster
```

**Requirements:**
- Bridge v3 with Redis
- Multi-region Redis cluster with replication
- Geographic load balancing (DNS/CDN)

## Deployment Methods

For inspiration you may take a look at [docker/](docker/) folder.

