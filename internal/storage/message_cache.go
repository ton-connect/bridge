package storage

import (
	"sync"
	"time"
)

// MessageCache interface defines methods for caching messages
type MessageCache interface {
	Mark(eventID int64)
	MarkIfNotExists(eventID int64) bool
	IsMarked(eventID int64) bool
	Cleanup() int
	Len() int
}

// InMemoryMessageCache tracks marked messages to avoid logging them as expired
type InMemoryMessageCache struct {
	markedMessages map[int64]time.Time // event_id -> timestamp
	mutex          sync.RWMutex
	ttl            time.Duration
}

// NewMessageCache creates a new message cache instance
func NewMessageCache(enable bool, ttl time.Duration) MessageCache {
	if !enable {
		return &NoopMessageCache{}
	}
	return &InMemoryMessageCache{
		markedMessages: make(map[int64]time.Time),
		ttl:            ttl,
	}
}

// Mark message
func (mc *InMemoryMessageCache) Mark(eventID int64) {
	mc.mutex.Lock()
	mc.markedMessages[eventID] = time.Now()
	mc.mutex.Unlock()
}

// Mark message
func (mc *InMemoryMessageCache) MarkIfNotExists(eventID int64) bool {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()
	if _, exists := mc.markedMessages[eventID]; !exists {
		mc.markedMessages[eventID] = time.Now()
		return true
	}
	return false
}

// IsMarked checks if a message was marked
func (mc *InMemoryMessageCache) IsMarked(eventID int64) bool {
	mc.mutex.RLock()
	_, marked := mc.markedMessages[eventID]
	mc.mutex.RUnlock()
	return marked
}

// Cleanup removes old marked message entries
func (mc *InMemoryMessageCache) Cleanup() int {
	counter := 0
	cutoff := time.Now().Add(-mc.ttl)
	mc.mutex.Lock()
	for eventID, deliveryTime := range mc.markedMessages {
		if deliveryTime.Before(cutoff) {
			delete(mc.markedMessages, eventID)
			counter++
		}
	}
	mc.mutex.Unlock()
	return counter
}

func (mc *InMemoryMessageCache) Len() int {
	mc.mutex.RLock()
	size := len(mc.markedMessages)
	mc.mutex.RUnlock()
	return size
}

// NoopMessageCache is a no-operation implementation of MessageCache
type NoopMessageCache struct{}

// NewNoopMessageCache creates a new no-operation message cache
func NewNoopMessageCache() MessageCache {
	return &NoopMessageCache{}
}

// Mark does nothing in the noop implementation
func (nc *NoopMessageCache) Mark(eventID int64) {
	// no-op
}

// MarkIfNotExists always returns true in the noop implementation
func (nc *NoopMessageCache) MarkIfNotExists(eventID int64) bool {
	return true
}

// IsMarked always returns false in the noop implementation
func (nc *NoopMessageCache) IsMarked(eventID int64) bool {
	return false
}

// Cleanup always returns 0 in the noop implementation
func (nc *NoopMessageCache) Cleanup() int {
	return 0
}

// Len always returns 0 in the noop implementation
func (nc *NoopMessageCache) Len() int {
	return 0
}
