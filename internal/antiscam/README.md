# Antiscam Domain Filtering

Blocks scam domains from using the bridge. Push requests receive a fake success response so the client cannot distinguish filtering from normal delivery; SSE connections are closed immediately with `403 Forbidden`.

## How It Works

1. **Blocklist loading** — Fetches a newline-separated domain list from a remote URL on startup and refreshes it periodically in the background.
2. **Push dropping** — `SendMessageHandler` checks the `Origin` header. If blocked, returns `200 OK` without delivering the message.
3. **SSE rejection** — `EventRegistrationHandler` checks the `Origin` header. If blocked, returns `403 Forbidden` and closes the connection without creating a session or subscribing to storage.

## Subdomain Matching

The blocklist matches against the full domain hierarchy. Adding `evil.com` to the list also blocks `sub.evil.com`, `deep.sub.evil.com`, etc.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `ANTISCAM_ENABLED` | `false` | Enable antiscam filtering |
| `BLACK_LISTED_DOMAINS_URL` | _(empty)_ | URL to fetch the domain blocklist from |
| `BLACK_LIST_REFRESH_INTERVAL` | `600` | Blocklist refresh interval in seconds |

Both `ANTISCAM_ENABLED=true` and a non-empty `BLACK_LISTED_DOMAINS_URL` are required to activate filtering. Otherwise a no-op checker is used.

## Blocklist Format

Plain text, one domain per line. Empty lines and lines starting with `#` are ignored. Matching is case-insensitive.

```
# Scam domains
evil.com
scam.org
phishing.net
```

## Metrics

| Metric | Type | Description |
|---|---|---|
| `antiscam_blocked_pushes_total` | Counter | Push messages silently dropped |
| `antiscam_blocked_connections_total` | Counter | SSE connections rejected |
| `antiscam_blocklist_size` | Gauge | Current number of domains in the blocklist |
| `antiscam_blocklist_refresh_errors_total` | Counter | Blocklist refresh failures |
