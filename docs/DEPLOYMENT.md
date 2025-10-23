# Deployment Guide

Production-ready deployment patterns and best practices for TON Connect Bridge v3.

## Quick Deployment Checklist

- [ ] Set up Redis/Valkey cluster or managed service
- [ ] Configure environment variables (see [CONFIGURATION.md](CONFIGURATION.md))
- [ ] Deploy multiple Bridge v3 instances for high availability
- [ ] Set up load balancer with health checks
- [ ] Configure TLS termination
- [ ] Enable monitoring and alerts (see [MONITORING.md](MONITORING.md))
- [ ] Set up rate limiting
- [ ] Run load tests with built-in [`benchmark/`](../benchmark/) tools

## Architecture Patterns

### Pattern 1: Single Instance (Development/Small Scale)

Bridge v3 + Redis

**Best for:** Development, testing, <1,000 concurrent connections

```
  Internet
     │
     ├── TLS Termination (Cloudflare/nginx)
     │
     ▼
┌────────┐
│Bridge  │──── Redis/Valkey
│  v3    │
└────────┘
```

**Pros:**
- Simple setup
- Easy to test
- Low cost

**Cons:**
- Single point of failure
- Limited scaling

### Pattern 2: Multi-Instance with Load Balancer (Recommended)

**Best for:** Production, >1,000 concurrent connections, high availability

```
Internet
  │
  ├── TLS Termination & Load Balancer
  │   (nginx, HAProxy, or cloud LB)
  │
  ├──────┬──────┬──────┐
  │      │      │      │
  ▼      ▼      ▼      ▼
┌───┐  ┌───┐  ┌───┐  ┌───┐
│Bv3│  │Bv3│  │Bv3│  │Bv3│ Bridge v3 Instances
└───┘  └───┘  └───┘  └───┘
   │      │      │      │
   └──────┴──────┴──────┘
            │
            ▼
   ┌─────────────────┐
   │  Redis Cluster  │
   │  (3+ nodes)     │
   └─────────────────┘
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
- **Bridge v3 Instances:** 3+ instances for redundancy
- **Redis Cluster:** 3-6 nodes with replication
- **Load Balancer:** Distributes traffic, health checks
- **TLS Termination:** SSL/TLS at load balancer level

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
     │ x3  │                │ x3  │                │ x3  │
     └──┬──┘                └──┬──┘                └──┬──┘
        │                      │                      │
        └──────────────────────┴──────────────────────┘
                               │
                     ┌─────────────────────┐
                     │ Global Redis Cluster│
                     │  (with replication) │
                     └─────────────────────┘
```

**Requirements:**
- Bridge v3 with Redis cluster
- Multi-region Redis cluster with cross-region replication
- Geographic load balancing (GeoDNS/CDN)
- Low-latency network between regions

## Deployment Methods

### Docker Compose (Quick Start)

For local development and testing:

```bash
# Start Bridge v3 with Redis
docker compose -f docker/docker-compose.valkey.yml up -d

# Check health
curl http://localhost:9103/health
```

### Kubernetes (Production)

Example Kubernetes deployment for Bridge v3:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bridge-v3
spec:
  replicas: 3
  selector:
    matchLabels:
      app: bridge-v3
  template:
    metadata:
      labels:
        app: bridge-v3
    spec:
      containers:
      - name: bridge
        image: ghcr.io/ton-connect/bridge3:latest
        env:
        - name: STORAGE
          value: "valkey"
        - name: VALKEY_URI
          valueFrom:
            secretKeyRef:
              name: bridge-secrets
              key: valkey-uri
        - name: LOG_LEVEL
          value: "info"
        - name: RPS_LIMIT
          value: "100000"
        - name: CONNECTIONS_LIMIT
          value: "500000"
        ports:
        - containerPort: 8081
          name: http
        - containerPort: 9103
          name: metrics
        livenessProbe:
          httpGet:
            path: /health
            port: 9103
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 9103
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: bridge-v3
spec:
  selector:
    app: bridge-v3
  ports:
  - port: 8081
    targetPort: 8081
    name: http
  - port: 9103
    targetPort: 9103
    name: metrics
  type: LoadBalancer
