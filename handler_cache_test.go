package main

import (
	"testing"
	"time"

	"github.com/tonkeeper/bridge/storage/memory"
)

func TestHandlerExpirableConnectCache(t *testing.T) {
	// Create a handler with embedded cache
	store := memory.NewStorage()
	h := newHandler(store, time.Minute)

	// Test adding clients
	client1 := connectClient{
		clientId: "client1",
		ip:       "192.168.1.1",
		origin:   "https://example.com",
		time:     time.Now(),
	}

	client2 := connectClient{
		clientId: "client2",
		ip:       "192.168.1.2",
		origin:   "https://example2.com",
		time:     time.Now(),
	}

	h.addConnectClient("test1", client1)
	h.addConnectClient("test1", client2) // Add another client to same key

	// Test retrieval
	clients := h.getConnectClients("test1")
	if len(clients) != 2 {
		t.Errorf("Expected 2 clients, got %d", len(clients))
	}

	// Test non-existent key
	clients = h.getConnectClients("nonexistent")
	if clients != nil {
		t.Errorf("Expected nil for non-existent key, got %v", clients)
	}
}

func TestHandlerConnectCacheFiltering(t *testing.T) {
	store := memory.NewStorage()
	h := newHandler(store, time.Minute)

	// Add a client with current time
	currentClient := connectClient{
		clientId: "client1",
		ip:       "192.168.1.1",
		origin:   "https://example.com",
		time:     time.Now(),
	}

	// Add a client that's older than 5 minutes (expired)
	expiredClient := connectClient{
		clientId: "client2",
		ip:       "192.168.1.2",
		origin:   "https://example2.com",
		time:     time.Now().Add(-6 * time.Minute),
	}

	h.addConnectClient("test", currentClient)
	h.addConnectClient("test", expiredClient)

	// When we get the clients, only the current one should be returned
	clients := h.getConnectClients("test")
	if len(clients) != 1 {
		t.Errorf("Expected 1 client (expired one filtered out), got %d", len(clients))
	}

	if len(clients) > 0 && clients[0].clientId != "client1" {
		t.Errorf("Expected current client to remain, got %s", clients[0].clientId)
	}
}
