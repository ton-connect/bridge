package analytics

import (
	"context"
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
	eventCh  chan interface{}
	notifyCh chan struct{}

	capacity        int
	triggerCapacity int
	dropped         atomic.Uint64

	// Sender fields
	sender        tonmetrics.AnalyticsClient
	flushInterval time.Duration
}

// NewCollector builds a collector with a periodic flush.
func NewCollector(capacity int, client tonmetrics.AnalyticsClient, flushInterval time.Duration) *Collector {
	triggerCapacity := capacity
	if capacity > 10 {
		triggerCapacity = capacity - 10
	}
	return &Collector{
		eventCh:         make(chan interface{}, capacity), // channel for events
		notifyCh:        make(chan struct{}, 1),           // channel to trigger flushing
		capacity:        capacity,
		triggerCapacity: triggerCapacity,
		sender:          client,
		flushInterval:   flushInterval,
	}
}

// TryAdd enqueues without blocking. If full, returns false and increments drop count.
func (c *Collector) TryAdd(event interface{}) bool {
	result := false
	select {
	case c.eventCh <- event:
		result = true
	default:
		c.dropped.Add(1)
		result = false
	}

	if len(c.eventCh) >= c.triggerCapacity {
		select {
		case c.notifyCh <- struct{}{}:
		default:
		}
	}
	return result
}

// PopAll drains all pending events.
// If there are 100 or more events, only reads 100 elements.
func (c *Collector) PopAll() []interface{} {
	channelLen := len(c.eventCh)
	if channelLen == 0 {
		return nil
	}

	limit := channelLen
	if channelLen >= 100 {
		limit = 100
	}

	result := make([]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		select {
		case event := <-c.eventCh:
			result = append(result, event)
		default:
			return result
		}
	}
	return result
}

// Dropped returns the number of events that were dropped due to buffer being full.
func (c *Collector) Dropped() uint64 {
	return c.dropped.Load()
}

// IsFull returns true if the buffer is at capacity.
func (c *Collector) IsFull() bool {
	return len(c.eventCh) >= c.capacity
}

// Len returns the current number of events in the buffer.
func (c *Collector) Len() int {
	return len(c.eventCh)
}

// Run periodically flushes events until the context is canceled.
// Flushes occur when:
// 1. The flush interval (500ms) has elapsed and there are events
// 2. The buffer has reached the trigger capacity (capacity - 10)
func (c *Collector) Run(ctx context.Context) {
	flushTicker := time.NewTicker(c.flushInterval)
	defer flushTicker.Stop()

	logrus.WithField("prefix", "analytics").Debugf("analytics collector started with flush interval %v", c.flushInterval)

	for {
		select {
		case <-ctx.Done():
			logrus.WithField("prefix", "analytics").Debug("analytics collector stopping, performing final flush")
			// Use fresh context for final flush since ctx is already cancelled
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			c.Flush(flushCtx)
			cancel()
			logrus.WithField("prefix", "analytics").Debug("analytics collector stopped")
			return
		case <-flushTicker.C:
			if c.Len() > 0 {
				logrus.WithField("prefix", "analytics").Debug("analytics collector ticker fired")
				c.Flush(ctx)
			}
		case <-c.notifyCh:
			logrus.WithField("prefix", "analytics").Debugf("analytics collector buffer reached %d events, flushing", c.Len())
			c.Flush(ctx)
		}
	}
}

func (c *Collector) Flush(ctx context.Context) {
	events := c.PopAll()
	if len(events) > 0 {
		logrus.WithField("prefix", "analytics").Debugf("flushing %d events from collector", len(events))
		if err := c.sender.SendBatch(ctx, events); err != nil {
			logrus.WithError(err).Warnf("analytics: failed to send batch of %d events", len(events))
		}
	}
}
