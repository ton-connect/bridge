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
	switch payload := event.Payload.(type) {
	case BridgeMessageExpiredPayload:
		t.client.SendEvent(t.client.CreateBridgeMessageExpiredEvent(
			payload.ClientID,
			payload.TraceID,
			payload.RequestType,
			payload.MessageID,
			payload.MessageHash,
		))
	case BridgeMessageSentPayload:
		t.client.SendEvent(t.client.CreateBridgeMessageSentEvent(
			payload.ClientID,
			payload.TraceID,
			payload.RequestType,
			payload.MessageID,
			payload.MessageHash,
		))
	case BridgeMessageReceivedPayload:
		t.client.SendEvent(t.client.CreateBridgeMessageReceivedEvent(
			payload.ClientID,
			payload.TraceID,
			payload.RequestType,
			payload.MessageID,
			payload.MessageHash,
		))
	case BridgeMessageValidationFailedPayload:
		t.client.SendEvent(t.client.CreateBridgeMessageValidationFailedEvent(
			payload.ClientID,
			payload.TraceID,
			payload.RequestType,
			payload.MessageHash,
		))
	case BridgeVerifyPayload:
		t.client.SendEvent(t.client.CreateBridgeVerifyEvent(
			payload.ClientID,
			payload.TraceID,
			payload.VerificationResult,
		))
	case BridgeEventsClientUnsubscribedPayload:
		t.client.SendEvent(t.client.CreateBridgeEventsClientUnsubscribedEvent(
			payload.ClientID,
			payload.TraceID,
		))
	default:
		// Unknown event types are dropped to keep producers non-blocking.
		logrus.WithField("event_type", event.Type).Debug("analytics: dropping unknown event type")
	}
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
