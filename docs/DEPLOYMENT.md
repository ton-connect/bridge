# Deployment Guide

Production-ready deployment patterns and best practices for TON Connect Bridge.

## Quick Deployment Checklist

- [ ] Choose Bridge version (v1 or v3) based on requirements
- [ ] Select storage backend
- [ ] Configure environment variables
- [ ] Set up TLS termination
- [ ] Configure load balancer (if multi-instance)
- [ ] Enable monitoring and alerts
- [ ] Test health endpoints
- [ ] Set up rate limiting
- [ ] Configure backups (if using PostgreSQL)
- [ ] Document rollback procedure

## Architecture Patterns

### Pattern 1: Single Instance (Small Scale)

**Best for:** <1,000 concurrent connections, low traffic

```
Internet
   │
   ├── TLS Termination (Cloudflare/nginx)
   │
   ▼
┌─────────┐
│ Bridge  │──── PostgreSQL or Valkey
└─────────┘
```

**Pros:**
- Simple setup
- Easy to manage
- Low cost

**Cons:**
- Single point of failure
- Limited scaling
- No high availability

---

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
│B1 │  │B2 │  │B3 │  │B4 │ Bridge Instances
└───┘  └───┘  └───┘  └───┘
   │      │      │      │
   └──────┴──────┴──────┘
            │
            ▼
   ┌─────────────────┐
   │ Shared Storage  │ (PostgreSQL/Valkey Cluster)
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

---

### Pattern 3: Multi-Region (Global)

**Best for:** Global audience, ultra-low latency requirements

```
     Region 1              Region 2              Region 3
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│ Load Balancer│      │ Load Balancer│      │ Load Balancer│
└──────┬───────┘      └──────┬───────┘      └──────┬───────┘
       │                     │                     │
    ┌──┴──┐               ┌──┴──┐               ┌──┴──┐
    │ B v3│               │ B v3│               │ B v3│
    └──┬──┘               └──┬──┘               └──┬──┘
       │                     │                     │
       └─────────────────────┴─────────────────────┘
                            │
                     Valkey Cluster
                   (with replication)
```

**Requirements:**
- Bridge v3 with Valkey
- Multi-region Valkey cluster with replication
- Geographic load balancing (DNS/CDN)

---

## Deployment Methods

### Docker Compose (Recommended for Small Scale)

**Bridge v1 + PostgreSQL:**

```yaml
# docker-compose.yml
version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: bridge_db
      POSTGRES_USER: bridge
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U bridge"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  bridge:
    build: .
    command: ./bridge
    environment:
      LOG_LEVEL: info
      PORT: 8081
      METRICS_PORT: 9103
      POSTGRES_URI: postgres://bridge:${DB_PASSWORD}@postgres:5432/bridge_db
      POSTGRES_MAX_CONNS: 50
      CORS_ENABLE: "true"
      HEARTBEAT_INTERVAL: 10
      RPS_LIMIT: 100
      CONNECTIONS_LIMIT: 500
      TRUSTED_PROXY_RANGES: "172.16.0.0/12"
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "8081:8081"
      - "9103:9103"
    restart: unless-stopped

  nginx:
    image: nginx:alpine
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
      - ./certs:/etc/nginx/certs
    ports:
      - "443:443"
      - "80:80"
    depends_on:
      - bridge
    restart: unless-stopped

volumes:
  postgres_data:
```

**Bridge v3 + Valkey:**

```yaml
# docker-compose.yml
version: '3.8'

services:
  valkey:
    image: valkey/valkey:7-alpine
    command: valkey-server --appendonly yes --maxmemory 2gb --maxmemory-policy allkeys-lru
    volumes:
      - valkey_data:/data
    healthcheck:
      test: ["CMD", "valkey-cli", "ping"]
      interval: 10s
      timeout: 3s
      retries: 3
    restart: unless-stopped

  bridge3:
    build:
      context: .
      dockerfile: docker/Dockerfile.bridge3
    command: ./bridge3
    environment:
      LOG_LEVEL: info
      PORT: 8081
      METRICS_PORT: 9103
      STORAGE: valkey
      VALKEY_URI: valkey://valkey:6379/0
      CORS_ENABLE: "true"
      HEARTBEAT_INTERVAL: 10
      RPS_LIMIT: 1000
      CONNECTIONS_LIMIT: 2000
      CONNECT_CACHE_SIZE: 5000000
      TRUSTED_PROXY_RANGES: "172.16.0.0/12"
    depends_on:
      valkey:
        condition: service_healthy
    ports:
      - "8081:8081"
      - "9103:9103"
    restart: unless-stopped
    deploy:
      resources:
        limits:
          memory: 2G
        reservations:
          memory: 1G

  nginx:
    image: nginx:alpine
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
      - ./certs:/etc/nginx/certs
    ports:
      - "443:443"
      - "80:80"
    depends_on:
      - bridge3
    restart: unless-stopped

volumes:
  valkey_data:
```

