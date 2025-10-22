# bridge benchmarks

This directory contains performance testing tools for the Ton Connect Bridge Server using K6 with SSE support.

## 🚀 Quick Start

1. **Install K6 with SSE extension:**
   ```bash
   go install go.k6.io/xk6/cmd/xk6@latest
   xk6 build --with github.com/phymbert/xk6-sse@latest
   ```

2. **Start the bridge server:**
   ```bash
   make build
   POSTGRES_URI="postgres://bridge_user:bridge_password@localhost:5432/bridge?sslmode=disable" \
   PORT=8081 CORS_ENABLE=true RPS_LIMIT=100 CONNECTIONS_LIMIT=8000 \
   LOG_LEVEL=error ./bridge
   ```

3. **Run benchmark:**
   ```bash
   cd benchmark
   ./k6 run bridge_test.js
   ```


Happy benchmarking! 🚀
