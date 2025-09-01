package main

import (
	"container/list"
	"sync"
	"time"
)

// sessionEntry represents a single session for a client
type sessionEntry struct {
	sessionKey string // unique session key (clientId:ip:origin)
	client     connectClient
	expiration time.Time
}

// cacheEntry represents an entry in the LRU cache with expiration
type cacheEntry struct {
	clientId   string
	sessions   map[string]*sessionEntry // map[sessionKey]*sessionEntry
	expiration time.Time
}

// LRUCache implements a thread-safe LRU cache with TTL expiration
// Now organized by client_id to avoid duplicates and allow getting all sessions per client
type LRUCache struct {
	capacity  int
	ttl       time.Duration
	items     map[string]*list.Element // map[clientId]*list.Element
	evictList *list.List
	mutex     sync.RWMutex
}

// NewLRUCache creates a new LRU cache with the specified capacity and TTL
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity:  capacity,
		ttl:       ttl,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

// StartBackgroundCleanup starts a background goroutine that periodically cleans expired entries
func (c *LRUCache) StartBackgroundCleanup() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.CleanExpired()
		}
	}()
}

// Add adds or updates a session for a client in the cache
func (c *LRUCache) Add(sessionKey string, client connectClient) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	expiration := now.Add(c.ttl)

	clientId := client.clientId

	// Check if client already exists
	if ent, ok := c.items[clientId]; ok {
		// Update existing client entry
		c.evictList.MoveToFront(ent)
		entry := ent.Value.(*cacheEntry)
		entry.expiration = expiration

		// Add or update session
		if entry.sessions == nil {
			entry.sessions = make(map[string]*sessionEntry)
		}
		entry.sessions[sessionKey] = &sessionEntry{
			sessionKey: sessionKey,
			client:     client,
			expiration: expiration,
		}
		return
	}

	// Create new client entry
	sessions := make(map[string]*sessionEntry)
	sessions[sessionKey] = &sessionEntry{
		sessionKey: sessionKey,
		client:     client,
		expiration: expiration,
	}

	entry := &cacheEntry{
		clientId:   clientId,
		sessions:   sessions,
		expiration: expiration,
	}

	// Check if we need to evict
	if c.evictList.Len() >= c.capacity {
		c.removeOldest()
	}

	// Add to front
	element := c.evictList.PushFront(entry)
	c.items[clientId] = element
}

// Get retrieves a session from the cache by sessionKey and moves client to front if found and not expired
func (c *LRUCache) Get(sessionKey string) (connectClient, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// For backward compatibility, try to extract clientId from sessionKey
	// sessionKey can be either just clientId or clientId:ip:origin
	parts := splitSessionKey(sessionKey)
	clientId := parts[0]

	if ent, ok := c.items[clientId]; ok {
		entry := ent.Value.(*cacheEntry)

		// Check if client entry is expired
		if time.Now().After(entry.expiration) {
			c.removeElement(ent)
			return connectClient{}, false
		}

		// Clean expired sessions
		c.cleanExpiredSessions(entry)

		// If no sessions left, remove the client
		if len(entry.sessions) == 0 {
			c.removeElement(ent)
			return connectClient{}, false
		}

		// Move to front (mark as recently used)
		c.evictList.MoveToFront(ent)

		// If sessionKey is just clientId, return any session
		if sessionKey == clientId {
			for _, session := range entry.sessions {
				return session.client, true
			}
		}

		// Look for specific session
		if session, exists := entry.sessions[sessionKey]; exists {
			if time.Now().After(session.expiration) {
				delete(entry.sessions, sessionKey)
				return connectClient{}, false
			}
			return session.client, true
		}
	}

	return connectClient{}, false
}

// GetAllSessions retrieves all active sessions for a given client_id
func (c *LRUCache) GetAllSessions(clientId string) ([]connectClient, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if ent, ok := c.items[clientId]; ok {
		entry := ent.Value.(*cacheEntry)

		// Check if client entry is expired
		if time.Now().After(entry.expiration) {
			c.removeElement(ent)
			return nil, false
		}

		// Clean expired sessions
		c.cleanExpiredSessions(entry)

		// If no sessions left, remove the client
		if len(entry.sessions) == 0 {
			c.removeElement(ent)
			return nil, false
		}

		// Move to front (mark as recently used)
		c.evictList.MoveToFront(ent)

		// Collect all active sessions
		sessions := make([]connectClient, 0, len(entry.sessions))
		for _, session := range entry.sessions {
			sessions = append(sessions, session.client)
		}

		return sessions, len(sessions) > 0
	}

	return nil, false
}

// cleanExpiredSessions removes expired sessions from a client entry
func (c *LRUCache) cleanExpiredSessions(entry *cacheEntry) {
	now := time.Now()
	for sessionKey, session := range entry.sessions {
		if now.After(session.expiration) {
			delete(entry.sessions, sessionKey)
		}
	}
}

// splitSessionKey splits a session key into parts (clientId, ip, origin)
func splitSessionKey(sessionKey string) []string {
	// Use strings package for proper splitting
	if sessionKey == "" {
		return []string{""}
	}

	// If sessionKey contains colons, split by them to extract clientId
	if colonIndex := findFirst(sessionKey, ':'); colonIndex > 0 {
		return []string{sessionKey[:colonIndex]}
	}

	// If no colon found, the entire key is the clientId
	return []string{sessionKey}
}

// findFirst finds the first occurrence of a character in a string
func findFirst(s string, char rune) int {
	for i, c := range s {
		if c == char {
			return i
		}
	}
	return -1
}

// Len returns the number of items in the cache
func (c *LRUCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.evictList.Len()
}

// CleanExpired removes all expired entries from the cache
func (c *LRUCache) CleanExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for {
		// Check from the back (oldest entries)
		ent := c.evictList.Back()
		if ent == nil {
			break
		}

		entry := ent.Value.(*cacheEntry)
		if now.After(entry.expiration) {
			c.removeElement(ent)
		} else {
			// Since we process from oldest to newest, if this one isn't expired,
			// none of the newer ones will be either
			break
		}
	}
}

// removeOldest removes the oldest entry from the cache
func (c *LRUCache) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement removes a specific element from the cache
func (c *LRUCache) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	entry := e.Value.(*cacheEntry)
	delete(c.items, entry.clientId)
}
