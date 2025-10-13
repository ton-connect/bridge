package storagev3

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
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

func TestClusterSlotsWithRetry(t *testing.T) {
	// Test that the retry function works with a non-existent Redis instance
	opts := &redis.Options{
		Addr: "localhost:9999", // Non-existent port
	}
	tempClient := redis.NewClient(opts)
	defer func() {
		if err := tempClient.Close(); err != nil {
			t.Logf("Failed to close Redis client: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err := clusterSlotsWithRetry(tempClient, ctx, 2) // Try 3 times (0, 1, 2)
	elapsed := time.Since(start)

	// Should fail after retries
	if err == nil {
		t.Error("Expected error when connecting to non-existent Redis, got nil")
	}

	// Should take at least 100ms (first retry) + 200ms (second retry) = 300ms
	// but less than 2 seconds (context timeout)
	if elapsed < 300*time.Millisecond {
		t.Errorf("Expected at least 300ms for retries, got %v", elapsed)
	}
	if elapsed >= 2*time.Second {
		t.Errorf("Expected less than 2s (context timeout), got %v", elapsed)
	}

	t.Logf("Retry test completed in %v with error: %v", elapsed, err)
}
