# bridge
[http bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) for tonconnect 2.0

**note:** for common issues and troubleshooting, see [KNOWN_ISSUES.md](KNOWN_ISSUES.md)

## requirements
- golang 1.24
- postgres

## how to  install

```
git clone https://github.com/ton-connect/bridge
make build
./bridge
```

## environments
- LOG_LEVEL ##example"info"
- PORT ##example"8081"
- POSTGRES_URI ##example"postgres://user:pass@host/dbname"
- WEBHOOK_URL ##example"https://your-webhook-url.com"
- COPY_TO_URL ##example"https://your-copy-url.com"
- CORS_ENABLE ##example"true"
- HEARTBEAT_INTERVAL (in seconds) ##example"10"
- RPS_LIMIT ##example"1"
- RATE_LIMITS_BY_PASS_TOKEN ##example"token1,token2"
- CONNECTIONS_LIMIT ##example"50"
- DISCONNECT_EVENTS_TTL (in seconds) ##example"3600"
- DISCONNECT_EVENT_MAX_SIZE (in bytes) ##example"512"
- CONNECT_CACHE_SIZE ##example"2000000" (maximum number of entries in connect client cache)
- CONNECT_CACHE_TTL ##example"300" (time-to-live for connect client cache entries in seconds)
- SELF_SIGNED_TLS ##example"false"
- TRUSTED_PROXY_RANGES ##example"10.0.0.0/8,172.16.0.0/12,192.168.0.0/16" (comma-separated list of IP ranges to trust for X-Forwarded-For header)
- PPROF_ENABLED ##examle"true"

## how to profile

Bridge exposes Prometheus metrics at http://localhost:9103/metrics.

Profiling will not affect performance unless you start exploring it. To view all available profiles, open http://localhost:9103/debug/pprof in your browser. For more information, see the [usage examples](https://pkg.go.dev/net/http/pprof/#hdr-Usage_examples).

To enable profiling feature, use `PPROF_ENABLED=true` flag.
