# bridge
[http bridge](https://github.com/ton-connect/docs/blob/main/bridge.md) for tonconnect 2.0

**note:** for common issues and troubleshooting, see [KNOWN_ISSUES.md](KNOWN_ISSUES.md)

## requirements
- golang 1.18
- postgres

## how to  install
- git clone https://github.com/ton-connect/bridge
- cd bridge
- go build ./ 
- go run bridge

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
- DISCONNECT_EVENT_MAX_SIZE (in bytes) ##example"1024"
- SELF_SIGNED_TLS ##example"false"
