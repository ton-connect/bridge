package storage

import (
	"sync"
	"time"
)

// MessageCache tracks marked messages to avoid logging them as expired
type MessageCache struct {
	markedMessages map[int64]time.Time // event_id -> timestamp
	mutex          sync.RWMutex
}

// NewMessageCache creates a new expired cache instance
func NewMessageCache() *MessageCache {
	return &MessageCache{
		markedMessages: make(map[int64]time.Time),
	}
}

// Mark message
func (ec *MessageCache) Mark(eventID int64) {
	ec.mutex.Lock()
	ec.markedMessages[eventID] = time.Now()
	ec.mutex.Unlock()
}

// Mark message
func (ec *MessageCache) MarkIfNotExists(eventID int64) bool {
	ec.mutex.Lock()
	defer ec.mutex.Unlock()
	if _, exists := ec.markedMessages[eventID]; !exists {
		ec.markedMessages[eventID] = time.Now()
		return true
	}
	return false
}

// IsMarked checks if a message was marked
func (ec *MessageCache) IsMarked(eventID int64) bool {
	ec.mutex.RLock()
	_, marked := ec.markedMessages[eventID]
	ec.mutex.RUnlock()
	return marked
}

// Cleanup removes old marked message entries (older than 1 hour)
func (ec *MessageCache) Cleanup() {
	cutoff := time.Now().Add(-time.Hour)
	ec.mutex.Lock()
	for eventID, deliveryTime := range ec.markedMessages {
		if deliveryTime.Before(cutoff) {
			delete(ec.markedMessages, eventID)
		}
	}
	ec.mutex.Unlock()
}

// Global instance
var ExpiredCache = NewMessageCache()
var TransferedCache = NewMessageCache()
