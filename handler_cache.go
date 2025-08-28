package main

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/tonkeeper/bridge/config"
)

// initConnectCache initializes the connect client cache for the handler
func (h *handler) initConnectCache() {
	ttl :=  time.Duration(config.Config.ConnectCacheTTL) * time.Second
	h.connectCache = expirable.NewLRU[string, []connectClient](config.Config.ConnectCacheSize, nil, ttl)
	h.connectCacheTTL = ttl
}

// addConnectClient adds a connect client to the cache, filtering out expired entries
func (h *handler) addConnectClient(clientID string, client connectClient) {
	existingClients, found := h.connectCache.Get(clientID)
	if !found {
		existingClients = make([]connectClient, 0)
	}

	now := time.Now()
	validClients := make([]connectClient, 0, len(existingClients)+1)
	for _, existing := range existingClients {
		if now.Sub(existing.time) < h.connectCacheTTL {
			validClients = append(validClients, existing)
		}
	}

	validClients = append(validClients, client)
	h.connectCache.Add(clientID, validClients)
}

// getConnectClients retrieves connect clients from cache, filtering out expired entries
func (h *handler) getConnectClients(clientID string) []connectClient {
	clients, found := h.connectCache.Get(clientID)
	if !found {
		return nil
	}

	now := time.Now()
	validClients := make([]connectClient, 0, len(clients))
	for _, client := range clients {
		if now.Sub(client.time) < h.connectCacheTTL {
			validClients = append(validClients, client)
		}
	}

	if len(validClients) == 0 {
		h.connectCache.Remove(clientID)
		return nil
	}

	if len(validClients) != len(clients) {
		h.connectCache.Add(clientID, validClients)
	}

	return validClients
}
