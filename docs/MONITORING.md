# Monitoring

Comprehensive monitoring and observability for TON Connect Bridge.

## Quick Overview

Bridge exposes Prometheus metrics at http://localhost:9103/metrics.

**Key metrics to watch:**
- `number_of_active_connections` - Current SSE connections
- `number_of_transfered_messages` - Total messages sent
- `bridge_health_status` - Health status (1 = healthy)
- `bridge_ready_status` - Ready status (1 = ready)

## Profiling

Profiling will not affect performance unless you start exploring it. To view all available profiles, open http://localhost:9103/debug/pprof in your browser. For more information, see the [usage examples](https://pkg.go.dev/net/http/pprof/#hdr-Usage_examples).

To enable profiling feature, use `PPROF_ENABLED=true` flag.

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
