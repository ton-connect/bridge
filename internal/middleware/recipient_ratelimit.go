package middleware

import (
	"container/list"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

var recipientRateLimitedMetric = promauto.NewCounter(prometheus.CounterOpts{
	Name: "number_of_recipient_rate_limited_messages",
	Help: "The total number of messages dropped by per-recipient rate limiting",
})

// RecipientRateLimiter enforces a per-recipient (to) push rate limit using a leaky bucket.
// When maxSize > 0, it uses LRU eviction to bound memory usage.
type RecipientRateLimiter struct {
	mu        sync.Mutex
	limiters  map[string]*list.Element
	evictList *list.List
	maxSize   int
	interval  time.Duration
	burst     int
	cleanupCh chan struct{}
}

type rateLimiterEntry struct {
	key      string
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRecipientRateLimiter creates a rate limiter that allows rpi pushes per interval per recipient.
// maxSize bounds the number of tracked recipients; 0 means unlimited.
// If interval is 0, rate limiting is disabled.
func NewRecipientRateLimiter(interval time.Duration, rpi int, maxSize int) *RecipientRateLimiter {
	if rpi < 1 {
		rpi = 1
	}
	rl := &RecipientRateLimiter{
		limiters:  make(map[string]*list.Element),
		evictList: list.New(),
		maxSize:   maxSize,
		interval:  interval,
		burst:     rpi,
		cleanupCh: make(chan struct{}),
	}
	if interval > 0 {
		go rl.cleanup()
	}
	return rl
}

// Allow checks if a push to the given recipient is allowed.
// Returns true if allowed, false if rate limited.
func (rl *RecipientRateLimiter) Allow(to string) bool {
	if rl.interval <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	elem, exists := rl.limiters[to]
	if !exists {
		// Evict oldest if at capacity
		if rl.maxSize > 0 && rl.evictList.Len() >= rl.maxSize {
			rl.removeOldest()
		}
		lim := rate.NewLimiter(rate.Every(rl.interval/time.Duration(rl.burst)), rl.burst)
		entry := &rateLimiterEntry{key: to, limiter: lim, lastSeen: time.Now()}
		elem = rl.evictList.PushFront(entry)
		rl.limiters[to] = elem
	} else {
		rl.evictList.MoveToFront(elem)
	}

	entry := elem.Value.(*rateLimiterEntry)
	entry.lastSeen = time.Now()

	if !entry.limiter.Allow() {
		recipientRateLimitedMetric.Inc()
		logrus.WithField("to", to).Warn("Message rate limited for recipient")
		return false
	}
	return true
}

// removeOldest evicts the least recently used entry. Must be called with mu held.
func (rl *RecipientRateLimiter) removeOldest() {
	elem := rl.evictList.Back()
	if elem == nil {
		return
	}
	rl.removeElement(elem)
}

// removeElement removes a specific element from the cache. Must be called with mu held.
func (rl *RecipientRateLimiter) removeElement(elem *list.Element) {
	rl.evictList.Remove(elem)
	entry := elem.Value.(*rateLimiterEntry)
	delete(rl.limiters, entry.key)
}

// cleanup periodically removes stale limiters for recipients that haven't been seen recently.
func (rl *RecipientRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-5 * rl.interval)
			// Iterate from back (oldest) and remove stale entries
			for {
				elem := rl.evictList.Back()
				if elem == nil {
					break
				}
				entry := elem.Value.(*rateLimiterEntry)
				if entry.lastSeen.Before(cutoff) {
					rl.removeElement(elem)
				} else {
					break
				}
			}
			rl.mu.Unlock()
		case <-rl.cleanupCh:
			return
		}
	}
}

// Stop terminates the background cleanup goroutine.
func (rl *RecipientRateLimiter) Stop() {
	close(rl.cleanupCh)
}
