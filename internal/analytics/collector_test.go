package analytics

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockAnalyticsClient is a mock implementation of tonmetrics.AnalyticsClient
type mockAnalyticsClient struct {
	batches   [][]interface{}
	mu        sync.Mutex
	sendErr   error
	sendDelay time.Duration
	callCount atomic.Int32
}

func (m *mockAnalyticsClient) SendBatch(ctx context.Context, events []interface{}) error {
	m.callCount.Add(1)
	if m.sendDelay > 0 {
		time.Sleep(m.sendDelay)
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches = append(m.batches, events)
	return nil
}

func (m *mockAnalyticsClient) getBatches() [][]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]interface{}, len(m.batches))
	copy(result, m.batches)
	return result
}

func (m *mockAnalyticsClient) totalEvents() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, batch := range m.batches {
		total += len(batch)
	}
	return total
}

func TestCollector_TryAdd_Basic(t *testing.T) {
	rc := NewCollector(3, nil, 0)

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

func TestCollector_TryAdd_DropNewest(t *testing.T) {
	rc := NewCollector(2, nil, 0)

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

func TestCollector_ZeroCapacity(t *testing.T) {
	rc := NewCollector(0, nil, 0)

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

func TestCollector_PopAll(t *testing.T) {
	rc := NewCollector(5, nil, 0)

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

func TestCollector_IsFull(t *testing.T) {
	rc := NewCollector(2, nil, 0)

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

func TestCollector_Dropped(t *testing.T) {
	rc := NewCollector(2, nil, 0)

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

func TestCollector_Concurrent(t *testing.T) {
	rc := NewCollector(1000, nil, 0)
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

	// PopAll should return at most 100 events per call when buffer has >= 100
	// So we need to call it multiple times to drain all 1000 events
	totalEvents := 0
	for {
		events := rc.PopAll()
		if events == nil {
			break
		}
		totalEvents += len(events)
		// Each batch should be at most 100 events
		if len(events) > 100 {
			t.Errorf("expected at most 100 events per batch, got %d", len(events))
		}
	}

	if totalEvents != numGoroutines*eventsPerGoroutine {
		t.Errorf("expected %d total events, got %d", numGoroutines*eventsPerGoroutine, totalEvents)
	}

	if rc.Dropped() != 0 {
		t.Errorf("expected no dropped events, got %d", rc.Dropped())
	}
}

func TestCollector_ConcurrentPopAndAdd(t *testing.T) {
	rc := NewCollector(100, nil, 0)
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

func TestCollector_TriggerCapacity(t *testing.T) {
	// For capacity > 10, triggerCapacity should be capacity - 10
	rc := NewCollector(20, nil, 0)

	// Add 9 events (below triggerCapacity of 10)
	for i := 0; i < 9; i++ {
		rc.TryAdd(i)
	}

	// Check that notifyCh is empty (no notification yet)
	select {
	case <-rc.notifyCh:
		t.Error("expected no notification before reaching triggerCapacity")
	default:
		// Expected - no notification
	}

	// Add event to reach triggerCapacity (10)
	rc.TryAdd(9)

	// Check that notifyCh has a notification
	select {
	case <-rc.notifyCh:
		// Expected - notification received
	default:
		t.Error("expected notification when reaching triggerCapacity")
	}
}

func TestCollector_TriggerCapacitySmallBuffer(t *testing.T) {
	// For capacity <= 10, triggerCapacity equals capacity
	rc := NewCollector(5, nil, 0)

	// Fill the buffer to capacity
	for i := 0; i < 5; i++ {
		rc.TryAdd(i)
	}

	// Check that notifyCh has a notification
	select {
	case <-rc.notifyCh:
		// Expected - notification received
	default:
		t.Error("expected notification when reaching triggerCapacity")
	}
}

func TestCollector_Flush(t *testing.T) {
	mock := &mockAnalyticsClient{}
	rc := NewCollector(10, mock, time.Second)

	// Add some events
	for i := 0; i < 5; i++ {
		rc.TryAdd(i)
	}

	// Flush manually
	ctx := context.Background()
	rc.Flush(ctx)

	// Check that events were sent
	batches := mock.getBatches()
	if len(batches) != 1 {
		t.Errorf("expected 1 batch, got %d", len(batches))
	}

	if len(batches[0]) != 5 {
		t.Errorf("expected 5 events in batch, got %d", len(batches[0]))
	}

	// Buffer should be empty now
	if rc.Len() != 0 {
		t.Errorf("expected buffer to be empty after flush, got %d", rc.Len())
	}
}

func TestCollector_FlushEmpty(t *testing.T) {
	mock := &mockAnalyticsClient{}
	rc := NewCollector(10, mock, time.Second)

	// Flush empty buffer
	ctx := context.Background()
	rc.Flush(ctx)

	// Check that no batches were sent
	batches := mock.getBatches()
	if len(batches) != 0 {
		t.Errorf("expected 0 batches for empty buffer, got %d", len(batches))
	}
}

func TestCollector_FlushWithError(t *testing.T) {
	mock := &mockAnalyticsClient{
		sendErr: context.DeadlineExceeded,
	}
	rc := NewCollector(10, mock, time.Second)

	// Add some events
	for i := 0; i < 5; i++ {
		rc.TryAdd(i)
	}

	// Flush should not panic even with error
	ctx := context.Background()
	rc.Flush(ctx)

	// Events should have been popped (even though send failed)
	if rc.Len() != 0 {
		t.Errorf("expected buffer to be empty after flush, got %d", rc.Len())
	}
}

func TestCollector_Run_PeriodicFlush(t *testing.T) {
	mock := &mockAnalyticsClient{}
	flushInterval := 50 * time.Millisecond
	rc := NewCollector(100, mock, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the collector in a goroutine
	done := make(chan struct{})
	go func() {
		rc.Run(ctx)
		close(done)
	}()

	// Add some events
	for i := 0; i < 5; i++ {
		rc.TryAdd(i)
	}

	// Wait for at least one flush interval
	time.Sleep(flushInterval * 2)

	// Check that events were flushed
	if mock.totalEvents() < 5 {
		t.Errorf("expected at least 5 events to be flushed, got %d", mock.totalEvents())
	}

	// Cancel and wait for Run to exit
	cancel()
	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Error("Run did not exit after context cancellation")
	}
}

func TestCollector_Run_TriggerCapacityFlush(t *testing.T) {
	mock := &mockAnalyticsClient{}
	// Use a long flush interval so we know flush is triggered by capacity, not timer
	flushInterval := 10 * time.Second
	capacity := 20
	rc := NewCollector(capacity, mock, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the collector in a goroutine
	go rc.Run(ctx)

	// Add events to reach triggerCapacity (capacity - 10 = 10)
	for i := 0; i < 10; i++ {
		rc.TryAdd(i)
	}

	// Wait a bit for the notification to be processed
	time.Sleep(100 * time.Millisecond)

	// Check that events were flushed due to trigger capacity
	if mock.totalEvents() < 10 {
		t.Errorf("expected at least 10 events to be flushed by trigger capacity, got %d", mock.totalEvents())
	}
}

func TestCollector_Run_FinalFlushOnCancel(t *testing.T) {
	mock := &mockAnalyticsClient{}
	flushInterval := 10 * time.Second // Long interval to ensure final flush is tested
	rc := NewCollector(100, mock, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the collector in a goroutine
	done := make(chan struct{})
	go func() {
		rc.Run(ctx)
		close(done)
	}()

	// Add some events
	for i := 0; i < 5; i++ {
		rc.TryAdd(i)
	}

	// Cancel immediately (before flush interval)
	cancel()

	// Wait for Run to exit
	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Error("Run did not exit after context cancellation")
	}

	// Check that final flush happened
	if mock.totalEvents() != 5 {
		t.Errorf("expected 5 events from final flush, got %d", mock.totalEvents())
	}
}

func TestCollector_Run_NoFlushWhenEmpty(t *testing.T) {
	mock := &mockAnalyticsClient{}
	flushInterval := 50 * time.Millisecond
	rc := NewCollector(100, mock, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the collector in a goroutine
	done := make(chan struct{})
	go func() {
		rc.Run(ctx)
		close(done)
	}()

	// Wait for a few flush intervals without adding events
	time.Sleep(flushInterval * 3)

	// Cancel and wait for Run to exit
	cancel()
	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Error("Run did not exit after context cancellation")
	}

	// Check that no batches were sent (buffer was always empty)
	if mock.totalEvents() != 0 {
		t.Errorf("expected 0 events (empty buffer), got %d", mock.totalEvents())
	}
}

func TestCollector_PopAllLimit100(t *testing.T) {
	rc := NewCollector(200, nil, 0)

	// Add 150 events
	for i := 0; i < 150; i++ {
		rc.TryAdd(i)
	}

	// First PopAll should return exactly 100 events
	events := rc.PopAll()
	if len(events) != 100 {
		t.Errorf("expected 100 events (limit), got %d", len(events))
	}

	// Second PopAll should return remaining 50 events
	events = rc.PopAll()
	if len(events) != 50 {
		t.Errorf("expected 50 remaining events, got %d", len(events))
	}

	// Third PopAll should return nil
	events = rc.PopAll()
	if events != nil {
		t.Errorf("expected nil for empty buffer, got %v", events)
	}
}

func TestNewCollector_TriggerCapacityCalculation(t *testing.T) {
	tests := []struct {
		name                    string
		capacity                int
		expectedTriggerCapacity int
	}{
		{"zero capacity", 0, 0},
		{"capacity 1", 1, 1},
		{"capacity 10", 10, 10},
		{"capacity 11", 11, 1},
		{"capacity 20", 20, 10},
		{"capacity 100", 100, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := NewCollector(tt.capacity, nil, 0)
			if rc.triggerCapacity != tt.expectedTriggerCapacity {
				t.Errorf("expected triggerCapacity %d, got %d", tt.expectedTriggerCapacity, rc.triggerCapacity)
			}
		})
	}
}
