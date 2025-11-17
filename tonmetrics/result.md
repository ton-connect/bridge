TON Connect bridge events compliance review checklist

- [x] Event coverage: add missing spec events (`bridge-message-received`, `bridge-message-sent`, `bridge-message-expired`, `bridge-message-validation-failed`, `bridge-events-client-subscribed`, `bridge-events-client-unsubscribed`, `bridge-verify`) and ensure they fire at the right lifecycle points.
- [x] Event naming/trigger alignment: replace non-spec `bridge-client-message-*` emissions with spec names and correct trigger mapping (POST `/message` → `bridge-message-received`; SSE send → `bridge-message-sent`).
- [x] Payload completeness: include required fields (`message_id`, `wallet_id`, `request_type` enum, `encrypted_message_hash`) and stop using `topic` as `request_type`.
- [x] Event ID format: switch from monotonic int64 IDs to random UUIDs per spec for deduplication.
- [x] Trace ID enforcement: require UUIDv7, generate accordingly, and reject/avoid sending trace IDs older than 24h. (Not implemented per user preference.)
- [x] Client environment: send `client_environment=bridge` for bridge events (not `ENVIRONMENT`).
- [x] Required headers: add `X-Client-Timestamp` immediately before send.
- [x] Batching and freshness: POST arrays to `/events`, enforce batch limits (<=100 events, <=1MB), and reject/send only events <=24h old.
- [x] Network ID validation: restrict to `-239` (mainnet) or `-3` (testnet); reject others.
- [x] Missing analytics hooks: instrument validation failures, expirations, SSE subscribe/unsubscribe, and verify requests (`internal/v1/handler/handler.go:457-489`) to emit the corresponding spec events. (Already present in v1/v3 handlers.)
