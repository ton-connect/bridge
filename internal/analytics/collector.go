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
	capacity int
	dropped  atomic.Uint64

	// Sender fields
	sender        tonmetrics.AnalyticsClient
	flushInterval time.Duration
}

// NewCollector builds a collector with a periodic flush.
func NewCollector(capacity int, client tonmetrics.AnalyticsClient, flushInterval time.Duration) *Collector {
	return &Collector{
		eventCh:       make(chan interface{}, capacity),
		capacity:      capacity,
		sender:        client,
		flushInterval: flushInterval,
	}
}

// TryAdd enqueues without blocking. If full, returns false and increments drop count.
func (c *Collector) TryAdd(event interface{}) bool {
	select {
	case c.eventCh <- event:
		return true
	default:
		c.dropped.Add(1)
		return false
	}
}

// PopAll drains all pending events.
func (c *Collector) PopAll() []interface{} {
	var result []interface{}
	for {
		select {
		case event := <-c.eventCh:
			result = append(result, event)
		default:
			return result
		}
	}
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
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

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
		case <-ticker.C:
			logrus.WithField("prefix", "analytics").Debug("analytics collector ticker fired")
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
