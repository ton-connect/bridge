# Go Integration Tests

End-to-end integration tests for the TON Connect Bridge, written in Go. These tests validate core bridge functionality including SSE connections, message delivery, reconnection scenarios, and stress testing.

> **Note**: These tests are adapted from the [bridge-sdk test suite](https://github.com/ton-connect/bridge-sdk/tree/main/test), ported from TypeScript to Go to enable native integration testing without external dependencies.

## Running Tests

```bash
# Run all integration tests
make test-gointegration

# Run specific test
go test -v -run TestBridge_ConnectAndClose ./test/gointegration/

# Run with custom bridge URL
BRIDGE_URL=https://walletbot.me/tonconnect-bridge/bridge go test -v ./test/gointegration/
```
