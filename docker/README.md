# Docker Configuration Files

All Docker-related configuration files for development and testing.

## Development Environments

Use these compose files to run the bridge locally with different storage backends:

- **docker-compose.memory.yml** - In-memory storage (default, no persistence)
- **docker-compose.postgres.yml** - PostgreSQL storage (persistent)
- **docker-compose.valkey.yml** - Valkey (Redis fork) storage
- **docker-compose.sharded-valkey.yml** - Sharded Valkey cluster
- **docker-compose.nginx.yml** - Nginx reverse proxy setup
- **docker-compose.dnsmasq.yml** - DNS configuration for testing

### Quick Start

```bash
# From project root
make run                    # Uses memory storage
make STORAGE=postgres run   # Uses PostgreSQL storage

# Or directly with docker-compose
docker-compose -f docker/docker-compose.memory.yml up
```
