package analytics

import "sync"

// EventCollector is a non-blocking analytics producer API.
type EventCollector interface {
	// TryAdd attempts to enqueue the event. Returns true if enqueued, false if dropped.
	TryAdd(interface{}) bool
}

// RingBuffer provides bounded, non-blocking storage for analytics events.
// Writes never block; when full, events are dropped per policy.
type RingBuffer struct {
	mu         sync.Mutex
	events     []interface{}
	head       int
	size       int
	capacity   int
	dropOldest bool
	dropped    uint64
}

// NewRingBuffer constructs a bounded ring buffer.
// If dropOldest is true, the oldest event is overwritten when full; otherwise new events are dropped.
func NewRingBuffer(capacity int, dropOldest bool) *RingBuffer {
	return &RingBuffer{
		events:     make([]interface{}, capacity),
		capacity:   capacity,
		dropOldest: dropOldest,
	}
}

// add inserts event into the buffer according to the drop policy.
// Returns true if the event was stored.
func (r *RingBuffer) add(event interface{}) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.capacity == 0 {
		r.dropped++
		return false
	}

	if r.size == r.capacity {
		if !r.dropOldest {
			r.dropped++
			return false
		}
		// Drop the oldest by moving head forward and shrinking size.
		r.head = (r.head + 1) % r.capacity
		r.size--
	}

	tail := (r.head + r.size) % r.capacity
	r.events[tail] = event
	r.size++
	return true
}

// popAll drains the buffer into a new slice.
func (r *RingBuffer) popAll() []interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.size == 0 {
		return nil
	}

	result := make([]interface{}, 0, r.size)
	for r.size > 0 {
		result = append(result, r.events[r.head])
		r.head = (r.head + 1) % r.capacity
		r.size--
	}
	return result
}

// droppedCount returns the number of events that were not enqueued.
func (r *RingBuffer) droppedCount() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

// RingCollector wraps a RingBuffer with a notify channel for collectors.
type RingCollector struct {
	buffer *RingBuffer
	notify chan struct{}
}

// NewRingCollector builds an analytics collector around a ring buffer.
func NewRingCollector(capacity int, dropOldest bool) *RingCollector {
	return &RingCollector{
		buffer: NewRingBuffer(capacity, dropOldest),
		notify: make(chan struct{}, 1),
	}
}

// TryAdd enqueues without blocking. If full, returns false and increments drop count.
func (e *RingCollector) TryAdd(event interface{}) bool {
	added := e.buffer.add(event)
	if added {
		select {
		case e.notify <- struct{}{}:
		default:
		}
	}
	return added
}

// PopAll drains all pending events.
func (e *RingCollector) PopAll() []interface{} {
	return e.buffer.popAll()
}

// Notify returns a channel signaled when new events arrive.
func (e *RingCollector) Notify() <-chan struct{} {
	return e.notify
}

// Dropped returns the number of enqueued events that were dropped.
func (e *RingCollector) Dropped() uint64 {
	return e.buffer.droppedCount()
}
