package main

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// connectCache manages the cache for connect clients with TTL-based expiration
type connectCache struct {
	cache *expirable.LRU[string, []connectClient]
	ttl   time.Duration
}

// newConnectCache creates a new connect cache with the given configuration
func newConnectCache(cacheSize int, cacheTTL int) *connectCache {
	ttl := time.Duration(cacheTTL) * time.Second
	cache := expirable.NewLRU[string, []connectClient](cacheSize, nil, ttl)

	return &connectCache{
		cache: cache,
		ttl:   ttl,
	}
}

// add adds a connect client to the cache, filtering out expired entries
func (cc *connectCache) add(clientID string, client connectClient) {
	existingClients, found := cc.cache.Get(clientID)
	if !found {
		existingClients = make([]connectClient, 0)
	}

	now := time.Now()
	validClients := make([]connectClient, 0, len(existingClients)+1)
	for _, existing := range existingClients {
		if now.Sub(existing.time) < cc.ttl {
			validClients = append(validClients, existing)
		}
	}

	validClients = append(validClients, client)
	cc.cache.Add(clientID, validClients)
}

// get retrieves connect clients from cache, filtering out expired entries
func (cc *connectCache) get(clientID string) []connectClient {
	clients, found := cc.cache.Get(clientID)
	if !found {
		return nil
	}

	now := time.Now()
	validClients := make([]connectClient, 0, len(clients))
	for _, client := range clients {
		if now.Sub(client.time) < cc.ttl {
			validClients = append(validClients, client)
		}
	}

	if len(validClients) == 0 {
		cc.cache.Remove(clientID)
		return nil
	}

	if len(validClients) != len(clients) {
		cc.cache.Add(clientID, validClients)
	}

	return validClients
}
