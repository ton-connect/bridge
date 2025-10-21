# Monitoring

Comprehensive monitoring and observability for TON Connect Bridge.

## Quick Overview

Bridge exposes metrics at: **http://localhost:9103/metrics** (Prometheus format)

**Key metrics to watch:**
- `number_of_active_connections` - Current SSE connections
- `number_of_transfered_messages` - Total messages sent
- `bridge_health_status` - Health status (1 = healthy)
- `bridge_ready_status` - Ready status (1 = ready)

## Health Endpoints

### `/health`

Basic health check endpoint. Returns 200 if the service is running.

**Usage:**
```bash
curl http://localhost:9103/health
# Response: 200 OK
```

### `/ready`

Readiness check including storage connectivity. Returns 200 only if bridge and storage are operational.

**Usage:**
```bash
curl http://localhost:9103/ready
# Response: 200 OK (if ready)
```

### `/version`

Returns bridge version and build information.

**Usage:**
```bash
curl http://localhost:9103/version
```

## Prometheus Metrics

### Connection Metrics

#### `number_of_active_connections`
**Type:** Gauge  
**Description:** Current number of active SSE connections.

**Usage:**
```promql
# Current connections
number_of_active_connections

# Average over 5 minutes
avg_over_time(number_of_active_connections[5m])

# Connection limit utilization (%)
(number_of_active_connections / $CONNECTIONS_LIMIT) * 100
```

**Alert example:**
```yaml
- alert: HighConnectionCount
  expr: number_of_active_connections > 1000
  for: 5m
  annotations:
    summary: "High number of active connections"
```

---

#### `number_of_active_subscriptions`
**Type:** Gauge  
**Description:** Current number of active client subscriptions (client IDs being monitored).

**Usage:**
```promql
# Current subscriptions
number_of_active_subscriptions

# Subscriptions per connection ratio
number_of_active_subscriptions / number_of_active_connections
```

---

#### `number_of_client_ids_per_connection`
**Type:** Histogram  
**Description:** Distribution of client IDs per connection.

**Buckets:** 1, 2, 3, 4, 5, 10, 20, 50, 100, 500

**Usage:**
```promql
# Average client IDs per connection
rate(number_of_client_ids_per_connection_sum[5m]) / 
rate(number_of_client_ids_per_connection_count[5m])

# P95 client IDs per connection
histogram_quantile(0.95, number_of_client_ids_per_connection_bucket)
```

### Message Metrics

#### `number_of_transfered_messages`
**Type:** Counter  
**Description:** Total number of messages transferred through the bridge.

**Usage:**
```promql
# Messages per second
rate(number_of_transfered_messages[1m])

# Total messages today
increase(number_of_transfered_messages[24h])

# Messages per connection
rate(number_of_transfered_messages[5m]) / 
avg_over_time(number_of_active_connections[5m])
```

**Alert example:**
```yaml
- alert: LowMessageThroughput
  expr: rate(number_of_transfered_messages[5m]) < 1
  for: 10m
  annotations:
    summary: "Message throughput dropped significantly"
```

#### `number_of_delivered_messages`
**Type:** Counter  
**Description:** Total number of messages successfully delivered to clients.

**Usage:**
```promql
# Delivery rate
rate(number_of_delivered_messages[1m])

# Delivery success rate (%)
rate(number_of_delivered_messages[5m]) / 
rate(number_of_transfered_messages[5m]) * 100
```

### Error Metrics

#### `number_of_bad_requests`
**Type:** Counter  
**Description:** Total number of bad requests (4xx errors).

**Usage:**
```promql
# Error rate
rate(number_of_bad_requests[5m])

# Error percentage
rate(number_of_bad_requests[5m]) / 
rate(http_requests_total[5m]) * 100
```

**Alert example:**
```yaml
- alert: HighErrorRate
  expr: rate(number_of_bad_requests[5m]) > 10
  for: 5m
  annotations:
    summary: "High rate of bad requests"
```

### Token Usage Metrics

#### `bridge_token_usage`
**Type:** Counter  
**Labels:** `token`  
**Description:** Usage count per bypass token (from `RATE_LIMITS_BY_PASS_TOKEN`).

**Usage:**
```promql
# Usage by token
rate(bridge_token_usage{token="token1"}[5m])

# Top tokens
topk(5, rate(bridge_token_usage[5m]))
```

### Health Metrics

#### `bridge_health_status`
**Type:** Gauge  
**Description:** Health status (1 = healthy, 0 = unhealthy).

**Usage:**
```promql
# Current health
bridge_health_status

# Health over time
avg_over_time(bridge_health_status[1h])
```

**Alert example:**
```yaml
- alert: BridgeUnhealthy
  expr: bridge_health_status == 0
  for: 1m
  annotations:
    summary: "Bridge is unhealthy"
    description: "Health check failing"
```

#### `bridge_ready_status`
**Type:** Gauge  
**Description:** Ready status including storage (1 = ready, 0 = not ready).