```

### Managed Cloud Services

#### AWS ElastiCache + ECS/EKS

1. **Create ElastiCache Redis Cluster:**
   ```bash
   aws elasticache create-replication-group \
     --replication-group-id bridge-redis \
     --replication-group-description "Bridge Redis Cluster" \
     --engine redis \
     --cache-node-type cache.r6g.xlarge \
     --num-cache-clusters 3 \
     --automatic-failover-enabled
   ```

2. **Deploy Bridge to ECS/EKS** with `VALKEY_URI` pointing to ElastiCache endpoint

#### GCP Memorystore + GKE

1. **Create Memorystore instance:**
   ```bash
   gcloud redis instances create bridge-redis \
     --size=5 \
     --region=us-central1 \
     --tier=standard
   ```

2. **Deploy Bridge to GKE** with `VALKEY_URI` pointing to Memorystore

#### Azure Cache for Redis + AKS

1. **Create Azure Cache:**
   ```bash
   az redis create \
     --name bridge-redis \
     --resource-group bridge-rg \
     --location eastus \
     --sku Standard \
     --vm-size c3
   ```

2. **Deploy Bridge to AKS** with `VALKEY_URI` pointing to Azure Cache

## Production Best Practices

### Redis/Valkey Configuration

**Single Node (Small Deployments):**
```bash
VALKEY_URI="valkey://redis.internal:6379/0"
VALKEY_POOL_SIZE=50
VALKEY_MIN_IDLE_CONNS=10
```

**Cluster (High Availability):**
```bash
VALKEY_URI="valkey://node1:6379,node2:6379,node3:6379,node4:6379,node5:6379,node6:6379"
VALKEY_POOL_SIZE=100
VALKEY_MIN_IDLE_CONNS=20
```

### Load Balancer Configuration

**Health Checks:**
- **Path:** `/ready` (port 9103)
- **Interval:** 10 seconds
- **Timeout:** 5 seconds
- **Healthy threshold:** 2
- **Unhealthy threshold:** 3

**Session Affinity:**
- Not required for Bridge v3 (stateless design)
- Can use round-robin or least-connections

### Monitoring & Alerting

Set up alerts for:
- `bridge_ready_status == 0` - Service not ready
- `number_of_active_connections > threshold` - High load
- Redis connection errors
- High error rates

See [MONITORING.md](MONITORING.md) for complete metrics reference.

### Scaling Guidelines

| Concurrent Connections | Bridge Instances | Redis Setup | Instance Size |
|------------------------|------------------|-------------|---------------|
| < 1,000 | 1-2 | Single node | Small (2 CPU, 4GB) |
| 1,000 - 10,000 | 2-3 | Single node | Medium (4 CPU, 8GB) |
| 10,000 - 100,000 | 3-5 | Cluster (3 nodes) | Large (8 CPU, 16GB) |
| 100,000+ | 5-10+ | Cluster (6+ nodes) | XLarge (16 CPU, 32GB) |

### Security Checklist

- [ ] Enable TLS for Redis connections
- [ ] Use strong Redis password/ACLs
- [ ] Configure `TRUSTED_PROXY_RANGES` correctly
- [ ] Enable rate limiting (`RPS_LIMIT`, `CONNECTIONS_LIMIT`)
- [ ] Use bypass tokens for trusted services (`RATE_LIMITS_BY_PASS_TOKEN`)
- [ ] Keep Bridge v3 updated
- [ ] Monitor security advisories

## Troubleshooting

### Common Issues

**Bridge instances not receiving messages:**
- Verify Redis pub/sub is working: `MONITOR` command
- Check all instances connect to same Redis cluster
- Ensure firewall allows Redis traffic

**High latency:**
- Check Redis network latency
- Verify connection pool settings
- Monitor `number_of_active_connections` metric

**Connection drops:**
- Check load balancer timeout settings
- Verify `HEARTBEAT_INTERVAL` is appropriate
- Monitor `/ready` endpoint on all instances

See [KNOWN_ISSUES.md](KNOWN_ISSUES.md) for more troubleshooting tips.

## Migration from Bridge v1

If migrating from Bridge v1 to Bridge v3:

1. **Set up Redis cluster** - Deploy Redis/Valkey with appropriate sizing
2. **Deploy Bridge v3** - Start with 2-3 instances behind load balancer
3. **Test thoroughly** - Run parallel deployments to verify functionality
4. **Gradual migration** - Use DNS or load balancer to shift traffic
5. **Monitor closely** - Watch metrics during migration
6. **Decommission v1** - After successful migration and monitoring period

For more examples, see the [`docker/`](../docker/) folder.

