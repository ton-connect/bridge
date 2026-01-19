# bridge benchmarks

This directory contains performance testing tools for the Ton Connect Bridge Server using K6 with SSE support.

## 🚀 Quick Start

1. **Install K6 with SSE extension:**
   ```bash
   go install go.k6.io/xk6/cmd/xk6@latest
   xk6 build --with github.com/phymbert/xk6-sse@latest
   ```

2. **Start the bridge server and storage:**
   ```bash
   make build
   docker-compose -f docker/docker-compose.cluster-valkey.yml up -d
   STORAGE=valkey VALKEY_URI=redis://127.0.0.1:6379 \
   PORT=8081 CORS_ENABLE=true RPS_LIMIT=100 CONNECTIONS_LIMIT=8000 \
   LOG_LEVEL=error ./bridge
   ```

> **Tip: Bypass Bridge Rate Limits in Local/Test**

If your bridge server is configured with request rate limiting (e.g. `RPS_LIMIT`), you can bypass these limits for benchmark and development purposes by setting the environment variable `RATE_LIMITS_BY_PASS_TOKEN` to a known token, such as `test-token`.  
The benchmark script will use this token via its `Authorization: Bearer` header, so you should make sure your bridge instantiation command includes this variable:

```bash
RATE_LIMITS_BY_PASS_TOKEN=test-token \
STORAGE=valkey VALKEY_URI=redis://127.0.0.1:6379 \
PORT=8081 CORS_ENABLE=true RPS_LIMIT=100 CONNECTIONS_LIMIT=8000 \
LOG_LEVEL=error ./bridge
```

This instructs the server to exempt any request with `Authorization: Bearer test-token` from rate limits. **Do not use this configuration in production environments.**  
By default, the `bridge_test.js` load script will send this token (unless overridden via the `AUTH_TOKEN` environment variable).

3. **Flush Redis data (important for clean test results):**
   
   Before running benchmarks, flush Redis data locally to avoid collisions with previous test data:
   ```bash
   redis-cli FLUSHALL
   ```
   
   Or if using a specific Redis database:
   ```bash
   redis-cli -n <database_number> FLUSHDB
   ```
   
   **Note**: This ensures that test results are not affected by stale data from previous test runs, providing accurate and reproducible benchmark results.

4. **Run benchmark:**
   ```bash
   cd benchmark
   ./k6 run bridge_test.js
   ```

## 📋 Test Overview

The `bridge_test.js` script performs end-to-end load testing of the Ton Connect Bridge Server by simulating:
- **SSE Listeners**: Virtual users that maintain persistent SSE connections to receive messages
- **Message Senders**: Virtual users that send messages through the bridge

The test measures message delivery latency, error rates, and system stability under load.

## 🏗️ Test Architecture

The test runs two parallel scenarios:

### 1. SSE Workers Scenario (`sse`)
- Maintains persistent SSE connections to `/events` endpoint
- Listens for incoming messages and measures delivery latency
- Supports multiple client IDs per VU (configurable via `LISTENER_WRITERS_RATIO`)
- Automatically reconnects on connection failures

### 2. Message Senders Scenario (`senders`)
- Sends POST requests to `/message` endpoint
- Uses ramping arrival rate executor to control message sending rate
- Generates random client IDs within the configured ID space
- Sends messages with timestamps for latency measurement

## ⚙️ Configuration Options

All configuration is done via environment variables:

- `AUTH_TOKEN` - Bearer token used to authenticate requests to the bridge server and bypass any rate limiting. For local development or testing, you can use a fixed string such as `test-token`; for production-like testing, set this to a valid token if your bridge enforces authentication.

### Load Configuration
- `SSE_VUS` (default: `100`) - Number of SSE listener virtual users
- `SEND_RATE` (default: `1000`) - Target messages per second to send
- `LISTENER_WRITERS_RATIO` (default: `3`) - Number of listeners per writer (affects ID space calculation)
- `TOTAL_INSTANCES` (default: `1`) - Total number of test instances (for distributed testing)
- `CURRENT_INSTANCE` (default: `0`) - Current instance index (0-based, for distributed testing)

### Timing Configuration
All duration values support `s` (seconds), `m` (minutes), or `h` (hours) suffixes.

**SSE Scenario:**
- `SSE_RAMP_UP` (default: `10s`) - Time to ramp up SSE connections
- `SSE_HOLD` (default: `50s`) - Duration to maintain steady SSE connections
- `SSE_RAMP_DOWN` (default: `10s`) - Time to ramp down SSE connections
- `SSE_DELAY` (default: `0s`) - Delay before starting SSE scenario

**Sender Scenario:**
- `SENDER_RAMP_UP` (default: `10s`) - Time to ramp up message sending rate
- `SENDER_HOLD` (default: `30s`) - Duration to maintain steady sending rate
- `SENDER_RAMP_DOWN` (default: `10s`) - Time to ramp down message sending rate
- `SENDER_DELAY` (default: `10s`) - Delay before starting sender scenario (typically set to allow SSE connections to establish)

