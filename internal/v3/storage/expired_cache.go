package storagev3

import (
	"sync"
	"time"
)

// ExpiredCache tracks delivered messages to avoid logging them as expired
type ExpiredCache struct {
	deliveredMessages map[int64]time.Time // event_id -> delivery timestamp
	mutex             sync.RWMutex
}

// NewExpiredCache creates a new expired cache instance
func NewExpiredCache() *ExpiredCache {
	return &ExpiredCache{
		deliveredMessages: make(map[int64]time.Time),
	}
}

// MarkDelivered marks a message as delivered
func (ec *ExpiredCache) MarkDelivered(eventID int64) {
	ec.mutex.Lock()
	ec.deliveredMessages[eventID] = time.Now()
	ec.mutex.Unlock()
}

// IsDelivered checks if a message was delivered
func (ec *ExpiredCache) IsDelivered(eventID int64) bool {
	ec.mutex.RLock()
	_, delivered := ec.deliveredMessages[eventID]
	ec.mutex.RUnlock()
	return delivered
}

// Cleanup removes old delivered message entries (older than 1 hour)
func (ec *ExpiredCache) Cleanup() {
	cutoff := time.Now().Add(-time.Hour)
	ec.mutex.Lock()
	for eventID, deliveryTime := range ec.deliveredMessages {
		if deliveryTime.Before(cutoff) {
			delete(ec.deliveredMessages, eventID)
		}
	}
	ec.mutex.Unlock()
}

// Global instance
var GlobalExpiredCache = NewExpiredCache()
