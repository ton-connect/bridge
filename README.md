# bridge

[HTTP bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) implementation for TON Connect 2.0.

## 🚀Quick Start

```bash
git clone https://github.com/ton-connect/bridge3
cd bridge
make build
./bridge
```

Use `make help` to see all available commands and storage options.

**Note:** For common issues and troubleshooting, see [KNOWN_ISSUES.md](docs/KNOWN_ISSUES.md)

## 📋Requirements

- Go 1.24+
- PostgreSQL or Redis-compatible storage
- Node.js & npm (for testing)

## 📚 Documentation

- **[Architecture](docs/ARCHITECTURE.md)** - Bridge v1 vs v3, storage backends, and HTTP API
- **[Configuration](docs/CONFIGURATION.md)** - Complete environment variables reference
- **[Deployment](docs/DEPLOYMENT.md)** - Production deployment best practices
- **[Know issues](docs/KNOWN_ISSUES.md)** - Known issues
- **[Monitoring](docs/MONITORING.md)** - Metrics, health checks, and observability

## Use local TON Connect Bridge

Default url `http://localhost:8081/bridge`

### Docker

```bash
git clone https://github.com/ton-connect/bridge.git
cd bridge
docker compose -f docker/docker-compose.memory.yml up --build -d
curl -I -f -s -o /dev/null -w "%{http_code}\n" http://localhost:9103/metrics
```

### GitHub Action

Example usage from another repository:

```yaml
name: e2e
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
        run: |
          npm run e2e
```

Made with ❤️ for the TON ecosystem