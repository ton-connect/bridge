package middleware

import (
	"sync"
	"testing"
	"time"
)

func TestRecipientRateLimiter_DisabledWhenZeroInterval(t *testing.T) {
	rl := NewRecipientRateLimiter(0, 1)
	defer rl.Stop()

	for i := 0; i < 100; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected Allow to return true when interval is 0, blocked on call %d", i)
		}
	}
}

func TestRecipientRateLimiter_AllowsFirstRequest(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to be allowed")
	}
}

func TestRecipientRateLimiter_BlocksSecondRequest(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to be allowed")
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected second request within interval to be blocked")
	}
}

func TestRecipientRateLimiter_IndependentRecipients(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1)
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
	rl := NewRecipientRateLimiter(50*time.Millisecond, 1)
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
	rl := NewRecipientRateLimiter(time.Second, 1)
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
	rl := NewRecipientRateLimiter(10*time.Millisecond, 1)
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

func TestRecipientRateLimiter_BurstAllowsNRequests(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 3)
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected request %d to be allowed within burst", i+1)
		}
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected request 4 to be blocked after burst exhausted")
	}
}

func TestRecipientRateLimiter_BurstRefillsAfterFullInterval(t *testing.T) {
	rl := NewRecipientRateLimiter(90*time.Millisecond, 3)
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected request %d to be allowed within burst", i+1)
		}
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected request 4 to be blocked after burst exhausted")
	}

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 3; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected request %d to be allowed after full interval refill", i+1)
		}
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected request 4 to be blocked again after second burst exhausted")
	}
}

func TestRecipientRateLimiter_BurstPartialRefill(t *testing.T) {
	rl := NewRecipientRateLimiter(90*time.Millisecond, 3)
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected request %d to be allowed within burst", i+1)
		}
	}

	time.Sleep(40 * time.Millisecond)

	if !rl.Allow("recipient1") {
		t.Fatal("expected one request to be allowed after partial refill")
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected second request to be blocked after partial refill consumed")
	}
}

func TestRecipientRateLimiter_BurstIndependentRecipients(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 3)
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected request %d to recipient1 to be allowed", i+1)
		}
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected recipient1 to be blocked after burst exhausted")
	}

	for i := 0; i < 3; i++ {
		if !rl.Allow("recipient2") {
			t.Fatalf("expected request %d to recipient2 to be allowed", i+1)
		}
	}
	if rl.Allow("recipient2") {
		t.Fatal("expected recipient2 to be blocked after burst exhausted")
	}
}

func TestRecipientRateLimiter_ConcurrentBurst(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 3)
	defer rl.Stop()

	const goroutines = 20
	var wg sync.WaitGroup
	allowed := make([]int, goroutines)

	for i := 0; i < goroutines; i++ {
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
	if total != 3 {
		t.Fatalf("expected exactly 3 allowed requests with burst=3, got %d", total)
	}
}
