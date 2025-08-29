package main

import (
	"container/list"
	"sync"
	"time"
)

// cacheEntry represents an entry in the LRU cache with expiration
type cacheEntry struct {
	key        string
	value      connectClient
	expiration time.Time
}

// LRUCache implements a thread-safe LRU cache with TTL expiration
type LRUCache struct {
	capacity  int
	ttl       time.Duration
	items     map[string]*list.Element
	evictList *list.List
	mutex     sync.RWMutex
}

// NewLRUCache creates a new LRU cache with the specified capacity and TTL
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	cache := &LRUCache{
		capacity:  capacity,
		ttl:       ttl,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
	
	// Start background cleanup goroutine
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cache.CleanExpired()
		}
	}()
	
	return cache
}

// Add adds or updates an entry in the cache
func (c *LRUCache) Add(key string, value connectClient) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	expiration := now.Add(c.ttl)

	// Check if key already exists
	if ent, ok := c.items[key]; ok {
		// Update existing entry
		c.evictList.MoveToFront(ent)
		entry := ent.Value.(*cacheEntry)
		entry.value = value
		entry.expiration = expiration
		return
	}

	// Add new entry
	entry := &cacheEntry{
		key:        key,
		value:      value,
		expiration: expiration,
	}

	// Check if we need to evict
	if c.evictList.Len() >= c.capacity {
		c.removeOldest()
	}

	// Add to front
	element := c.evictList.PushFront(entry)
	c.items[key] = element
}

// Get retrieves an entry from the cache and moves it to front if found and not expired
func (c *LRUCache) Get(key string) (connectClient, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if ent, ok := c.items[key]; ok {
		entry := ent.Value.(*cacheEntry)

		// Check if expired
		if time.Now().After(entry.expiration) {
			c.removeElement(ent)
			return connectClient{}, false
		}

		// Move to front (mark as recently used)
		c.evictList.MoveToFront(ent)
		return entry.value, true
	}

	return connectClient{}, false
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
	delete(c.items, entry.key)
}
