package tonmetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/config"
)

const (
	analyticsURL = "https://analytics.ton.org/events"
)

// AnalyticsClient defines the interface for analytics clients
type AnalyticsClient interface {
	SendBatch(ctx context.Context, events []interface{})
}

// TonMetricsClient handles sending analytics events
type TonMetricsClient struct {
	client      *http.Client
	bridgeURL   string
	environment string
	subsystem   string
	version     string
	networkId   string
}

// NewAnalyticsClient creates a new analytics client
func NewAnalyticsClient() AnalyticsClient {
	if !config.Config.TFAnalyticsEnabled {
		return &NoopMetricsClient{}
	}
	if config.Config.NetworkId != "-239" && config.Config.NetworkId != "-3" {
		logrus.Fatalf("invalid NETWORK_ID '%s'. Allowed values: -239 (mainnet) and -3 (testnet)", config.Config.NetworkId)
	}
	return &TonMetricsClient{
		client:      http.DefaultClient,
		bridgeURL:   config.Config.BridgeURL,
		subsystem:   "bridge",
		environment: "bridge",
		version:     config.Config.BridgeVersion,
		networkId:   config.Config.NetworkId,
	}
}

// SendBatch sends a batch of events to the analytics endpoint in a single HTTP request.
func (a *TonMetricsClient) SendBatch(ctx context.Context, events []interface{}) {
	if len(events) == 0 {
		return
	}

	log := logrus.WithField("prefix", "analytics")

	log.Debugf("preparing to send analytics batch of %d events to %s", len(events), analyticsURL)

	analyticsData, err := json.Marshal(events)
	if err != nil {
		log.Errorf("failed to marshal analytics batch: %v", err)
		return
	}

	log.Debugf("marshaled analytics data size: %d bytes", len(analyticsData))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, analyticsURL, bytes.NewReader(analyticsData))
	if err != nil {
		log.Errorf("failed to create analytics request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	log.Debugf("sending analytics batch request to %s", analyticsURL)

	resp, err := a.client.Do(req)
	if err != nil {
		log.Errorf("failed to send analytics batch: %v", err)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Errorf("failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Warnf("analytics batch request returned status %d", resp.StatusCode)
		return
	}

	log.Debugf("analytics batch of %d events sent successfully with status %d", len(events), resp.StatusCode)
}

// NoopMetricsClient does nothing when analytics are disabled
type NoopMetricsClient struct{}

// SendBatch does nothing for NoopMetricsClient
func (n *NoopMetricsClient) SendBatch(ctx context.Context, events []interface{}) {
	// No-op
}
