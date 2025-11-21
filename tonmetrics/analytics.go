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

// AnalyticsClient defines the interface for analytics clients
type AnalyticsClient interface {
	SendBatch(ctx context.Context, events []interface{})
}

// TonMetricsClient handles sending analytics events
type TonMetricsClient struct {
	client       *http.Client
	analyticsURL string
	bridgeURL    string
	environment  string
	subsystem    string
	version      string
	networkId    string
}

// NewAnalyticsClient creates a new analytics client
func NewAnalyticsClient() AnalyticsClient {
	configuredAnalyticsURL := config.Config.TonAnalyticsURL
	if !config.Config.TONAnalyticsEnabled {
		return NewNoopMetricsClient(configuredAnalyticsURL)
	}
	if config.Config.TonAnalyticsNetworkId != "-239" && config.Config.TonAnalyticsNetworkId != "-3" {
		logrus.Fatalf("invalid NETWORK_ID '%s'. Allowed values: -239 (mainnet) and -3 (testnet)", config.Config.TonAnalyticsNetworkId)
	}
	return &TonMetricsClient{
		client:       http.DefaultClient,
		analyticsURL: configuredAnalyticsURL,
		bridgeURL:    config.Config.TonAnalyticsBridgeURL,
		subsystem:    "bridge",
		environment:  "bridge",
		version:      config.Config.TonAnalyticsBridgeVersion,
		networkId:    config.Config.TonAnalyticsNetworkId,
	}
}

// SendBatch sends a batch of events to the analytics endpoint in a single HTTP request.
func (a *TonMetricsClient) SendBatch(ctx context.Context, events []interface{}) {
	a.send(ctx, events, a.analyticsURL, "analytics")
}

func (a *TonMetricsClient) send(ctx context.Context, events []interface{}, endpoint string, prefix string) {
	if len(events) == 0 {
		return
	}

	log := logrus.WithField("prefix", prefix)

	log.Debugf("preparing to send analytics batch of %d events to %s", len(events), endpoint)

	analyticsData, err := json.Marshal(events)
	if err != nil {
		log.Errorf("failed to marshal analytics batch: %v", err)
		return
	}

	log.Debugf("marshaled analytics data size: %d bytes", len(analyticsData))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(analyticsData))
	if err != nil {
		log.Errorf("failed to create analytics request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	log.Debugf("sending analytics batch request to %s", endpoint)

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
		log.Warnf("analytics batch request to %s returned status %d", endpoint, resp.StatusCode)
		return
	}

	log.Debugf("analytics batch of %d events sent successfully with status %d", len(events), resp.StatusCode)
}

// NoopMetricsClient forwards analytics to a mock endpoint when analytics are disabled.
type NoopMetricsClient struct {
	client  *http.Client
	mockURL string
}

// NewNoopMetricsClient builds a mock metrics client to help integration tests capture analytics payloads.
func NewNoopMetricsClient(mockURL string) *NoopMetricsClient {
	return &NoopMetricsClient{
		client:  http.DefaultClient,
		mockURL: mockURL,
	}
}

// SendBatch forwards analytics to the configured mock endpoint to aid testing.
func (n *NoopMetricsClient) SendBatch(ctx context.Context, events []interface{}) {
	if n.mockURL == "" {
		logrus.WithField("prefix", "analytics").Debug("analytics disabled and no mock URL configured, skipping send")
		return
	}
	if len(events) == 0 {
		return
	}
	logrus.WithField("prefix", "analytics").Debugf("analytics disabled, forwarding batch to mock server at %s", n.mockURL)
	(&TonMetricsClient{client: n.client}).send(ctx, events, n.mockURL, "analytics-mock")
}