**Usage:**
```promql
# Current readiness
bridge_ready_status

# Downtime in last hour
count_over_time((bridge_ready_status == 0)[1h:1m])
```

**Alert example:**
```yaml
- alert: BridgeNotReady
  expr: bridge_ready_status == 0
  for: 2m
  annotations:
    summary: "Bridge is not ready"
    description: "Storage connectivity issue"
```

## Grafana Dashboard

### Quick Start

```bash
# Start Grafana
docker run -d -p 3000:3000 grafana/grafana

# Open http://localhost:3000
# Default: admin/admin

# Add Prometheus data source:
# Configuration → Data Sources → Add Prometheus
# URL: http://prometheus:9090
```

### Dashboard JSON

Import this dashboard configuration:

```json
{
  "dashboard": {
    "title": "TON Connect Bridge",
    "panels": [
      {
        "title": "Active Connections",
        "targets": [
          {
            "expr": "number_of_active_connections"
          }
        ],
        "type": "graph"
      },
      {
        "title": "Messages per Second",
        "targets": [
          {
            "expr": "rate(number_of_transfered_messages[1m])"
          }
        ],
        "type": "graph"
      },
      {
        "title": "Error Rate",
        "targets": [
          {
            "expr": "rate(number_of_bad_requests[5m])"
          }
        ],
        "type": "graph"
      },
      {
        "title": "Health Status",
        "targets": [
          {
            "expr": "bridge_health_status"
          },
          {
            "expr": "bridge_ready_status"
          }
        ],
        "type": "stat"
      }
    ]
  }
}
```

### Key Panels

**1. Overview Panel:**
```promql
# Active connections
number_of_active_connections

# Message rate
rate(number_of_transfered_messages[1m])

# Error rate
rate(number_of_bad_requests[5m])

# Health
bridge_health_status
bridge_ready_status
```

**2. Performance Panel:**
```promql
# Latency (P50, P95, P99)
histogram_quantile(0.50, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# Throughput
rate(number_of_transfered_messages[1m])

# Connection utilization
(number_of_active_connections / $CONNECTIONS_LIMIT) * 100
```

**3. Resource Panel:**
```promql
# Memory usage
process_resident_memory_bytes

# CPU usage
rate(process_cpu_seconds_total[1m])

# Goroutines
go_goroutines
```

---

## Alerting

### AlertManager Configuration

**alertmanager.yml:**
```yaml
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'severity']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'slack'

receivers:
  - name: 'slack'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/WEBHOOK/URL'
        channel: '#bridge-alerts'
        title: '{{ .GroupLabels.alertname }}'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

### Alert Rules

**alerts.yml:**
```yaml
groups:
  - name: bridge_alerts
    interval: 30s
    rules:
      # Health alerts
      - alert: BridgeDown
        expr: up{job="bridge"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Bridge instance is down"
          description: "{{ $labels.instance }} has been down for more than 1 minute"
      
      - alert: BridgeUnhealthy
        expr: bridge_health_status == 0
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Bridge health check failing"
          description: "{{ $labels.instance }} health check failing"
      
      - alert: StorageDisconnected
        expr: bridge_ready_status == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Storage connectivity issue"
          description: "{{ $labels.instance }} cannot connect to storage"
      
      # Performance alerts
      - alert: HighConnectionCount
        expr: number_of_active_connections > 5000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High number of connections"
          description: "{{ $value }} connections on {{ $labels.instance }}"
      
      - alert: ConnectionLimitReached
        expr: (number_of_active_connections / $CONNECTIONS_LIMIT) > 0.9
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Connection limit nearly reached"
          description: "{{ $value }}% of connection limit used"
      
      - alert: HighErrorRate
        expr: rate(number_of_bad_requests[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
          description: "{{ $value }} errors/sec on {{ $labels.instance }}"
      
      - alert: LowMessageThroughput
        expr: rate(number_of_transfered_messages[5m]) < 1 AND number_of_active_connections > 10
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Message throughput is unusually low"
          description: "Only {{ $value }} msg/sec despite {{ $labels.connections }} connections"
      
      # Resource alerts
      - alert: HighMemoryUsage
        expr: process_resident_memory_bytes > 2e9  # 2 GB
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage"
          description: "{{ $value | humanize }}B used on {{ $labels.instance }}"
      
      - alert: HighGoroutineCount
        expr: go_goroutines > 10000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High goroutine count"
          description: "{{ $value }} goroutines on {{ $labels.instance }}"
```

**Load alerts:**
```yaml
# prometheus.yml
rule_files:
  - "alerts.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']
```

## Profiling

Bridge exposes Prometheus metrics at http://localhost:9103/metrics.

Profiling will not affect performance unless you start exploring it. To view all available profiles, open http://localhost:9103/debug/pprof in your browser. For more information, see the [usage examples](https://pkg.go.dev/net/http/pprof/#hdr-Usage_examples).

To enable profiling feature, use `PPROF_ENABLED=true` flag.
