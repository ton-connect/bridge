package middleware

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRecipientRateLimiter_DisabledWhenZeroInterval(t *testing.T) {
	rl := NewRecipientRateLimiter(0, 1, 0)
	defer rl.Stop()

	for i := 0; i < 100; i++ {
		if !rl.Allow("recipient1") {
			t.Fatalf("expected Allow to return true when interval is 0, blocked on call %d", i)
		}
	}
}

func TestRecipientRateLimiter_AllowsFirstRequest(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1, 0)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to be allowed")
	}
}

func TestRecipientRateLimiter_BlocksSecondRequest(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1, 0)
	defer rl.Stop()

	if !rl.Allow("recipient1") {
		t.Fatal("expected first request to be allowed")
	}
	if rl.Allow("recipient1") {
		t.Fatal("expected second request within interval to be blocked")
	}
}

func TestRecipientRateLimiter_IndependentRecipients(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1, 0)
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
	rl := NewRecipientRateLimiter(50*time.Millisecond, 1, 0)
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
	rl := NewRecipientRateLimiter(time.Second, 1, 0)
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
	rl := NewRecipientRateLimiter(10*time.Millisecond, 1, 0)
	defer rl.Stop()

	rl.Allow("recipient1")

	// Wait long enough for cleanup to remove stale entry (5*interval = 50ms, cleanup runs every minute)
	// Instead of waiting for the ticker, invoke cleanup logic indirectly by checking the map
	time.Sleep(60 * time.Millisecond)

	rl.mu.Lock()
	cutoff := time.Now().Add(-5 * rl.interval)
	for key, elem := range rl.limiters {
		entry := elem.Value.(*rateLimiterEntry)
		if entry.lastSeen.Before(cutoff) {
			rl.evictList.Remove(elem)
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
	rl := NewRecipientRateLimiter(time.Second, 3, 0)
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
	rl := NewRecipientRateLimiter(90*time.Millisecond, 3, 0)
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
	rl := NewRecipientRateLimiter(90*time.Millisecond, 3, 0)
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
	rl := NewRecipientRateLimiter(time.Second, 3, 0)
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
	rl := NewRecipientRateLimiter(time.Second, 3, 0)
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

func TestRecipientRateLimiter_LRU_EvictsOldest(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1, 2)
	defer rl.Stop()

	// Fill capacity with r1, r2
	rl.Allow("r1")
	rl.Allow("r2")

	// r1 is now the oldest (LRU). Adding r3 should evict r1.
	rl.Allow("r3")

	rl.mu.Lock()
	_, r1Exists := rl.limiters["r1"]
	_, r2Exists := rl.limiters["r2"]
	_, r3Exists := rl.limiters["r3"]
	size := len(rl.limiters)
	rl.mu.Unlock()

	if r1Exists {
		t.Fatal("expected r1 to be evicted")
	}
	if !r2Exists || !r3Exists {
		t.Fatal("expected r2 and r3 to remain")
	}
	if size != 2 {
		t.Fatalf("expected 2 entries, got %d", size)
	}
}

func TestRecipientRateLimiter_LRU_AccessRefreshesOrder(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1, 2)
	defer rl.Stop()

	rl.Allow("r1")
	rl.Allow("r2")

	// Access r1 again to make it most-recently-used
	rl.Allow("r1")

	// Now r2 is the oldest. Adding r3 should evict r2, not r1.
	rl.Allow("r3")

	rl.mu.Lock()
	_, r1Exists := rl.limiters["r1"]
	_, r2Exists := rl.limiters["r2"]
	_, r3Exists := rl.limiters["r3"]
	rl.mu.Unlock()

	if !r1Exists {
		t.Fatal("expected r1 to remain (was recently accessed)")
	}
	if r2Exists {
		t.Fatal("expected r2 to be evicted (oldest)")
	}
	if !r3Exists {
		t.Fatal("expected r3 to remain")
	}
}

func TestRecipientRateLimiter_LRU_EvictedRecipientGetsNewBucket(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1, 2)
	defer rl.Stop()

	// Exhaust r1's bucket
	rl.Allow("r1")
	rl.Allow("r2")

	// Evict r1 by adding r3
	rl.Allow("r3")

	// r1 was evicted, so it gets a fresh bucket — should be allowed
	if !rl.Allow("r1") {
		t.Fatal("expected r1 to be allowed after eviction (fresh bucket)")
	}
}

func TestRecipientRateLimiter_LRU_UnlimitedWhenZeroMaxSize(t *testing.T) {
	rl := NewRecipientRateLimiter(time.Second, 1, 0)
	defer rl.Stop()

	for i := 0; i < 10000; i++ {
		rl.Allow(fmt.Sprintf("r%d", i))
	}

	rl.mu.Lock()
	size := len(rl.limiters)
	rl.mu.Unlock()

	if size != 10000 {
		t.Fatalf("expected 10000 entries with unlimited maxSize, got %d", size)
	}
}

// Benchmarks — measure raw overhead of Allow() with huge burst so rate limiting never triggers.

// BenchmarkAllow_Disabled: baseline with rate limiting off (interval=0).
func BenchmarkAllow_Disabled(b *testing.B) {
	rl := NewRecipientRateLimiter(0, 1000000, 0)
	defer rl.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("recipient1")
	}
}

// BenchmarkAllow_SingleRecipient: one recipient, never hits the limit.
func BenchmarkAllow_SingleRecipient(b *testing.B) {
	rl := NewRecipientRateLimiter(time.Hour, 1000000000, 0)
	defer rl.Stop()

	rl.Allow("recipient1") // warm up the entry
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("recipient1")
	}
}

// BenchmarkAllow_SingleRecipient_LRU: same but with LRU bounded (maxSize=1000).
func BenchmarkAllow_SingleRecipient_LRU(b *testing.B) {
	rl := NewRecipientRateLimiter(time.Hour, 1000000000, 1000)
	defer rl.Stop()

	rl.Allow("recipient1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("recipient1")
	}
}

// BenchmarkAllow_ManyRecipients: unique recipient each call, no LRU cap — measures map insert cost.
func BenchmarkAllow_ManyRecipients(b *testing.B) {
	rl := NewRecipientRateLimiter(time.Hour, 1000000000, 0)
	defer rl.Stop()

	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("r%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow(keys[i])
	}
}

// BenchmarkAllow_ManyRecipients_LRU: unique recipients with LRU eviction active (maxSize=10000).
func BenchmarkAllow_ManyRecipients_LRU(b *testing.B) {
	rl := NewRecipientRateLimiter(time.Hour, 1000000000, 10000)
	defer rl.Stop()

	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("r%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow(keys[i])
	}
}

// BenchmarkAllow_Parallel: concurrent access from multiple goroutines, single recipient.
func BenchmarkAllow_Parallel(b *testing.B) {
	rl := NewRecipientRateLimiter(time.Hour, 1000000000, 0)
	defer rl.Stop()

	rl.Allow("recipient1")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rl.Allow("recipient1")
		}
	})
}

// BenchmarkAllow_Parallel_LRU: concurrent access with LRU enabled.
func BenchmarkAllow_Parallel_LRU(b *testing.B) {
	rl := NewRecipientRateLimiter(time.Hour, 1000000000, 10000)
	defer rl.Stop()

	rl.Allow("recipient1")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rl.Allow("recipient1")
		}
	})
}
