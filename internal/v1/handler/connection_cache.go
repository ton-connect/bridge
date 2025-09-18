package handlerv1

import (
	"container/list"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
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
	capacity    int
	ttl         time.Duration
	items       map[ConnectionKey]*list.Element // struct key -> element
	clientIndex map[string][]*list.Element      // clientID -> list of elements for efficient lookup
	evictList   *list.List                      // LRU order
	mutex       sync.RWMutex
}

// connectionCacheEntry represents a single cache entry
type connectionCacheEntry struct {
	key        ConnectionKey
	expiration time.Time
}

// NewConnectionCache creates a new connection cache with the specified capacity and TTL
func NewConnectionCache(capacity int, ttl time.Duration) *ConnectionCache {
	return &ConnectionCache{
		capacity:    capacity,
		ttl:         ttl,
		items:       make(map[ConnectionKey]*list.Element),
		clientIndex: make(map[string][]*list.Element),
		evictList:   list.New(),
	}
}

// StartBackgroundCleanup starts a background goroutine that periodically cleans expired entries
func (c *ConnectionCache) StartBackgroundCleanup(customCleanupInterval *time.Duration) {
	tickerDuration := time.Minute
	if customCleanupInterval != nil {
		tickerDuration = *customCleanupInterval
	}
	go func() {
		ticker := time.NewTicker(tickerDuration)
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

	// Add to clientID index
	c.clientIndex[clientID] = append(c.clientIndex[clientID], element)
}

// Verify checks transaction source and returns verification status
// Returns: "ok" (verified match), "danger" (fraud indication), "warning" (suspicious), "unknown" (new/untracked)
func (c *ConnectionCache) Verify(clientID, ip, origin string) string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	foundCount := 0
	leastSuspicious := "danger"
	now := time.Now()
	// No exact match - check for partial matches using clientID index (O(1) lookup)
	if elements, exists := c.clientIndex[clientID]; exists {
		for _, ent := range elements {
			entry := ent.Value.(*connectionCacheEntry)

			if now.After(entry.expiration) {
				continue
			}

			foundCount++

			cachedKey := entry.key
			// If we have an exact match, return ok
			if cachedKey.Origin == origin && cachedKey.IP == ip {
				return "ok"
			}

			// If we have at least Origin match, return warning
			if cachedKey.Origin == origin {
				leastSuspicious = "warning"
			}
		}
	}

	if foundCount == 0 {
		return "unknown"
	}

	return leastSuspicious
}

// CleanExpired removes all expired entries from the cache
func (c *ConnectionCache) CleanExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for {
		ent := c.evictList.Back()
		if ent == nil {
			break
		}

		entry := ent.Value.(*connectionCacheEntry)
		if now.After(entry.expiration) {
			c.removeElement(ent)
		} else {
			break
		}
	}
}

// removeOldest removes the oldest entry from the cache
// Should be called with mutex locked
func (c *ConnectionCache) removeOldest() {
	if c.mutex.TryLock() {
		log.Error("removeOldest called without mutex locked!!!")
		defer c.mutex.Unlock()
	}

	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement removes a specific element from the cache
// Should be called with mutex locked
func (c *ConnectionCache) removeElement(e *list.Element) {
	if c.mutex.TryLock() {
		log.Error("removeElement called without mutex locked!!!")
		defer c.mutex.Unlock()
	}

	if e == nil {
		log.Error("removeElement called with nil element!!!")
		return
	}

	c.evictList.Remove(e)
	entry := e.Value.(*connectionCacheEntry)
	delete(c.items, entry.key)

	// Remove from clientID index
	clientID := entry.key.ClientID
	elements := c.clientIndex[clientID]
	for i, elem := range elements {
		if elem == e {
			// Remove this element from the slice
			c.clientIndex[clientID] = append(elements[:i], elements[i+1:]...)
			break
		}
	}

	// Clean up empty slice
	if len(c.clientIndex[clientID]) == 0 {
		delete(c.clientIndex, clientID)
	}
}

// Len returns the number of items in the cache
func (c *ConnectionCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.evictList.Len()
}
