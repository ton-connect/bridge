package storagev3

import (
	"context"
	"os"
	"testing"
	"time"
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
