package main

import (
	"testing"
	"time"
)

func TestConnectionCache_BasicOperations(t *testing.T) {
	cache := NewConnectionCache(2, time.Minute)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")

	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") != "ok" {
		t.Error("Expected verification to return 'ok'")
	}

	if cache.Verify("client2", "127.0.0.1", "https://example.com", "Mozilla/5.0") != "unknown" {
		t.Error("Expected verification to return 'unknown' with wrong client ID")
	}

	if cache.Verify("client1", "192.168.1.1", "https://example.com", "Mozilla/5.0") != "warning" {
		t.Error("Expected verification to return 'warning' with wrong IP")
	}

	if cache.Verify("client1", "127.0.0.1", "https://other.com", "Mozilla/5.0") != "danger" {
		t.Error("Expected verification to return 'danger' with wrong origin")
	}

	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Chrome/91.0") != "danger" {
		t.Error("Expected verification to return 'danger' with wrong user agent")
	}
}

func TestConnectionCache_Expiration(t *testing.T) {
	cache := NewConnectionCache(10, 50*time.Millisecond)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")

	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") != "ok" {
		t.Error("Expected verification to return 'ok' immediately")
	}

	time.Sleep(60 * time.Millisecond)

	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") != "unknown" {
		t.Error("Expected verification to return 'unknown' after expiration")
	}
}

func TestConnectionCache_CapacityLimit(t *testing.T) {
	cache := NewConnectionCache(2, time.Minute)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0")
	cache.Add("client3", "127.0.0.3", "https://example.com", "Mozilla/5.0") // Should evict client1

	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") != "unknown" {
		t.Error("Expected client1 to be evicted and return 'unknown'")
	}

	if cache.Verify("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0") != "ok" {
		t.Error("Expected client2 to still be in cache and return 'ok'")
	}
	if cache.Verify("client3", "127.0.0.3", "https://example.com", "Mozilla/5.0") != "ok" {
		t.Error("Expected client3 to still be in cache and return 'ok'")
	}
}

func TestConnectionCache_MultipleConnectionsSameClient(t *testing.T) {
	cache := NewConnectionCache(10, time.Minute)

	// Add multiple connections for same client with different IPs
	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client1", "192.168.1.1", "https://example.com", "Mozilla/5.0")

	// Both connections should be verified as "ok"
	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") != "ok" {
		t.Error("Expected first connection to return 'ok'")
	}

	if cache.Verify("client1", "192.168.1.1", "https://example.com", "Mozilla/5.0") != "ok" {
		t.Error("Expected second connection to return 'ok'")
	}

	// Different IP for same client should return "warning"
	if cache.Verify("client1", "10.0.0.1", "https://example.com", "Mozilla/5.0") != "warning" {
		t.Error("Expected verification with new IP to return 'warning'")
	}

	// Add connection with different origin - should trigger "danger" for new requests
	cache.Add("client1", "127.0.0.1", "https://malicious.com", "Mozilla/5.0")

	if cache.Verify("client1", "127.0.0.1", "https://different.com", "Mozilla/5.0") != "danger" {
		t.Error("Expected verification with different origin to return 'danger'")
	}
}

func TestConnectionCache_CleanExpired(t *testing.T) {
	cache := NewConnectionCache(10, 50*time.Millisecond)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0")

	if cache.Len() != 2 {
		t.Errorf("Expected cache length to be 2, got %d", cache.Len())
	}

	time.Sleep(60 * time.Millisecond)
	cache.CleanExpired()

	if cache.Len() != 0 {
		t.Errorf("Expected cache length to be 0 after cleanup, got %d", cache.Len())
	}
}