**Deploy:**
```bash
# Set password
export DB_PASSWORD="your_strong_password_here"

# Start services
docker-compose up -d

# Check health
curl http://localhost:9103/health
curl http://localhost:9103/metrics

# View logs
docker-compose logs -f bridge
```

---

### Kubernetes (Recommended for Large Scale)

**Bridge v3 + Valkey Deployment:**

```yaml
# bridge-deployment.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: bridge-config
data:
  LOG_LEVEL: "info"
  STORAGE: "valkey"
  VALKEY_URI: "valkey://valkey-service:6379/0"
  CORS_ENABLE: "true"
  HEARTBEAT_INTERVAL: "10"
  RPS_LIMIT: "1000"
  CONNECTIONS_LIMIT: "5000"
  CONNECT_CACHE_SIZE: "10000000"

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bridge3
  labels:
    app: bridge3
spec:
  replicas: 3
  selector:
    matchLabels:
      app: bridge3
  template:
    metadata:
      labels:
        app: bridge3
    spec:
      containers:
      - name: bridge3
        image: tonconnect/bridge3:latest
        ports:
        - containerPort: 8081
          name: http
        - containerPort: 9103
          name: metrics
        envFrom:
        - configMapRef:
            name: bridge-config
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 9103
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 9103
          initialDelaySeconds: 5
          periodSeconds: 10

---
apiVersion: v1
kind: Service
metadata:
  name: bridge3-service
spec:
  selector:
    app: bridge3
  ports:
  - name: http
    port: 8081
    targetPort: 8081
  - name: metrics
    port: 9103
    targetPort: 9103
  type: ClusterIP

---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: bridge3-ingress
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - bridge.yourdomain.com
    secretName: bridge-tls
  rules:
  - host: bridge.yourdomain.com
    http:
      paths:
      - path: /bridge
        pathType: Prefix
        backend:
          service:
            name: bridge3-service
            port:
              number: 8081
      - path: /health
        pathType: Prefix
        backend:
          service:
            name: bridge3-service
            port:
              number: 9103

---
# Valkey StatefulSet
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: valkey
spec:
  serviceName: valkey-service
  replicas: 3
  selector:
    matchLabels:
      app: valkey
  template:
    metadata:
      labels:
        app: valkey
    spec:
      containers:
      - name: valkey
        image: valkey/valkey:7-alpine
        ports:
        - containerPort: 6379
          name: valkey
        volumeMounts:
        - name: valkey-data
          mountPath: /data
        resources:
          requests:
            memory: "2Gi"
            cpu: "500m"
          limits:
            memory: "4Gi"
            cpu: "2000m"
  volumeClaimTemplates:
  - metadata:
      name: valkey-data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi

---
apiVersion: v1
kind: Service
metadata:
  name: valkey-service
spec:
  selector:
    app: valkey
  ports:
  - port: 6379
    targetPort: 6379
  clusterIP: None
```

**Deploy:**
```bash
kubectl apply -f bridge-deployment.yaml

# Check status
kubectl get pods -l app=bridge3
kubectl logs -f deployment/bridge3

# Scale
kubectl scale deployment bridge3 --replicas=5
```

---

### Systemd Service (Bare Metal/VM)

**Bridge v3 systemd service:**

```ini
# /etc/systemd/system/bridge3.service
[Unit]
Description=TON Connect Bridge v3
After=network.target valkey.service
Requires=valkey.service

[Service]
Type=simple
User=bridge
Group=bridge
WorkingDirectory=/opt/bridge
Environment="LOG_LEVEL=info"
Environment="PORT=8081"
Environment="METRICS_PORT=9103"
Environment="STORAGE=valkey"
Environment="VALKEY_URI=valkey://localhost:6379/0"
Environment="CORS_ENABLE=true"
Environment="RPS_LIMIT=1000"
Environment="CONNECTIONS_LIMIT=2000"
ExecStart=/opt/bridge/bridge3
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=bridge3

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/bridge

# Limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

**Setup:**
```bash
# Create user
sudo useradd -r -s /bin/false bridge

