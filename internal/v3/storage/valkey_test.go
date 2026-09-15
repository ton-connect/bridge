package storagev3

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/ton-connect/bridge/internal/models"
)

func TestNewValkeyStorage_SingleNode(t *testing.T) {
	// Test with invalid URI to ensure it fails gracefully
	_, err := NewValkeyStorage("invalid://uri")
	if err == nil {
		t.Error("Expected error for invalid URI, got nil")
	}
}

func TestNewValkeyStorage_RedisURI(t *testing.T) {
	// Test parsing of redis:// URI (should work even if connection fails)
	valkeyURI := "redis://localhost:6379"

	// This will fail to connect, but should parse URI correctly
	_, err := NewValkeyStorage(valkeyURI)

	// We expect a connection error, not a parsing error
	if err != nil && err.Error() != "connection failed: dial tcp [::1]:6379: connect: connection refused" &&
		err.Error() != "connection failed: dial tcp 127.0.0.1:6379: connect: connection refused" {
		// Connection error is expected since there's no Redis running
		// But the error message format tells us the URI was parsed correctly
		t.Logf("Connection failed as expected: %v", err)
	}
}

// getTestValkeyURI returns the Valkey URI from environment or skips the test
func getTestValkeyURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("VALKEY_URI")
	if uri == "" {
		uri = os.Getenv("REDIS_URI")
	}
	if uri == "" {
		t.Skip("Skipping Valkey integration test: VALKEY_URI or REDIS_URI not set")
	}
	return uri
}

// waitSwept blocks until the sweep Sub launched has removed the expired members, or fails
// the test. Sub no longer sweeps inline - it would hold the subscriber lock across the
// round trips - so the counter these tests read is written by a goroutine, and the backlog
// shrinking is the observable edge that says the write has happened.
func waitSwept(t *testing.T, storage *ValkeyStorage, clientID string, want int64) {
	t.Helper()
	clientKey := fmt.Sprintf("client:%s", clientID)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, err := storage.client.ZCard(context.Background(), clientKey).Result()
		if err != nil {
			t.Fatalf("ZCard failed: %v", err)
		}
		if n == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("sweep did not reduce the backlog of %s to %d entries within 5s", clientID, want)
}

// cleanupKeys removes the keys a test made. They hash to different slots, so a multi-key
// DEL would come back CROSSSLOT and delete neither.
func cleanupKeys(storage *ValkeyStorage, clientID string) {
	ctx := context.Background()
	storage.client.Del(ctx, fmt.Sprintf("client:%s", clientID))
	storage.client.Del(ctx, deliveredKey(clientID))
}

