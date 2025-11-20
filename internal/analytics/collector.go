package analytics

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/tonmetrics"
)

// AnalyticSender delivers analytics events to a backend.
type AnalyticSender interface {
	Publish(context.Context, Event) error
}

// TonMetricsSender adapts TonMetrics to the AnalyticSender interface.
type TonMetricsSender struct {
	client tonmetrics.AnalyticsClient
}

// NewTonMetricsSender constructs a TonMetrics-backed sender.
func NewTonMetricsSender(client tonmetrics.AnalyticsClient) AnalyticSender {
	return &TonMetricsSender{client: client}
}

func (t *TonMetricsSender) Publish(_ context.Context, event Event) error {
	event.Dispatch(t.client)
	return nil
}

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

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.collector.Notify():
		case <-ticker.C:
		}

		events := c.collector.PopAll()
		for _, event := range events {
			if err := c.sender.Publish(ctx, event); err != nil {
				logrus.WithError(err).Warn("analytics: failed to publish event")
			}
		}
	}
}