### Server Configuration
- `BRIDGE_URL` (default: `http://localhost:8081/bridge`) - Base URL of the bridge server

## 📊 Metrics Collected

The test tracks the following custom metrics:

- **`sse_message_received`** - Counter of messages received via SSE
- **`sse_message_sent`** - Counter of messages successfully sent
- **`sse_errors`** - Counter of SSE connection/communication errors
- **`post_errors`** - Counter of failed POST requests
- **`delivery_latency`** - Trend metric tracking end-to-end message delivery latency (milliseconds)
- **`json_parse_errors`** - Counter of JSON parsing errors
- **`missing_timestamps`** - Counter of messages received without timestamps

Standard k6 metrics are also collected (HTTP request duration, status codes, etc.).

## 🎯 Test Thresholds

The test includes the following pass/fail thresholds:

- `http_req_failed < 0.01%` - HTTP request failure rate must be below 0.01%
- `delivery_latency p(95) < 2000ms` - 95th percentile latency must be below 2 seconds
- `sse_errors < 10` - Total SSE errors must be below 10
- `json_parse_errors < 5` - Total JSON parse errors must be below 5
- `missing_timestamps < 100` - Messages without timestamps must be below 100
- `sse_message_sent > 5` - At least 5 messages must be sent
- `sse_message_received > 5` - At least 5 messages must be received

## 🚀 Usage Examples

### Basic Run
```bash
./k6 run bridge_test.js
```

### Custom Load Configuration
```bash
SSE_VUS=2000 SEND_RATE=20 ./k6 run bridge_test.js
```

### Extended Test Duration
```bash
SSE_RAMP_UP=30s SSE_HOLD=5m SSE_RAMP_DOWN=30s \
SENDER_RAMP_UP=30s SENDER_HOLD=5m SENDER_RAMP_DOWN=30s \
SENDER_DELAY=30s ./k6 run bridge_test.js
```

### High Load Test
```bash
SSE_VUS=5000 SEND_RATE=50 \
SSE_RAMP_UP=10m SSE_HOLD=30m SSE_RAMP_DOWN=10m \
SENDER_RAMP_UP=10m SENDER_HOLD=30m SENDER_RAMP_DOWN=10m \
./k6 run bridge_test.js
```

### Distributed Testing (Multiple Instances)
On instance 0:
```bash
TOTAL_INSTANCES=3 CURRENT_INSTANCE=0 ./k6 run bridge_test.js
```

On instance 1:
```bash
TOTAL_INSTANCES=3 CURRENT_INSTANCE=1 ./k6 run bridge_test.js
```

On instance 2:
```bash
TOTAL_INSTANCES=3 CURRENT_INSTANCE=2 ./k6 run bridge_test.js
```

### Custom Bridge URL
```bash
BRIDGE_URL=http://bridge.example.com:8081/bridge ./k6 run bridge_test.js
```

## 📈 Understanding Results

### Preparation for Accurate Results

**Important**: Always flush Redis data before running tests to ensure clean results:
```bash
redis-cli FLUSHALL
```

Local valkey cluster cleanup
```bash
docker exec -i docker-valkey-shard1-1 redis-cli -p 6379 FLUSHALL
docker exec -i docker-valkey-shard2-1 redis-cli -p 6380 FLUSHALL
docker exec -i docker-valkey-shard3-1 redis-cli -p 6381 FLUSHALL
```

This prevents:
- Stale messages from previous test runs affecting metrics
- Client ID collisions with previous test data
- Inaccurate latency measurements due to cached data
- Message delivery inconsistencies

### Key Metrics to Monitor

1. **Delivery Latency (`delivery_latency`)**
   - Measures time from message creation to delivery via SSE
   - Lower is better; p(95) should be < 2000ms

2. **Message Throughput**
   - Compare `sse_message_sent` vs `sse_message_received`
   - Should be approximately equal (accounting for timing differences)

3. **Error Rates**
   - Monitor `sse_errors` and `post_errors`
   - Should remain low (< 1% failure rate)

4. **Connection Stability**
   - Low `sse_errors` indicates stable SSE connections
   - High error count suggests connection issues or server overload

### Common Issues

- **High latency**: Server may be overloaded, reduce `SEND_RATE` or increase server resources
- **SSE connection errors**: Check `CONNECTIONS_LIMIT` on bridge server, may need to increase
- **Message delivery failures**: Verify `sse_message_sent` vs `sse_message_received` ratio
- **JSON parse errors**: May indicate malformed messages or encoding issues

## 🔧 ID Space Management

The test generates client IDs within a calculated ID space to ensure:
- Listeners exist for messages being sent
- No collisions between test instances in distributed setups
- Predictable ID distribution

The ID space is calculated as:
```
ID_SPACE_SIZE = TOTAL_INSTANCES × LISTENER_WRITERS_RATIO × SSE_VUS - 1
```

Each instance uses a different offset:
```
START_INDEX_OFFSET = CURRENT_INSTANCE × LISTENER_WRITERS_RATIO × SSE_VUS
```

Happy benchmarking! 🚀
