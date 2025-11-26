package analytics

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// Collector drains events from a collector and forwards to a sender.
type Collector struct {
	collector     *RingCollector
	sender        AnalyticSender
	flushInterval time.Duration
}

// NewCollector builds a collector with a periodic flush.
func NewCollector(collector *RingCollector, analyticsSender AnalyticSender, flushInterval time.Duration) *Collector {
	return &Collector{
		collector:     collector,
		sender:        analyticsSender,
		flushInterval: flushInterval,
	}
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
			events := c.collector.PopAll()
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

		events := c.collector.PopAll()
		if len(events) > 0 {
			logrus.WithField("prefix", "analytics").Debugf("flushing %d events from collector", len(events))
			if err := c.sender.SendBatch(ctx, events); err != nil {
				logrus.WithError(err).Warnf("analytics: failed to send batch of %d events", len(events))
			}
		}
	}
}
