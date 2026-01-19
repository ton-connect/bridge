# TON Connect Bridge

[HTTP bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) implementation for TON Connect 2.0.

## 🚀 Quick Start

```bash
git clone https://github.com/ton-connect/bridge
cd bridge
make build3
./bridge3
```

For production deployments, use Redis/Valkey storage. See [Configuration](docs/CONFIGURATION.md) for details.

Use `make help` to see all available commands.

**Note:** For common issues and troubleshooting, see [KNOWN_ISSUES.md](docs/KNOWN_ISSUES.md)

## 📋 Requirements

- Go 1.24+
- Redis/Valkey 7.0+ (or any Redis-compatible storage) for production
- Node.js & npm (for testing)

## 📚 Documentation

- [Architecture](docs/ARCHITECTURE.md) - Bridge architecture, pub/sub design, and scaling
- [Configuration](docs/CONFIGURATION.md) - Complete environment variables reference
- [Deployment](docs/DEPLOYMENT.md) - Production deployment patterns and best practices
- [Known Issues](docs/KNOWN_ISSUES.md) - Common issues and troubleshooting
- [Monitoring](docs/MONITORING.md) - Metrics, health checks, and observability

<details>
<summary>Looking for PostgreSQL-based setup?</summary>

> For existing PostgreSQL-based installations, see [Bridge v1](cmd/bridge/README.md).
>
> We recommend migrating to the current version — it scales with your user base and receives active development.
>
> The current setup uses Redis/Valkey for pub/sub, enabling horizontal scaling across multiple instances. The PostgreSQL-based version is limited to a single instance and cannot scale horizontally.

</details>

## Use local TON Connect Bridge

Default url: `http://localhost:8081/bridge`

### Docker

```bash
git clone https://github.com/ton-connect/bridge.git
cd bridge
docker compose -f docker/docker-compose.valkey.yml up --build -d
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