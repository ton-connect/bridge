package analytics

import (
	"sync"
	"testing"
)

func TestRingCollector_TryAdd_Basic(t *testing.T) {
	rc := NewRingCollector(3)

	// First add should succeed
	if !rc.TryAdd("event1") {
		t.Error("expected first TryAdd to succeed")
	}

	if rc.Len() != 1 {
		t.Errorf("expected length 1, got %d", rc.Len())
	}

	// Second add should succeed
	if !rc.TryAdd("event2") {
		t.Error("expected second TryAdd to succeed")
	}

	if rc.Len() != 2 {
		t.Errorf("expected length 2, got %d", rc.Len())
	}

	// Third add should succeed (buffer full)
	if !rc.TryAdd("event3") {
		t.Error("expected third TryAdd to succeed")
	}

	if rc.Len() != 3 {
		t.Errorf("expected length 3, got %d", rc.Len())
	}

	// Fourth add should fail (buffer full, drop newest)
	if rc.TryAdd("event4") {
		t.Error("expected fourth TryAdd to fail (buffer full, drop newest)")
	}

	if rc.Len() != 3 {
		t.Errorf("expected length to remain 3, got %d", rc.Len())
	}

	if rc.Dropped() != 1 {
		t.Errorf("expected dropped count 1, got %d", rc.Dropped())
	}

	// Verify the events in buffer
	events := rc.PopAll()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	expected := []interface{}{"event1", "event2", "event3"}
	for i, event := range events {
		if event != expected[i] {
			t.Errorf("expected event %v at position %d, got %v", expected[i], i, event)
		}
	}
}

func TestRingCollector_TryAdd_DropNewest(t *testing.T) {
	rc := NewRingCollector(2)

	// Fill the buffer
	rc.TryAdd("event1")
	rc.TryAdd("event2")

	// These should be dropped (buffer full)
	if rc.TryAdd("event3") {
		t.Error("expected third TryAdd to fail (buffer full)")
	}

	if rc.TryAdd("event4") {
		t.Error("expected fourth TryAdd to fail (buffer full)")
	}

	if rc.Dropped() != 2 {
		t.Errorf("expected dropped count 2, got %d", rc.Dropped())
	}

	// Verify only first two events are in buffer
	events := rc.PopAll()
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	expected := []interface{}{"event1", "event2"}
	for i, event := range events {
		if event != expected[i] {
			t.Errorf("expected event %v at position %d, got %v", expected[i], i, event)
		}
	}
}

func TestRingCollector_ZeroCapacity(t *testing.T) {
	rc := NewRingCollector(0)

	// All adds should fail
	if rc.TryAdd("event1") {
		t.Error("expected TryAdd to fail with zero capacity")
	}

	if rc.Dropped() != 1 {
		t.Errorf("expected dropped count 1, got %d", rc.Dropped())
	}

	if rc.TryAdd("event2") {
		t.Error("expected TryAdd to fail with zero capacity")
	}

	if rc.Dropped() != 2 {
		t.Errorf("expected dropped count 2, got %d", rc.Dropped())
	}

	events := rc.PopAll()
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}
}

func TestRingCollector_PopAll(t *testing.T) {
	rc := NewRingCollector(5)

	// Add events
	for i := 1; i <= 5; i++ {
		rc.TryAdd(i)
	}

	// Pop all and verify order
	events := rc.PopAll()
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}

	for i, event := range events {
		if event.(int) != i+1 {
			t.Errorf("expected event %d at position %d, got %v", i+1, i, event)
		}
	}

	// Buffer should be empty now
	if rc.Len() != 0 {
		t.Errorf("expected length 0 after PopAll, got %d", rc.Len())
	}

	// Second PopAll should return nil
	events = rc.PopAll()
	if events != nil {
		t.Errorf("expected nil for second PopAll, got %v", events)
	}
}

func TestRingCollector_IsFull(t *testing.T) {
	rc := NewRingCollector(2)

	if rc.IsFull() {
		t.Error("expected buffer not to be full initially")
	}

	rc.TryAdd("event1")
	if rc.IsFull() {
		t.Error("expected buffer not to be full with 1 event")
	}

	rc.TryAdd("event2")
	if !rc.IsFull() {
		t.Error("expected buffer to be full with 2 events")
	}

	// Pop all and check again
	rc.PopAll()
	if rc.IsFull() {
		t.Error("expected buffer not to be full after PopAll")
	}
}

func TestRingCollector_Dropped(t *testing.T) {
	rc := NewRingCollector(2)

	if rc.Dropped() != 0 {
		t.Errorf("expected initial dropped count 0, got %d", rc.Dropped())
	}

	rc.TryAdd("event1")
	rc.TryAdd("event2")

	if rc.Dropped() != 0 {
		t.Errorf("expected dropped count 0 after filling buffer, got %d", rc.Dropped())
	}

	// These should be dropped
	rc.TryAdd("event3")
	rc.TryAdd("event4")

	if rc.Dropped() != 2 {
		t.Errorf("expected dropped count 2, got %d", rc.Dropped())
	}
}

func TestRingCollector_Concurrent(t *testing.T) {
	rc := NewRingCollector(1000)
	var wg sync.WaitGroup

	// Spawn multiple goroutines to add events
	numGoroutines := 10
	eventsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				rc.TryAdd(id*1000 + j)
			}
		}(i)
	}

	wg.Wait()

	// All events should be added
	events := rc.PopAll()
	if len(events) != numGoroutines*eventsPerGoroutine {
		t.Errorf("expected %d events, got %d", numGoroutines*eventsPerGoroutine, len(events))
	}

	if rc.Dropped() != 0 {
		t.Errorf("expected no dropped events, got %d", rc.Dropped())
	}
}

func TestRingCollector_ConcurrentPopAndAdd(t *testing.T) {
	rc := NewRingCollector(100)
	var wg sync.WaitGroup

	// Goroutine adding events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			rc.TryAdd(i)
		}
	}()

	// Goroutine popping events
	wg.Add(1)
	totalPopped := 0
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			events := rc.PopAll()
			totalPopped += len(events)
		}
	}()

	wg.Wait()

	// Final pop to get remaining events
	events := rc.PopAll()
	totalPopped += len(events)

	// We should have received all 200 events (either popped or dropped)
	totalReceived := totalPopped + int(rc.Dropped())
	if totalReceived != 200 {
		t.Errorf("expected 200 total events (popped + dropped), got %d", totalReceived)
	}
}
