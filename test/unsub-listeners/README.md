# Unsub Listeners Bug Test

> **Note**: These tests are not running in CI.

## Bug

When multiple SSE connections are open for the same `client_id`, closing one connection unsubscribes ALL listeners. This causes remaining open connections to stop receiving messages.

**Expected:** Only the closed connection should be unsubscribed. Other connections for the same `client_id` should continue receiving messages.

## Reproduction Steps

1. Open 2 SSE connections for the same client_id:

```
# Terminal 1curl -s "http://localhost:8081/bridge/events?client_id=0000000000000000000000000000000000000000000000000000000000000001"# Terminal 2  curl -s "http://localhost:8081/bridge/events?client_id=0000000000000000000000000000000000000000000000000000000000000001"
```

2. Send a message:

```
curl -X POST "http://localhost:8081/bridge/message?client_id=sender&to=0000000000000000000000000000000000000000000000000000000000000001&ttl=300&topic=test" -d "message $(date +%s)"
```

Both terminals receive the message

3. Close Terminal 1 (triggers Unsub)
4. Send another message:

```
curl -X POST "http://localhost:8081/bridge/message?client_id=sender&to=0000000000000000000000000000000000000000000000000000000000000001&ttl=300&topic=test" -d "message $(date +%s)"
```

How it was before: Terminal 2 does NOT receive the message.
How is it now: Terminal 2 receives the message.

## How to Run Test

**Prerequisites:** Docker, Docker Compose, ports 8081 and 16379 available.

1. Start the services:
```bash
cd test/unsub-listeners
docker-compose up -d
```

2. Wait for Bridge to be ready (check logs or visit http://localhost:8081/health)

3. Run the test:
```bash
./run-test.sh
```

4. Clean up when done:
```bash
docker-compose down -v
```
