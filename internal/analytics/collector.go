package analytics

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/tonmetrics"
)

// EventCollector is a non-blocking analytics producer API.
type EventCollector interface {
	// TryAdd attempts to enqueue the event. Returns true if enqueued, false if dropped.
	TryAdd(interface{}) bool
}

// Collector provides bounded, non-blocking storage for analytics events and
// periodically flushes them to a backend. When buffer is full, new events are dropped.
type Collector struct {
	// Buffer fields
	mu       sync.Mutex
	events   []interface{}
	capacity int
	dropped  atomic.Uint64

	// Sender fields
	sender        tonmetrics.AnalyticsClient
	flushInterval time.Duration
}

// NewCollector builds a collector with a periodic flush.
func NewCollector(capacity int, client tonmetrics.AnalyticsClient, flushInterval time.Duration) *Collector {
	return &Collector{
		events:        make([]interface{}, 0, capacity),
		capacity:      capacity,
		sender:        client,
		flushInterval: flushInterval,
	}
}

// TryAdd enqueues without blocking. If full, returns false and increments drop count.
func (c *Collector) TryAdd(event interface{}) bool {
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
func (c *Collector) PopAll() []interface{} {
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
func (c *Collector) Dropped() uint64 {
	return c.dropped.Load()
}

// IsFull returns true if the buffer is at capacity.
func (c *Collector) IsFull() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events) >= c.capacity
}

// Len returns the current number of events in the buffer.
func (c *Collector) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

// Run starts draining until the context is canceled.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	logrus.WithField("prefix", "analytics").Debugf("analytics collector started with flush interval %v", c.flushInterval)

	for {
		select {
		case <-ctx.Done():
			logrus.WithField("prefix", "analytics").Debug("analytics collector stopping, performing final flush")
			events := c.PopAll()
			if len(events) > 0 {
				logrus.WithField("prefix", "analytics").Debugf("final flush: sending %d events from collector", len(events))
				// Use background context for final flush since original context is done
				if err := c.sender.SendBatch(context.Background(), events); err != nil {
					logrus.WithError(err).Warnf("analytics: failed to send final batch of %d events", len(events))
				}
			}
			logrus.WithField("prefix", "analytics").Debug("analytics collector stopped")
			return
		case <-ticker.C:
			logrus.WithField("prefix", "analytics").Debug("analytics collector ticker fired")
		}

		events := c.PopAll()
		if len(events) > 0 {
			logrus.WithField("prefix", "analytics").Debugf("flushing %d events from collector", len(events))
			if err := c.sender.SendBatch(ctx, events); err != nil {
				logrus.WithError(err).Warnf("analytics: failed to send batch of %d events", len(events))
			}
		}
	}
}
