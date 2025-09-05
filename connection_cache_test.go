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

	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Chrome/91.0") != "warning" {
		t.Error("Expected verification to return 'warning' with wrong user agent")
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

func TestConnectionCache_VerificationStatuses(t *testing.T) {
	cache := NewConnectionCache(10, time.Minute)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")

	if status := cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0"); status != "ok" {
		t.Errorf("Expected 'ok', got '%s'", status)
	}

	if status := cache.Verify("client1", "127.0.0.1", "https://malicious.com", "Mozilla/5.0"); status != "danger" {
		t.Errorf("Expected 'danger', got '%s'", status)
	}

	if status := cache.Verify("client1", "192.168.1.1", "https://example.com", "Mozilla/5.0"); status != "warning" {
		t.Errorf("Expected 'warning', got '%s'", status)
	}

	if status := cache.Verify("client1", "127.0.0.1", "https://example.com", "Chrome/91.0"); status != "warning" {
		t.Errorf("Expected 'warning', got '%s'", status)
	}

	if status := cache.Verify("client999", "10.0.0.1", "https://newsite.com", "Safari/14.0"); status != "unknown" {
		t.Errorf("Expected 'unknown', got '%s'", status)
	}

	cacheShort := NewConnectionCache(10, 10*time.Millisecond)
	cacheShort.Add("client2", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	time.Sleep(20 * time.Millisecond)

	if status := cacheShort.Verify("client2", "127.0.0.1", "https://example.com", "Mozilla/5.0"); status != "unknown" {
		t.Errorf("Expected 'unknown' for expired entry, got '%s'", status)
	}
}
