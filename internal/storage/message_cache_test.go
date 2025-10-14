package storage

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMarkAndIsMarkedBasic(t *testing.T) {
	mc := NewMessageCache(true, time.Hour)

	// Initially not marked
	if mc.IsMarked(42) {
		t.Fatalf("expected not marked")
	}

	// Mark
	mc.Mark(42)

	// Now marked
	if !mc.IsMarked(42) {
		t.Fatalf("expected marked after Mark")
	}

	// MarkIfNotExists should return false now
	if ok := mc.MarkIfNotExists(42); ok {
		t.Fatalf("expected MarkIfNotExists to return false for existing id")
	}
}

func TestConcurrentMarkAndIsMarked(t *testing.T) {
	const total = 1000000
	mc := NewMessageCache(true, time.Hour)
	workers := runtime.NumCPU() * 4

	// First pass: ensure MarkIfNotExists returns true for each unique id and IsMarked true
	var successFirst int64
	var badFirst int64

	jobs := make(chan int64, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if mc.MarkIfNotExists(id) {
					atomic.AddInt64(&successFirst, 1)
				} else {
					atomic.AddInt64(&badFirst, 1)
				}
				if !mc.IsMarked(id) {
					atomic.AddInt64(&badFirst, 1)
				}
			}
		}()
	}

	for i := 0; i < total; i++ {
		jobs <- int64(i)
	}
	close(jobs)
	wg.Wait()

	if got := atomic.LoadInt64(&successFirst); got != total {
		t.Fatalf("expected %d successes in first pass, got %d (bad: %d)", total, got, atomic.LoadInt64(&badFirst))
	}
	if atomic.LoadInt64(&badFirst) != 0 {
		t.Fatalf("unexpected failures in first pass: %d", atomic.LoadInt64(&badFirst))
	}

	// Second pass: MarkIfNotExists should return false for all duplicate ids
	var duplicatesReported int64
	jobs2 := make(chan int64, workers)
	var wg2 sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for id := range jobs2 {
				if mc.MarkIfNotExists(id) {
					// unexpected true
					atomic.AddInt64(&duplicatesReported, 1)
				}
				// IsMarked must still be true
				if !mc.IsMarked(id) {
					atomic.AddInt64(&duplicatesReported, 1)
				}
			}
		}()
	}

	for i := 0; i < total; i++ {
		jobs2 <- int64(i)
	}
	close(jobs2)
	wg2.Wait()

	if got := atomic.LoadInt64(&duplicatesReported); got != 0 {
		t.Fatalf("expected 0 unexpected successes/failed checks in second pass, got %d", got)
	}
}

func TestCleanupRemovesOldEntries(t *testing.T) {
	ttl := 50 * time.Millisecond
	mc := NewMessageCache(true, ttl).(*InMemoryMessageCache)

	// populate entries: 10 old, 10 fresh
	mc.mutex.Lock()
	now := time.Now()
	for i := 0; i < 10; i++ {
		mc.markedMessages[int64(i)] = now.Add(-ttl * 10) // old
	}
	for i := 10; i < 20; i++ {
		mc.markedMessages[int64(i)] = now // fresh
	}
	mc.mutex.Unlock()

	removed := mc.Cleanup()
	if removed != 10 {
		t.Fatalf("expected removed 10 old entries, got %d", removed)
	}

	// ensure remaining are the fresh ones
	mc.mutex.RLock()
	remaining := len(mc.markedMessages)
	mc.mutex.RUnlock()
	if remaining != 10 {
		t.Fatalf("expected 10 remaining entries, got %d", remaining)
	}
	for i := 10; i < 20; i++ {
		if !mc.IsMarked(int64(i)) {
			t.Fatalf("expected id %d to remain marked", i)
		}
	}
}

func BenchmarkConcurrentMarkIsMarked(b *testing.B) {
	const total = 1000000
	workers := runtime.NumCPU() * 4

	for it := 0; it < b.N; it++ {
		mc := NewMessageCache(true, time.Second)
		var wg sync.WaitGroup
		jobs := make(chan int64, workers)

		start := time.Now()
		b.ResetTimer()

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for id := range jobs {
					_ = mc.MarkIfNotExists(id)
					_ = mc.IsMarked(id)
				}
			}()
		}

		for i := 0; i < total; i++ {
			jobs <- int64(i)
		}
		close(jobs)
		wg.Wait()

		b.StopTimer()
		_ = start // keep start if additional instrumentation needed in future
	}
}

func BenchmarkConcurrentCleaning(b *testing.B) {
	const total = 1000000
	workers := runtime.NumCPU() * 4

	for it := 0; it < b.N; it++ {
		mc := NewMessageCache(true, 100*time.Millisecond)
		var wg sync.WaitGroup
		jobs := make(chan int64, workers)

		b.ResetTimer()

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for id := range jobs {
					_ = mc.MarkIfNotExists(id)
					_ = mc.IsMarked(id)
				}
			}()
		}

		for i := 0; i < total; i++ {
			jobs <- int64(i)
		}
		close(jobs)
		wg.Wait()

		b.StopTimer()
		time.Sleep(time.Second)
		// runtime.GC()
		_ = mc.Cleanup()
		size := mc.Len()
		b.Logf("final cache size after wait: %d", size)
		if size != 0 {
			b.Fatalf("expected cache to be fully cleaned, got size %d", size)
		}

	}
}
