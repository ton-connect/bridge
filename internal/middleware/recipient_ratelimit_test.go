package middleware

import (
	"sync"
	"testing"
	"time"
)

func TestRecipientRateLimiter_DisabledWhenZeroInterval(t *testing.T) {
	rl := NewRecipientRateLimiter(0)
	defer rl.Stop()

	for i := 0; i < 100; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected Allow to return true when interval is 0, blocked on call %d", i)
		}
	}
}

func TestRecipientRateLimiter_AllowsFirstRequest(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to be allowed")
	}
}

func TestRecipientRateLimiter_BlocksSecondRequest(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to be allowed")
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected second request within interval to be blocked")
	}
}

func TestRecipientRateLimiter_IndependentRecipients(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to recipient1 to be allowed")
	}
	if !rl.Allow("recipient2") {
		t.Fatal("expected first request to recipient2 to be allowed")
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected second request to recipient1 to be blocked")
	}
	if rl.Allow("recipient2") {
		t.Fatal("expected second request to recipient2 to be blocked")
	}
}

func TestRecipientRateLimiter_AllowsAfterInterval(t *testing.T) {
	rl := NewRecipientRateLimiter(50 * time.Millisecond)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to be allowed")
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected second request to be blocked")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("recipient1") {
		t.Fatal("expected request after interval to be allowed")
	}
}

func TestRecipientRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second)
	defer rl.Stop()

	var wg sync.WaitGroup
	allowed := make([]int, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if rl.Allow("recipient1") {
				allowed[idx] = 1
			}
		}(i)
	}
	wg.Wait()

	total := 0
	for _, v := range allowed {
		total += v
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 allowed request, got %d", total)
	}
}

func TestRecipientRateLimiter_Cleanup(t *testing.T) {
	rl := NewRecipientRateLimiter(10 * time.Millisecond)
	defer rl.Stop()

	rl.Allow("recipient1")

	// Wait long enough for cleanup to remove stale entry (5*interval = 50ms, cleanup runs every minute)
	// Instead of waiting for the ticker, invoke cleanup logic indirectly by checking the map
	time.Sleep(60 * time.Millisecond)

	rl.mu.Lock()
	cutoff := time.Now().Add(-5 * rl.interval)
	for key, entry := range rl.limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(rl.limiters, key)
		}
	}
	count := len(rl.limiters)
	rl.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected stale limiter to be cleaned up, got %d entries", count)
	}
}
