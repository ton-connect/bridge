# Bridge Documentation

[HTTP bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) implementation for TON Connect 2.0.

## 📚 Documentation

- **[Architecture & API](ARCHITECTURE.md)** - Bridge v1 vs v3, storage backends, and HTTP API
- **[Configuration](CONFIGURATION.md)** - Complete environment variables reference
- **[Deployment](DEPLOYMENT.md)** - Production deployment best practices
- **[Monitoring](MONITORING.md)** - Metrics, health checks, and observability

## 🚀 Quick Start

```bash
git clone https://github.com/ton-connect/bridge
cd bridge
make build      # Build Bridge v1
make build3     # Build Bridge v3
./bridge        # Run Bridge v1 (requires PostgreSQL)
./bridge3       # Run Bridge v3 (memory by default)
```

Use `make help` to see all available commands and storage options.

**Note:** For common issues and troubleshooting, see [KNOWN_ISSUES.md](../KNOWN_ISSUES.md)

## 📋 Requirements

- Go 1.23+
- PostgreSQL (for Bridge v1 or Bridge v3 with PostgreSQL storage)
- Valkey/Redis (for Bridge v3 with Valkey storage)
- Node.js & npm (for SDK testing)

## ⚡ Choosing Your Bridge

| Feature | Bridge v1 | Bridge v3 |
|---------|-----------|-----------|
| **Protocol** | HTTP Long Polling + SSE | Pub/Sub with SSE |
| **Storage** | PostgreSQL, Memory | Memory, Valkey, PostgreSQL* |
| **Use Case** | Production-ready, persistent | High-performance, real-time |
| **Maturity** | Stable | Stable |

\* PostgreSQL support for v3 is limited (no pub/sub yet)

See [Architecture docs](ARCHITECTURE.md) for detailed comparison.

## 🐳 Quick Start with Docker

```bash
# Bridge v1 with PostgreSQL
make STORAGE=postgres run

# Bridge v3 with memory
docker-compose -f docker/docker-compose.memory.yml up

# Bridge v3 with Valkey
docker-compose -f docker/docker-compose.valkey.yml up
```

## 🔗 Resources

- [TON Connect Documentation](https://github.com/ton-connect/docs)
- [Bridge SDK](../bridge-sdk/)
- [GitHub Action](../actions/local/)

Made with ❤️ for the TON ecosystem
