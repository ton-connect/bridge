package middleware

import (
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
type RecipientRateLimiter struct {
	mu        sync.Mutex
	limiters  map[string]*rateLimiterEntry
	interval  time.Duration
	cleanupCh chan struct{}
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRecipientRateLimiter creates a rate limiter that allows 1 push per interval per recipient.
// If interval is 0, rate limiting is disabled.
func NewRecipientRateLimiter(interval time.Duration) *RecipientRateLimiter {
	rl := &RecipientRateLimiter{
		limiters:  make(map[string]*rateLimiterEntry),
		interval:  interval,
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

	entry, exists := rl.limiters[to]
	if !exists {
		// 1 event per interval, burst of 1
		lim := rate.NewLimiter(rate.Every(rl.interval), 1)
		entry = &rateLimiterEntry{limiter: lim, lastSeen: time.Now()}
		rl.limiters[to] = entry
	}
	entry.lastSeen = time.Now()

	if !entry.limiter.Allow() {
		recipientRateLimitedMetric.Inc()
		logrus.WithField("to", to).Warn("Message rate limited for recipient")
		return false
	}
	return true
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
			for key, entry := range rl.limiters {
				if entry.lastSeen.Before(cutoff) {
					delete(rl.limiters, key)
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
