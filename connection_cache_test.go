package main

import (
	"testing"
	"time"
)

func TestConnectionCache_BasicOperations(t *testing.T) {
	cache := NewConnectionCache(2, time.Minute)

	// Test Add and Verify
	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")

	// Should verify successfully
	if !cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected verification to succeed")
	}

	// Should fail with wrong parameters
	if cache.Verify("client2", "127.0.0.1", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected verification to fail with wrong client ID")
	}

	if cache.Verify("client1", "192.168.1.1", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected verification to fail with wrong IP")
	}

	if cache.Verify("client1", "127.0.0.1", "https://other.com", "Mozilla/5.0") {
		t.Error("Expected verification to fail with wrong origin")
	}

	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Chrome/91.0") {
		t.Error("Expected verification to fail with wrong user agent")
	}
}

func TestConnectionCache_Expiration(t *testing.T) {
	cache := NewConnectionCache(10, 50*time.Millisecond)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")

	// Should verify immediately
	if !cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected verification to succeed immediately")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should fail after expiration
	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected verification to fail after expiration")
	}
}

func TestConnectionCache_CapacityLimit(t *testing.T) {
	cache := NewConnectionCache(2, time.Minute)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0")
	cache.Add("client3", "127.0.0.3", "https://example.com", "Mozilla/5.0") // Should evict client1

	// client1 should be evicted
	if cache.Verify("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected client1 to be evicted")
	}

	// client2 and client3 should still be there
	if !cache.Verify("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected client2 to still be in cache")
	}
	if !cache.Verify("client3", "127.0.0.3", "https://example.com", "Mozilla/5.0") {
		t.Error("Expected client3 to still be in cache")
	}
}