# Install bridge
sudo mkdir -p /opt/bridge
sudo cp bridge3 /opt/bridge/
sudo chown -R bridge:bridge /opt/bridge

# Enable service
sudo systemctl daemon-reload
sudo systemctl enable bridge3
sudo systemctl start bridge3

# Check status
sudo systemctl status bridge3
sudo journalctl -u bridge3 -f
```

---

## TLS/SSL Configuration

### Option 1: Cloudflare (Recommended)

**Pros:** Free, automatic cert renewal, DDoS protection, CDN

1. Add your domain to Cloudflare
2. Set DNS record: `bridge.yourdomain.com → your_server_ip`
3. Enable "Full (strict)" SSL mode
4. Bridge runs on HTTP internally (Cloudflare handles TLS)

**No TLS configuration needed in bridge!**

---

### Option 2: Let's Encrypt + nginx

**nginx configuration:**

```nginx
# /etc/nginx/conf.d/bridge.conf
upstream bridge_backend {
    least_conn;
    server 127.0.0.1:8081 max_fails=3 fail_timeout=30s;
    # Add more instances if needed
    # server 127.0.0.1:8082 max_fails=3 fail_timeout=30s;
}

# HTTP → HTTPS redirect
server {
    listen 80;
    server_name bridge.yourdomain.com;
    
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }
    
    location / {
        return 301 https://$server_name$request_uri;
    }
}

