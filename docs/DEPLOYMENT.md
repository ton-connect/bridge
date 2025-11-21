# Deployment Guide

Production-ready deployment patterns and best practices for TON Connect Bridge v3.

## Quick Deployment Checklist

- [ ] Set up Redis/Valkey 7.0+ **cluster** (Bridge v3 requires cluster mode)
- [ ] Configure environment variables (see [CONFIGURATION.md](CONFIGURATION.md))
- [ ] Deploy multiple Bridge instances for high availability
- [ ] Set up load balancer with health checks
- [ ] Configure TLS termination
- [ ] Enable monitoring and alerts (see [MONITORING.md](MONITORING.md))
- [ ] Set up rate limiting
- [ ] Run load tests with built-in [`benchmark/`](../benchmark/) tools

## Architecture Patterns

> **Note:** Bridge v3 requires Redis/Valkey cluster mode. Single-node deployments are not supported.

### Pattern 1: Multi-Instance with Load Balancer (Recommended)

**Best for:** Production, >10,000 concurrent connections, high availability

```
Internet
  │
  ├── TLS Termination & Load Balancer
  │   (nginx, HAProxy, or cloud LB)
  │
  ├─────────────┬───────────┬────────────┐
  │             │           │            │
  ▼             ▼           ▼            ▼
┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐
│ Bridge │  │ Bridge │  │ Bridge │  │ Bridge │
│Instance│  │Instance│  │Instance│  │Instance│
└────────┘  └────────┘  └────────┘  └────────┘
     │          │          │          │
     └──────────┴──────────┴──────────┘
            │
            ▼
   ┌───────────────┐
   │ Redis Cluster │
   └───────────────┘
```

**Pros:**
- High availability
- Horizontal scaling
- Load distribution
- Zero-downtime deployments

**Cons:**
- More complex setup
- Higher cost

**Key Components:**
- Bridge v3 Instances: 3+ instances for redundancy
- Redis Cluster: 3-6 nodes with replication (required for Bridge v3)
- Load Balancer: Distributes traffic, health checks
- TLS Termination: SSL/TLS at load balancer level

### Pattern 2: Multi-Region (Global)

**Best for:** Global audience, ultra-low latency requirements

```
     Region 1              Region 2              Region 3
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│ Load Balancer │      │ Load Balancer │      │ Load Balancer │
└───────┬───────┘      └───────┬───────┘      └───────┬────────┘
        │                      │                      │
  ┌─────┴──────┐         ┌─────┴──────┐         ┌─────┴──────┐
  │   Bridge   │         │   Bridge   │         │   Bridge   │
  └─────┬──────┘         └─────┬──────┘         └─────┬──────┘
        │                      │                      │
        └──────────────────────┴──────────────────────┘
                               │
                    ┌──────────────────────┐
                    │ Global Redis Cluster │
                    │  (with replication)  │
                    └──────────────────────┘
```

**Requirements:**
- Bridge v3 with Redis cluster
- Multi-region Redis cluster with cross-region replication
- Geographic load balancing (GeoDNS/CDN)
- Low-latency network between regions

