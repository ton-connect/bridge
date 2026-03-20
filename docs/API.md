# API Reference

This bridge implements the [TON Connect HTTP Bridge Specification](https://github.com/ton-blockchain/ton-connect/blob/main/bridge.md).

## Bridge Endpoints

**Port:** `8081` (default, configurable via `PORT`)

- `POST /bridge/message` - Send a message to a client (optional `wallet` param triggers a signed [webhook](WEBHOOKS.md))
- `GET /bridge/events` - Subscribe to SSE stream for real-time messages
- `GET /bridge/webhook/public-key` - Ed25519 public key for [webhook verification](WEBHOOKS.md)

## Health & Monitoring Endpoints

**Port:** `9103` (default, configurable via `METRICS_PORT`)

- `GET /health` - Basic health check
- `GET /ready` - Readiness check (includes storage connectivity)
- `GET /version` - Bridge version and build information
- `GET /metrics` - Prometheus metrics endpoint
