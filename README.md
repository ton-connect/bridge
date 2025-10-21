# Bridge

[HTTP bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) implementation for TON Connect 2.0.

## 📚 Documentation

Complete documentation is available in the [docs/](docs/) folder:

- **[Getting Started](docs/README.md)** - Quick start guide and overview
- **[Architecture & API](docs/ARCHITECTURE.md)** - Bridge v1 vs v3, storage backends, and HTTP API
- **[Configuration](docs/CONFIGURATION.md)** - Complete environment variables reference
- **[Deployment](docs/DEPLOYMENT.md)** - Production deployment best practices
- **[Monitoring](docs/MONITORING.md)** - Metrics, health checks, and observability

**Note:** For common issues and troubleshooting, see [KNOWN_ISSUES.md](KNOWN_ISSUES.md)

## 🚀 Quick Start

```bash
git clone https://github.com/ton-connect/bridge
cd bridge
make build      # Build Bridge v1
make build3     # Build Bridge v3
./bridge        # Run Bridge v1 (requires PostgreSQL)
./bridge3       # Run Bridge v3 (memory by default)
```

Use `make help` to see all available commands.

## 📋 Requirements

- Go 1.23+
- PostgreSQL (for Bridge v1 or Bridge v3 with PostgreSQL storage)
- Valkey/Redis (for Bridge v3 with Valkey storage)
- Node.js & npm (for SDK testing)

## ⚡ Choose Your Bridge

| Feature | Bridge v1 | Bridge v3 |
|---------|-----------|-----------|
| **Protocol** | HTTP Long Polling + SSE | Pub/Sub with SSE |
| **Storage** | PostgreSQL, Memory | Memory, Valkey, PostgreSQL* |
| **Use Case** | Production-ready, persistent | High-performance, real-time |
| **Maturity** | Stable | Stable |

\* PostgreSQL support for v3 is limited (no pub/sub yet)

See [Architecture docs](docs/ARCHITECTURE.md) for detailed comparison.

### Core Settings

```bash
LOG_LEVEL=info                               # Logging level
PORT=8081                                    # HTTP server port
METRICS_PORT=9103                            # Metrics port
```

### Storage (Bridge v3)

```bash
STORAGE=valkey                               # Storage backend: memory, valkey, postgres
VALKEY_URI="valkey://localhost:6379"         # Valkey connection string
```

### Storage (Bridge v1)

```bash
POSTGRES_URI="postgres://user:pass@host/db"  # PostgreSQL connection string
POSTGRES_MAX_CONNS=50                        # Connection pool size
```

### Performance

```bash
CORS_ENABLE=true                             # Enable CORS
HEARTBEAT_INTERVAL=10                        # Heartbeat interval (seconds)
RPS_LIMIT=100                                # Rate limit per second
CONNECTIONS_LIMIT=500                        # Max concurrent connections per IP
```

See [Configuration docs](docs/CONFIGURATION.md) for complete reference.

## 📊 Monitoring

Bridge exposes Prometheus metrics at `http://localhost:9103/metrics`.

**Key endpoints:**
- `/health` - Health check
- `/ready` - Readiness check (includes storage connectivity)
- `/metrics` - Prometheus metrics
- `/debug/pprof/*` - Go profiling (if `PPROF_ENABLED=true`)

See [Monitoring docs](docs/MONITORING.md) for complete setup.

## 🐳 Docker Quick Start

**Bridge v1 with PostgreSQL:**
```bash
make STORAGE=postgres run
```

**Bridge v3 with memory (development):**
```bash
docker-compose -f docker/docker-compose.memory.yml up
```

**Bridge v3 with Valkey (production):**
```bash
docker-compose -f docker/docker-compose.valkey.yml up
```

## 🔌 Using Bridge in Your Project

### Direct Usage

Default URL: `http://localhost:8081/bridge`

```bash
# Subscribe to events
curl -N "http://localhost:8081/bridge/events?client_id=your-client-id"

# Send message
curl -X POST "http://localhost:8081/bridge/message?client_id=sender&to=recipient&ttl=300" \
  -H "Content-Type: application/json" \
  -d '{"message":"base64_encoded_message"}'
```

See [API docs](docs/ARCHITECTURE.md#api-reference) for complete reference.

### GitHub Action

Use Bridge in your CI/CD pipeline:

```yaml
name: E2E Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Start Bridge
        uses: ton-connect/bridge/actions/local@master
        with:
          repository: "ton-connect/bridge"
          branch: "master"

      - name: Run E2E tests
        env:
          BRIDGE_URL: http://localhost:8081/bridge
        run: npm run e2e
```

### TypeScript SDK

```typescript
import { BridgeGateway } from '@tonconnect/bridge-sdk';

const gateway = new BridgeGateway({
  bridgeUrl: 'http://localhost:8081/bridge',
  clientId: 'my-client-id'
});

gateway.on('message', (message) => {
  console.log('Received:', message);
});

await gateway.send({
  to: 'recipient-id',
  message: 'Hello!',
  ttl: 300
});
```

See [bridge-sdk/](bridge-sdk/) for details.

## 🏗️ Development

```bash
# Build
make build          # Bridge v1
make build3         # Bridge v3

# Test
make test           # Unit tests
make test-gointegration  # Integration tests

# Format & Lint
make fmt            # Format code
make lint           # Run linter

# Run with different storage
make run                    # Memory storage (default)
make STORAGE=postgres run   # PostgreSQL storage
```

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📄 License

See [LICENSE](LICENSE) file for details.

## 🔗 Resources

- [TON Connect Documentation](https://github.com/ton-connect/docs)
- [Bridge SDK](bridge-sdk/)
- [Docker Setup](docker/)
- [Test Suite](test/)

---

Made with ❤️ for the TON ecosystem
