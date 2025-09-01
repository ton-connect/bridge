package main

import (
	"testing"
	"time"
)

func TestLRUCache_BasicOperations(t *testing.T) {
	cache := NewLRUCache(2, time.Minute)

	// Test Add and Get with clientId and session key
	client1 := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	sessionKey1 := "client1:127.0.0.1:test"
	cache.Add(sessionKey1, client1)

	// Test getting by session key
	retrieved, found := cache.Get(sessionKey1)
	if !found {
		t.Fatal("Expected to find session key")
	}
	if retrieved.clientId != "client1" {
		t.Fatal("Retrieved value doesn't match")
	}

	// Test getting by client ID
	retrieved, found = cache.Get("client1")
	if !found {
		t.Fatal("Expected to find by client ID")
	}
	if retrieved.clientId != "client1" {
		t.Fatal("Retrieved value doesn't match")
	}

	// Test capacity limit with different clients
	client2 := connectClient{clientId: "client2", ip: "127.0.0.2", origin: "test", time: time.Now()}
	client3 := connectClient{clientId: "client3", ip: "127.0.0.3", origin: "test", time: time.Now()}

	sessionKey2 := "client2:127.0.0.2:test"
	sessionKey3 := "client3:127.0.0.3:test"

	cache.Add(sessionKey2, client2)
	cache.Add(sessionKey3, client3) // This should evict client1

	// client1 should be evicted
	_, found = cache.Get("client1")
	if found {
		t.Fatal("client1 should have been evicted")
	}

	// client2 and client3 should still be there
	_, found = cache.Get("client2")
	if !found {
		t.Fatal("client2 should still be in cache")
	}
	_, found = cache.Get("client3")
	if !found {
		t.Fatal("client3 should still be in cache")
	}
}

func TestLRUCache_Expiration(t *testing.T) {
	cache := NewLRUCache(10, 50*time.Millisecond)

	client := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	sessionKey := "client1:127.0.0.1:test"
	cache.Add(sessionKey, client)

	// Should be found immediately
	_, found := cache.Get(sessionKey)
	if !found {
		t.Fatal("session should be found")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	_, found = cache.Get(sessionKey)
	if found {
		t.Fatal("session should have expired")
	}
}

func TestLRUCache_LRUBehavior(t *testing.T) {
	cache := NewLRUCache(2, time.Minute)

	client1 := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	client2 := connectClient{clientId: "client2", ip: "127.0.0.2", origin: "test", time: time.Now()}
	client3 := connectClient{clientId: "client3", ip: "127.0.0.3", origin: "test", time: time.Now()}

	sessionKey1 := "client1:127.0.0.1:test"
	sessionKey2 := "client2:127.0.0.2:test"
	sessionKey3 := "client3:127.0.0.3:test"

	cache.Add(sessionKey1, client1)
	cache.Add(sessionKey2, client2)

	// Access client1 to make it recently used
	cache.Get("client1")

	// Add client3, which should evict client2 (least recently used)
	cache.Add(sessionKey3, client3)

	// client1 should still be there (recently used)
	_, found := cache.Get("client1")
	if !found {
		t.Fatal("client1 should still be in cache")
	}

	// client2 should be evicted
	_, found = cache.Get("client2")
	if found {
		t.Fatal("client2 should have been evicted")
	}

	// client3 should be there
	_, found = cache.Get("client3")
	if !found {
		t.Fatal("client3 should be in cache")
	}
}

func TestLRUCache_CleanExpired(t *testing.T) {
	cache := NewLRUCache(10, 50*time.Millisecond)

	client1 := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	client2 := connectClient{clientId: "client2", ip: "127.0.0.2", origin: "test", time: time.Now()}

	sessionKey1 := "client1:127.0.0.1:test"
	sessionKey2 := "client2:127.0.0.2:test"

	cache.Add(sessionKey1, client1)
	time.Sleep(60 * time.Millisecond) // Let client1 expire
	cache.Add(sessionKey2, client2)   // client2 is not expired

	if cache.Len() != 2 {
		t.Fatal("Cache should have 2 items before cleanup")
	}

	cache.CleanExpired()

	if cache.Len() != 1 {
		t.Fatal("Cache should have 1 item after cleanup")
	}

	// client1 should be gone
	_, found := cache.Get("client1")
	if found {
		t.Fatal("client1 should be cleaned up")
	}

	// client2 should still be there
	_, found = cache.Get("client2")
	if !found {
		t.Fatal("client2 should still be in cache")
	}
}

func TestLRUCache_GetAllSessions(t *testing.T) {
	cache := NewLRUCache(10, time.Minute)

	// Add multiple sessions for the same client
	client1_session1 := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "origin1", time: time.Now()}
	client1_session2 := connectClient{clientId: "client1", ip: "127.0.0.2", origin: "origin2", time: time.Now()}
	client2_session1 := connectClient{clientId: "client2", ip: "127.0.0.3", origin: "origin1", time: time.Now()}

	sessionKey1 := "client1:127.0.0.1:origin1"
	sessionKey2 := "client1:127.0.0.2:origin2"
	sessionKey3 := "client2:127.0.0.3:origin1"

	cache.Add(sessionKey1, client1_session1)
	cache.Add(sessionKey2, client1_session2)
	cache.Add(sessionKey3, client2_session1)

	// Get all sessions for client1
	sessions, found := cache.GetAllSessions("client1")
	if !found {
		t.Fatal("Expected to find sessions for client1")
	}
	if len(sessions) != 2 {
		t.Fatalf("Expected 2 sessions for client1, got %d", len(sessions))
	}

	// Verify sessions contain the right data
	foundOrigin1 := false
	foundOrigin2 := false
	for _, session := range sessions {
		if session.origin == "origin1" {
			foundOrigin1 = true
		}
		if session.origin == "origin2" {
			foundOrigin2 = true
		}
	}
	if !foundOrigin1 || !foundOrigin2 {
		t.Fatal("Expected to find both origin1 and origin2 sessions")
	}

	// Get all sessions for client2
	sessions, found = cache.GetAllSessions("client2")
	if !found {
		t.Fatal("Expected to find sessions for client2")
	}
	if len(sessions) != 1 {
		t.Fatalf("Expected 1 session for client2, got %d", len(sessions))
	}

	// Get all sessions for non-existent client
	sessions, found = cache.GetAllSessions("client3")
	if found {
		t.Fatal("Should not find sessions for non-existent client")
	}
	if sessions != nil {
		t.Fatal("Sessions should be nil for non-existent client")
	}
}
