package analytics

import (
	"sync"
	"sync/atomic"
)

// EventCollector is a non-blocking analytics producer API.
type EventCollector interface {
	// TryAdd attempts to enqueue the event. Returns true if enqueued, false if dropped.
	TryAdd(interface{}) bool
}

// RingCollector provides bounded, non-blocking storage for analytics events.
// When full, new events are dropped.
type RingCollector struct {
	mu       sync.Mutex
	events   []interface{}
	capacity int
	dropped  atomic.Uint64
}

// NewRingCollector builds a simple analytics collector with a capped slice.
// When the buffer is full, new events are dropped.
func NewRingCollector(capacity int) *RingCollector {
	return &RingCollector{
		events:   make([]interface{}, 0, capacity),
		capacity: capacity,
	}
}

// TryAdd enqueues without blocking. If full, returns false and increments drop count.
func (c *RingCollector) TryAdd(event interface{}) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.events) >= c.capacity {
		c.dropped.Add(1)
		return false
	}

	c.events = append(c.events, event)

	return true
}

// PopAll drains all pending events.
func (c *RingCollector) PopAll() []interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.events) == 0 {
		return nil
	}

	result := c.events
	c.events = make([]interface{}, 0, c.capacity)
	return result
}

// Dropped returns the number of events that were dropped due to buffer being full.
func (c *RingCollector) Dropped() uint64 {
	return c.dropped.Load()
}

// IsFull returns true if the buffer is at capacity.
func (c *RingCollector) IsFull() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events) >= c.capacity
}

// Len returns the current number of events in the buffer.
func (c *RingCollector) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}
