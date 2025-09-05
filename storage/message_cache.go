package storage

import (
	"sync"
	"time"
)

// MessageCache tracks marked messages to avoid logging them as expired
type MessageCache struct {
	markedMessages map[int64]time.Time // event_id -> timestamp
	mutex          sync.RWMutex
	ttl            time.Duration
}

// NewMessageCache creates a new expired cache instance
func NewMessageCache(ttl time.Duration) *MessageCache {
	return &MessageCache{
		markedMessages: make(map[int64]time.Time),
		ttl:            ttl,
	}
}

// Mark message
func (mc *MessageCache) Mark(eventID int64) {
	mc.mutex.Lock()
	mc.markedMessages[eventID] = time.Now()
	mc.mutex.Unlock()
}

// Mark message
func (mc *MessageCache) MarkIfNotExists(eventID int64) bool {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()
	if _, exists := mc.markedMessages[eventID]; !exists {
		mc.markedMessages[eventID] = time.Now()
		return true
	}
	return false
}

// IsMarked checks if a message was marked
func (mc *MessageCache) IsMarked(eventID int64) bool {
	mc.mutex.RLock()
	_, marked := mc.markedMessages[eventID]
	mc.mutex.RUnlock()
	return marked
}

// Cleanup removes old marked message entries
func (mc *MessageCache) Cleanup() {
	cutoff := time.Now().Add(-mc.ttl)
	mc.mutex.Lock()
	for eventID, deliveryTime := range mc.markedMessages {
		if deliveryTime.Before(cutoff) {
			delete(mc.markedMessages, eventID)
		}
	}
	mc.mutex.Unlock()
}

// Global instance
var ExpiredCache = NewMessageCache(time.Hour)
var TransferedCache = NewMessageCache(time.Minute)
