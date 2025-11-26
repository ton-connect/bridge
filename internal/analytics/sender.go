package analytics

import (
	"context"

	"github.com/ton-connect/bridge/tonmetrics"
)

// AnalyticSender delivers analytics events to a backend.
type AnalyticSender interface {
	SendBatch(context.Context, []interface{}) error
}

// TonMetricsSender adapts TonMetrics to the AnalyticSender interface.
type TonMetricsSender struct {
	client tonmetrics.AnalyticsClient
}

// NewTonMetricsSender constructs a TonMetrics-backed sender.
func NewTonMetricsSender(client tonmetrics.AnalyticsClient) AnalyticSender {
	return &TonMetricsSender{client: client}
}

func (t *TonMetricsSender) SendBatch(ctx context.Context, events []interface{}) error {
	if len(events) == 0 {
		return nil
	}
	t.client.SendBatch(ctx, events)
	return nil
}
