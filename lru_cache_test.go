package main

import (
	"testing"
	"time"
)

func TestLRUCache_BasicOperations(t *testing.T) {
	cache := NewLRUCache(2, time.Minute)

	// Test Add and Get
	client1 := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	cache.Add("key1", client1)

	retrieved, found := cache.Get("key1")
	if !found {
		t.Fatal("Expected to find key1")
	}
	if retrieved.clientId != "client1" {
		t.Fatal("Retrieved value doesn't match")
	}

	// Test capacity limit
	client2 := connectClient{clientId: "client2", ip: "127.0.0.2", origin: "test", time: time.Now()}
	client3 := connectClient{clientId: "client3", ip: "127.0.0.3", origin: "test", time: time.Now()}

	cache.Add("key2", client2)
	cache.Add("key3", client3) // This should evict key1

	// key1 should be evicted
	_, found = cache.Get("key1")
	if found {
		t.Fatal("key1 should have been evicted")
	}

	// key2 and key3 should still be there
	_, found = cache.Get("key2")
	if !found {
		t.Fatal("key2 should still be in cache")
	}
	_, found = cache.Get("key3")
	if !found {
		t.Fatal("key3 should still be in cache")
	}
}

func TestLRUCache_Expiration(t *testing.T) {
	cache := NewLRUCache(10, 50*time.Millisecond)

	client := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	cache.Add("key1", client)

	// Should be found immediately
	_, found := cache.Get("key1")
	if !found {
		t.Fatal("key1 should be found")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	_, found = cache.Get("key1")
	if found {
		t.Fatal("key1 should have expired")
	}
}

func TestLRUCache_LRUBehavior(t *testing.T) {
	cache := NewLRUCache(2, time.Minute)

	client1 := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	client2 := connectClient{clientId: "client2", ip: "127.0.0.2", origin: "test", time: time.Now()}
	client3 := connectClient{clientId: "client3", ip: "127.0.0.3", origin: "test", time: time.Now()}

	cache.Add("key1", client1)
	cache.Add("key2", client2)

	// Access key1 to make it recently used
	cache.Get("key1")

	// Add key3, which should evict key2 (least recently used)
	cache.Add("key3", client3)

	// key1 should still be there (recently used)
	_, found := cache.Get("key1")
	if !found {
		t.Fatal("key1 should still be in cache")
	}

	// key2 should be evicted
	_, found = cache.Get("key2")
	if found {
		t.Fatal("key2 should have been evicted")
	}

	// key3 should be there
	_, found = cache.Get("key3")
	if !found {
		t.Fatal("key3 should be in cache")
	}
}

func TestLRUCache_Remove(t *testing.T) {
	cache := NewLRUCache(10, time.Minute)

	client := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	cache.Add("key1", client)

	// Should be found
	_, found := cache.Get("key1")
	if !found {
		t.Fatal("key1 should be found")
	}

	// Remove it
	removed := cache.Remove("key1")
	if !removed {
		t.Fatal("Remove should return true")
	}

	// Should not be found anymore
	_, found = cache.Get("key1")
	if found {
		t.Fatal("key1 should not be found after removal")
	}

	// Removing non-existent key should return false
	removed = cache.Remove("nonexistent")
	if removed {
		t.Fatal("Remove should return false for non-existent key")
	}
}

func TestLRUCache_CleanExpired(t *testing.T) {
	cache := NewLRUCache(10, 50*time.Millisecond)

	client1 := connectClient{clientId: "client1", ip: "127.0.0.1", origin: "test", time: time.Now()}
	client2 := connectClient{clientId: "client2", ip: "127.0.0.2", origin: "test", time: time.Now()}

	cache.Add("key1", client1)
	time.Sleep(60 * time.Millisecond) // Let key1 expire
	cache.Add("key2", client2)        // key2 is not expired

	if cache.Len() != 2 {
		t.Fatal("Cache should have 2 items before cleanup")
	}

	cache.CleanExpired()

	if cache.Len() != 1 {
		t.Fatal("Cache should have 1 item after cleanup")
	}

	// key1 should be gone
	_, found := cache.Get("key1")
	if found {
		t.Fatal("key1 should be cleaned up")
	}

	// key2 should still be there
	_, found = cache.Get("key2")
	if !found {
		t.Fatal("key2 should still be in cache")
	}
}