func TestValkeyStorage_ConnectionVerification_ExactMatch(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	conn := ConnectionInfo{
		ClientID:  "test-client-1",
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}

	// Add connection
	ttl := 5 * time.Second
	if err := storage.AddConnection(ctx, conn, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify with exact match -> should return "ok"
	status, err := storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_SameOriginDifferentIP(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := "test-client-2"

	// Add first connection
	conn1 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}
	ttl := 5 * time.Second
	if err := storage.AddConnection(ctx, conn1, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify with same origin but different IP -> should return "warning"
	conn2 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.2",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/2.0",
	}
	status, err := storage.VerifyConnection(ctx, conn2)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "warning" {
		t.Errorf("expected status 'warning', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_DifferentOrigin(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := "test-client-3"

	// Add first connection
	conn1 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}
	ttl := 5 * time.Second
	if err := storage.AddConnection(ctx, conn1, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify with different origin -> should return "danger"
	conn2 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.2",
		Origin:    "https://malicious.com",
		UserAgent: "TestAgent/2.0",
	}
	status, err := storage.VerifyConnection(ctx, conn2)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "danger" {
		t.Errorf("expected status 'danger', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_Unknown(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Verify without adding connection -> should return "unknown"
	conn := ConnectionInfo{
		ClientID:  "test-client-unknown",
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}
	status, err := storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "unknown" {
		t.Errorf("expected status 'unknown', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_TTLExpiration(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	conn := ConnectionInfo{
		ClientID:  "test-client-ttl",
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}

	// Add connection with short TTL
	ttl := 2 * time.Second
	if err := storage.AddConnection(ctx, conn, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify immediately -> should return "ok"
	status, err := storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "ok" {
		t.Errorf("expected status 'ok' before expiration, got '%s'", status)
	}

	// Wait for TTL to expire
	time.Sleep(3 * time.Second)

	// Verify after expiration -> should return "unknown"
	status, err = storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "unknown" {
		t.Errorf("expected status 'unknown' after expiration, got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_MultipleConnections(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := "test-client-multi"
	ttl := 10 * time.Second

	// Add multiple connections for the same client
	connections := []ConnectionInfo{
		{ClientID: clientID, IP: "192.168.1.1", Origin: "https://example.com", UserAgent: "Browser/1.0"},
		{ClientID: clientID, IP: "192.168.1.2", Origin: "https://example.com", UserAgent: "Browser/1.0"},
		{ClientID: clientID, IP: "10.0.0.1", Origin: "https://example.com", UserAgent: "Mobile/1.0"},
	}

	for _, conn := range connections {
		if err := storage.AddConnection(ctx, conn, ttl); err != nil {
			t.Fatalf("AddConnection failed: %v", err)
		}
	}

	// Verify exact match with first connection -> "ok"
	status, err := storage.VerifyConnection(ctx, connections[0])
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "ok" {
		t.Errorf("expected 'ok' for exact match, got '%s'", status)
	}

	// Verify with new IP but same origin -> "warning"
	newConn := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.100",
		Origin:    "https://example.com",
		UserAgent: "NewBrowser/1.0",
	}
	status, err = storage.VerifyConnection(ctx, newConn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "warning" {
		t.Errorf("expected 'warning' for same origin different IP, got '%s'", status)
	}
}

// TestValkeyStorage_SubCountsUndeliveredExpiry drives the production call site rather than
// the sweep helper: expiry is resolved inside Sub, so a test that called the helper
// directly would stay green if that call were dropped or moved after the replay.
func TestValkeyStorage_SubCountsUndeliveredExpiry(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("reap-%d", time.Now().UnixNano())

	expiring := models.SseMessage{
		EventId: 1,
		Message: []byte(`{"from":"sender","message":"gone","trace_id":"trace-expired"}`),
		To:      clientID,
	}
	surviving := models.SseMessage{
		EventId: 2,
		Message: []byte(`{"from":"sender","message":"kept","trace_id":"trace-live"}`),
		To:      clientID,
	}
	if err := storage.Pub(ctx, expiring, 1); err != nil {
		t.Fatalf("Pub(expiring) failed: %v", err)
	}
	if err := storage.Pub(ctx, surviving, 300); err != nil {
		t.Fatalf("Pub(surviving) failed: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	before := testutil.ToFloat64(expiredMessagesMetric)

	messageCh := make(chan models.SseMessage, 8)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()

	// One of the two survives its TTL, so a finished sweep leaves exactly one entry.
	waitSwept(t, storage, clientID, 1)

	if got := testutil.ToFloat64(expiredMessagesMetric) - before; got != 1 {
		t.Errorf("expected the undelivered expired message to be counted once, counter moved by %v", got)
	}

	// Sub replays the backlog it did not sweep, so the live message arrives and the expired
	// one must not: a client reconnecting after the TTL has to see neither the message nor
	// a gap in event ids it cannot explain.
	var replayed []int64
	for len(messageCh) > 0 {
		replayed = append(replayed, (<-messageCh).EventId)
	}
	if len(replayed) != 1 || replayed[0] != surviving.EventId {
		t.Errorf("expected only the live message replayed, got event ids %v", replayed)
	}

	// A second reconnect finds nothing left to sweep and must add nothing. There is no
	// state change to wait on here - a sweep that removes nothing leaves no trace - so this
	// one gets a settle long enough for a sweep that did run to have finished counting.
	after := testutil.ToFloat64(expiredMessagesMetric)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("second Sub failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if got := testutil.ToFloat64(expiredMessagesMetric); got != after {
		t.Errorf("second sweep double-counted: counter moved from %v to %v", after, got)
	}

	cleanupKeys(storage, clientID)
}

// TestValkeyStorage_DeliveredExpiryIsNotCounted pins the distinction the counter exists for.
// Pub stores a copy of every message whether or not anyone was listening, so a delivered
// message sits in the backlog until its TTL exactly like an undelivered one. Counting the
// sweep blindly would report successful deliveries as losses.
func TestValkeyStorage_DeliveredExpiryIsNotCounted(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("delivered-%d", time.Now().UnixNano())

	msg := models.SseMessage{
		EventId: 7,
		Message: []byte(`{"from":"sender","message":"seen","trace_id":"trace-delivered"}`),
		To:      clientID,
	}
	if err := storage.Pub(ctx, msg, 1); err != nil {
		t.Fatalf("Pub failed: %v", err)
	}
	if err := storage.MarkDelivered(ctx, clientID, msg.EventId); err != nil {
		t.Fatalf("MarkDelivered failed: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	before := testutil.ToFloat64(expiredMessagesMetric)

	messageCh := make(chan models.SseMessage, 4)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()

	// The only message expired, so a finished sweep empties the backlog.
	waitSwept(t, storage, clientID, 0)

	if got := testutil.ToFloat64(expiredMessagesMetric); got != before {
		t.Errorf("a delivered message was counted as expired: counter moved from %v to %v", before, got)
	}

	cleanupKeys(storage, clientID)
}