# HTTPS
server {
    listen 443 ssl http2;
    server_name bridge.yourdomain.com;
    
    # SSL certificates (Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/bridge.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/bridge.yourdomain.com/privkey.pem;
    ssl_trusted_certificate /etc/letsencrypt/live/bridge.yourdomain.com/chain.pem;
    
    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    
    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "DENY" always;
    
    # Bridge endpoints
    location /bridge/events {
        proxy_pass http://bridge_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # SSE specific
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
    
    location /bridge/message {
        proxy_pass http://bridge_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Rate limiting
        limit_req zone=bridge_rate burst=20 nodelay;
    }
    
    # Metrics (restrict access!)
    location /metrics {
        proxy_pass http://127.0.0.1:9103;
        allow 10.0.0.0/8;    # Internal network
        allow 172.16.0.0/12;  # Docker network
        deny all;
    }
    
    location /health {
        proxy_pass http://127.0.0.1:9103;
        access_log off;
    }
}

# Rate limiting zone
limit_req_zone $binary_remote_addr zone=bridge_rate:10m rate=10r/s;
```

**Get certificate:**
```bash
# Install certbot
sudo apt install certbot python3-certbot-nginx

# Get certificate
sudo certbot --nginx -d bridge.yourdomain.com

# Auto-renewal
sudo systemctl enable certbot.timer
```

---

## Monitoring Setup

### Prometheus + Grafana

**prometheus.yml:**
```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'bridge'
    static_configs:
      - targets: ['localhost:9103']
        labels:
          instance: 'bridge-prod-1'
          version: 'v3'
```

**Grafana dashboard queries:**

```promql
# Active connections
number_of_active_connections

# Messages per second
rate(number_of_transfered_messages[1m])

# Error rate
rate(number_of_bad_requests[5m])

# Connection limit utilization
number_of_active_connections / $CONNECTIONS_LIMIT * 100
```

See [MONITORING.md](MONITORING.md) for complete setup.

---

## Security Best Practices

### 1. Network Security

- ✅ Use TLS termination (Cloudflare/nginx)
- ✅ Restrict metrics endpoint to internal network
- ✅ Configure `TRUSTED_PROXY_RANGES` correctly
- ✅ Use firewall rules (only 443, 80 public)
- ✅ Enable DDoS protection (Cloudflare)

### 2. Rate Limiting

```bash
# Per-IP rate limits
RPS_LIMIT=100

# Bypass tokens for trusted clients
RATE_LIMITS_BY_PASS_TOKEN="secret-token-1,secret-token-2"

# Connection limits per IP
CONNECTIONS_LIMIT=500
```

### 3. Resource Limits

**Docker:**
```yaml
deploy:
  resources:
    limits:
      memory: 2G
      cpus: '2'
```

**Systemd:**
```ini
LimitNOFILE=65536
LimitNPROC=4096
```

### 4. Secrets Management

**Never hardcode secrets!**

```bash
# Bad
POSTGRES_URI="postgres://user:password@host/db"

# Good - use environment or secrets manager
POSTGRES_URI="${DB_URI}"  # From secure storage

# Docker secrets
docker secret create db_uri postgres://...
```

---

## High Availability

### Health Checks

**Configure load balancer health checks:**

```bash
# Health endpoint
curl http://bridge:9103/health
# Returns 200 if healthy

# Readiness endpoint (includes storage check)
curl http://bridge:9103/ready
# Returns 200 if ready
```

**Load balancer config (nginx):**
```nginx
upstream bridge {
    server bridge1:8081 max_fails=3 fail_timeout=30s;
    server bridge2:8081 max_fails=3 fail_timeout=30s;
    
    # Health check
    check interval=10000 rise=2 fall=3 timeout=5000;
}
```

### Graceful Shutdown

Bridge handles SIGTERM for graceful shutdown:

```bash
# Gracefully stop
kill -TERM $(pidof bridge3)

# Kubernetes handles this automatically
kubectl rollout restart deployment/bridge3
```

---

## Backup & Disaster Recovery

### PostgreSQL Backups

```bash
# Automated daily backups
0 2 * * * pg_dump -h db -U bridge bridge_db | gzip > /backup/bridge_$(date +\%Y\%m\%d).sql.gz

# Retention: keep 30 days
find /backup -name "bridge_*.sql.gz" -mtime +30 -delete
```

### Valkey Backups (optional)

```bash
# RDB snapshot
valkey-cli BGSAVE

# AOF backup
cp /var/lib/valkey/appendonly.aof /backup/
```

### Disaster Recovery Plan

1. **Monitor alerts** → detect issue
2. **Assess impact** → check health/metrics
3. **Restore from backup** (PostgreSQL) or **restart** (Valkey)
4. **Scale horizontally** if needed
5. **Post-mortem** → document incident

---

## Performance Tuning

### OS-Level Tuning

```bash
# /etc/sysctl.conf
# Increase open file limits
fs.file-max = 2097152

# TCP tuning
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_tw_reuse = 1

# Apply
sudo sysctl -p
```

### Bridge Tuning

```bash
# High-performance Bridge v3 config
STORAGE=valkey
CONNECTIONS_LIMIT=10000
RPS_LIMIT=10000
HEARTBEAT_INTERVAL=15
CONNECT_CACHE_SIZE=10000000
CONNECT_CACHE_TTL=600
```

### Valkey Tuning

```conf
# valkey.conf
maxmemory 8gb
maxmemory-policy allkeys-lru
tcp-backlog 511
timeout 0
tcp-keepalive 300
```

---

## Deployment Checklist

### Pre-Deployment

- [ ] Environment variables configured
- [ ] TLS certificates obtained
- [ ] Firewall rules set
- [ ] Monitoring configured
- [ ] Backups scheduled (PostgreSQL)
- [ ] Load test completed
- [ ] Rollback plan documented

### Deployment

- [ ] Deploy to staging first
- [ ] Run smoke tests
- [ ] Deploy to production
- [ ] Verify health endpoints
- [ ] Check metrics dashboard
- [ ] Monitor logs for errors

### Post-Deployment

- [ ] Verify client connectivity
- [ ] Monitor metrics for anomalies
- [ ] Check error rates
- [ ] Review logs
- [ ] Document deployment

---

## Troubleshooting

### Bridge won't start

```bash
# Check logs
docker-compose logs bridge
journalctl -u bridge3 -n 50

# Common issues:
# - Invalid POSTGRES_URI or VALKEY_URI
# - Port already in use
# - Storage backend not reachable
```

### High memory usage

```bash
# Check connections
curl http://localhost:9103/metrics | grep active_connections

# Reduce cache size
CONNECT_CACHE_SIZE=1000000

# Check for memory leaks
curl http://localhost:9103/debug/pprof/heap
```

### Rate limiting not working

```bash
# Verify TRUSTED_PROXY_RANGES includes your load balancer
TRUSTED_PROXY_RANGES="10.0.0.0/8,172.16.0.0/12"

# Check X-Forwarded-For header is set
curl -H "X-Forwarded-For: 1.2.3.4" http://bridge:8081/bridge/message
```

---

## See Also

- [Architecture](ARCHITECTURE.md) - Choose v1 or v3
- [Storage](STORAGE.md) - Storage backend setup
- [Configuration](CONFIGURATION.md) - Environment variables
- [Monitoring](MONITORING.md) - Metrics and alerts
