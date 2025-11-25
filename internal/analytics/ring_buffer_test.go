package analytics

import (
	"sync"
	"testing"
)

func TestRingBuffer_Add_DropNewest(t *testing.T) {
	rb := NewRingBuffer(3, false)

	// Add first event
	if !rb.add("event1") {
		t.Error("expected first add to succeed")
	}
	if rb.size != 1 {
		t.Errorf("expected size 1, got %d", rb.size)
	}

	// Add second event
	if !rb.add("event2") {
		t.Error("expected second add to succeed")
	}
	if rb.size != 2 {
		t.Errorf("expected size 2, got %d", rb.size)
	}

	// Add third event (buffer full)
	if !rb.add("event3") {
		t.Error("expected third add to succeed")
	}
	if rb.size != 3 {
		t.Errorf("expected size 3, got %d", rb.size)
	}

	// Add fourth event (should be dropped, buffer full)
	if rb.add("event4") {
		t.Error("expected fourth add to fail (buffer full, drop newest)")
	}
	if rb.size != 3 {
		t.Errorf("expected size to remain 3, got %d", rb.size)
	}
	if rb.dropped != 1 {
		t.Errorf("expected dropped count 1, got %d", rb.dropped)
	}

	// Verify the events in buffer
	events := rb.popAll()
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

func TestRingBuffer_Add_DropOldest(t *testing.T) {
	rb := NewRingBuffer(3, true)

	// Fill the buffer
	rb.add("event1")
	rb.add("event2")
	rb.add("event3")

	// Add fourth event (should overwrite oldest)
	if !rb.add("event4") {
		t.Error("expected fourth add to succeed (drop oldest)")
	}
	if rb.size != 3 {
		t.Errorf("expected size to remain 3, got %d", rb.size)
	}
	if rb.dropped != 0 {
		t.Errorf("expected dropped count 0 (we don't count overwrites), got %d", rb.dropped)
	}

	// Verify the events in buffer (event1 should be gone)
	events := rb.popAll()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
	expected := []interface{}{"event2", "event3", "event4"}
	for i, event := range events {
		if event != expected[i] {
			t.Errorf("expected event %v at position %d, got %v", expected[i], i, event)
		}
	}
}

func TestRingBuffer_Add_ZeroCapacity(t *testing.T) {
	rb := NewRingBuffer(0, false)

	// All adds should fail
	if rb.add("event1") {
		t.Error("expected add to fail with zero capacity")
	}
	if rb.dropped != 1 {
		t.Errorf("expected dropped count 1, got %d", rb.dropped)
	}

	if rb.add("event2") {
		t.Error("expected add to fail with zero capacity")
	}
	if rb.dropped != 2 {
		t.Errorf("expected dropped count 2, got %d", rb.dropped)
	}

	events := rb.popAll()
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}
}

func TestRingBuffer_PopAll_Order(t *testing.T) {
	rb := NewRingBuffer(5, false)

	// Add events
	for i := 1; i <= 5; i++ {
		rb.add(i)
	}

	// Pop all and verify order
	events := rb.popAll()
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}
	for i, event := range events {
		if event.(int) != i+1 {
			t.Errorf("expected event %d at position %d, got %v", i+1, i, event)
		}
	}

	// Buffer should be empty now
	if rb.size != 0 {
		t.Errorf("expected size 0 after popAll, got %d", rb.size)
	}

	// Second popAll should return nil
	events = rb.popAll()
	if events != nil {
		t.Errorf("expected nil for second popAll, got %v", events)
	}
}

func TestRingBuffer_PopAll_WithWrap(t *testing.T) {
	rb := NewRingBuffer(3, true)

	// Fill buffer
	rb.add("event1")
	rb.add("event2")
	rb.add("event3")

	// Cause wrap by adding more
	rb.add("event4") // overwrites event1
	rb.add("event5") // overwrites event2

	events := rb.popAll()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	expected := []interface{}{"event3", "event4", "event5"}
	for i, event := range events {
		if event != expected[i] {
			t.Errorf("expected event %v at position %d, got %v", expected[i], i, event)
		}
	}
}

func TestRingBuffer_DroppedCount(t *testing.T) {
	rb := NewRingBuffer(2, false)

	if rb.droppedCount() != 0 {
		t.Errorf("expected initial dropped count 0, got %d", rb.droppedCount())
	}

	rb.add("event1")
	rb.add("event2")

	// These should be dropped
	rb.add("event3")
	rb.add("event4")

	if rb.droppedCount() != 2 {
		t.Errorf("expected dropped count 2, got %d", rb.droppedCount())
	}
}

func TestRingBuffer_Concurrent(t *testing.T) {
	rb := NewRingBuffer(1000, false)
	var wg sync.WaitGroup

	// Spawn multiple goroutines to add events
	numGoroutines := 10
	eventsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				rb.add(id*1000 + j)
			}
		}(i)
	}

	wg.Wait()

	// All events should be added
	if rb.size != numGoroutines*eventsPerGoroutine {
		t.Errorf("expected size %d, got %d", numGoroutines*eventsPerGoroutine, rb.size)
	}
	if rb.dropped != 0 {
		t.Errorf("expected no dropped events, got %d", rb.dropped)
	}

	events := rb.popAll()
	if len(events) != numGoroutines*eventsPerGoroutine {
		t.Errorf("expected %d events, got %d", numGoroutines*eventsPerGoroutine, len(events))
	}
}

func TestRingCollector_TryAdd(t *testing.T) {
	rc := NewRingCollector(2, false)

	// First add should succeed
	if !rc.TryAdd("event1") {
		t.Error("expected first TryAdd to succeed")
	}

	// Check notification
	select {
	case <-rc.Notify():
		// Good, we got notified
	default:
		t.Error("expected notification after add")
	}

	// Second add should succeed
	if !rc.TryAdd("event2") {
		t.Error("expected second TryAdd to succeed")
	}

	// Third add should fail (buffer full, drop newest)
	if rc.TryAdd("event3") {
		t.Error("expected third TryAdd to fail")
	}

	if rc.Dropped() != 1 {
		t.Errorf("expected dropped count 1, got %d", rc.Dropped())
	}
}

func TestRingCollector_PopAll(t *testing.T) {
	rc := NewRingCollector(5, false)

	rc.TryAdd("event1")
	rc.TryAdd("event2")
	rc.TryAdd("event3")

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

	// Second PopAll should return nil
	events = rc.PopAll()
	if events != nil {
		t.Errorf("expected nil for second PopAll, got %v", events)
	}
}

func TestRingCollector_Dropped(t *testing.T) {
	rc := NewRingCollector(2, false)

	if rc.Dropped() != 0 {
		t.Errorf("expected initial dropped count 0, got %d", rc.Dropped())
	}

	rc.TryAdd("event1")
	rc.TryAdd("event2")
	rc.TryAdd("event3") // should be dropped
	rc.TryAdd("event4") // should be dropped

	if rc.Dropped() != 2 {
		t.Errorf("expected dropped count 2, got %d", rc.Dropped())
	}
}

func TestRingCollector_Concurrent(t *testing.T) {
	rc := NewRingCollector(1000, false)
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

	events := rc.PopAll()
	if len(events) != numGoroutines*eventsPerGoroutine {
		t.Errorf("expected %d events, got %d", numGoroutines*eventsPerGoroutine, len(events))
	}
	if rc.Dropped() != 0 {
		t.Errorf("expected no dropped events, got %d", rc.Dropped())
	}
}
