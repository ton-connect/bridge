package main

import (
	"container/list"
	"sync"
	"time"
)

// ConnectionKey represents a unique connection identifier
type ConnectionKey struct {
	ClientID  string
	IP        string
	Origin    string
	UserAgent string
}

// ConnectionCache implements a simplified LRU cache for connection verification
type ConnectionCache struct {
	capacity  int
	ttl       time.Duration
	items     map[ConnectionKey]*list.Element // struct key -> element
	evictList *list.List                      // LRU order
	mutex     sync.RWMutex
}

// connectionCacheEntry represents a single cache entry
type connectionCacheEntry struct {
	key        ConnectionKey
	expiration time.Time
}

// NewConnectionCache creates a new connection cache with the specified capacity and TTL
func NewConnectionCache(capacity int, ttl time.Duration) *ConnectionCache {
	return &ConnectionCache{
		capacity:  capacity,
		ttl:       ttl,
		items:     make(map[ConnectionKey]*list.Element),
		evictList: list.New(),
	}
}

// StartBackgroundCleanup starts a background goroutine that periodically cleans expired entries
func (c *ConnectionCache) StartBackgroundCleanup() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.CleanExpired()
		}
	}()
}

// Add adds a connection to the cache
func (c *ConnectionCache) Add(clientID, ip, origin, userAgent string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	key := ConnectionKey{ClientID: clientID, IP: ip, Origin: origin, UserAgent: userAgent}
	now := time.Now()
	expiration := now.Add(c.ttl)

	// Check if key already exists
	if ent, ok := c.items[key]; ok {
		// Update existing entry
		c.evictList.MoveToFront(ent)
		entry := ent.Value.(*connectionCacheEntry)
		entry.expiration = expiration
		return
	}

	// Check if we need to evict
	if c.evictList.Len() >= c.capacity {
		c.removeOldest()
	}

	// Add new entry
	entry := &connectionCacheEntry{
		key:        key,
		expiration: expiration,
	}

	element := c.evictList.PushFront(entry)
	c.items[key] = element
}

// Verify checks transaction source and returns verification status
// Returns: "ok" (verified match), "danger" (fraud indication), "warning" (suspicious), "unknown" (new/untracked)
func (c *ConnectionCache) Verify(clientID, ip, origin, userAgent string) string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	key := ConnectionKey{ClientID: clientID, IP: ip, Origin: origin, UserAgent: userAgent}

	// Check for exact match first
	if ent, ok := c.items[key]; ok {
		entry := ent.Value.(*connectionCacheEntry)

		// Check if expired
		if time.Now().After(entry.expiration) {
			// Expired entry - treat as unknown
			return "unknown"
		}

		// Exact match found and valid
		return "ok"
	}

	// No exact match - check for partial matches to detect suspicious activity
	for cachedKey, ent := range c.items {
		entry := ent.Value.(*connectionCacheEntry)

		// Skip expired entries
		if time.Now().After(entry.expiration) {
			continue
		}

		// Check if same client ID but different details
		if cachedKey.ClientID == clientID {
			// Same client but different origin - strong fraud indicator
			if cachedKey.Origin != origin {
				return "danger"
			}

			// Same client and origin but different IP or User-Agent - suspicious but could be legitimate
			if cachedKey.IP != ip || cachedKey.UserAgent != userAgent {
				return "warning"
			}
		}
	}

	// No matching client ID found - new or untracked origin
	return "unknown"
}

// CleanExpired removes all expired entries from the cache
func (c *ConnectionCache) CleanExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for {
		// Check from the back (oldest entries)
		ent := c.evictList.Back()
		if ent == nil {
			break
		}

		entry := ent.Value.(*connectionCacheEntry)
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
func (c *ConnectionCache) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement removes a specific element from the cache
func (c *ConnectionCache) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	entry := e.Value.(*connectionCacheEntry)
	delete(c.items, entry.key)
}

// Len returns the number of items in the cache
func (c *ConnectionCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.evictList.Len()
}
