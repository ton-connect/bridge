# Unsub Listeners Bug Test

## Bug

When multiple SSE connections are open for the same `client_id`, closing one connection unsubscribes ALL listeners. This causes remaining open connections to stop receiving messages.

**Expected:** Only the closed connection should be unsubscribed. Other connections for the same `client_id` should continue receiving messages.

## Reproduction Steps

1. Open 2 SSE connections for the same `client_id`
2. Send a message → both connections receive it
3. Close first connection
4. Send another message → second connection does NOT receive it (BUG)

## How to Run

```bash
cd test/unsub-listeners
./run-test.sh
```

**Prerequisites:** Docker, Docker Compose, ports 8081 and 16379 available.

The test will automatically:
- Start Bridge and Valkey services
- Open 2 SSE connections
- Verify the bug is fixed
- Clean up
