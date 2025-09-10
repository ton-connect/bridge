package tonmetrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/config"
)

const (
	analyticsBaseURL = "https://analytics.ton.org/events/"
)

// AnalyticsClient defines the interface for analytics clients
type AnalyticsClient interface {
	SendBridgeRequestReceivedEvent(event BridgeRequestReceivedEvent)
	SendBridgeRequestSentEvent(event BridgeRequestSentEvent)
}

// TonMetricsClient handles sending analytics events
type TonMetricsClient struct {
	client *http.Client
}

// NewAnalyticsClient creates a new analytics client
func NewAnalyticsClient() AnalyticsClient {
	if !config.Config.TFAnalyticsEnabled {
		return &NoopMetricsClient{}
	}
	return &TonMetricsClient{
		client: http.DefaultClient,
	}
}

// SendBridgeRequestReceivedEvent sends a bridge request received event to analytics
func (a *TonMetricsClient) SendBridgeRequestReceivedEvent(event BridgeRequestReceivedEvent) {
	log := logrus.WithField("prefix", "analytics")

	if err := a.sendEvent(event, "bridge-request-received"); err != nil {
		log.Errorf("failed to send bridge request received event: %v", err)
	}
}

// SendBridgeRequestSentEvent sends a bridge request sent event to analytics
func (a *TonMetricsClient) SendBridgeRequestSentEvent(event BridgeRequestSentEvent) {
	log := logrus.WithField("prefix", "analytics")

	if err := a.sendEvent(event, "bridge-request-sent"); err != nil {
		log.Errorf("failed to send bridge request sent event: %v", err)
	}
}

// sendEvent sends an event to the analytics endpoint
func (a *TonMetricsClient) sendEvent(event interface{}, eventType string) error {
	// log := logrus.WithField("prefix", "analytics")
	analyticsData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal analytics data: %w", err)
	}

	url := analyticsBaseURL + eventType
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(analyticsData))
	if err != nil {
		return fmt.Errorf("failed to create analytics request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	_, err = a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send analytics request: %w", err)
	}

	// log.WithField("event_type", eventType).WithField("url", url).Debug("analytics request sent successfully")
	return nil
}

// CreateBridgeRequestReceivedEvent creates a BridgeRequestReceivedEvent with common fields populated
func CreateBridgeRequestReceivedEvent(bridgeURL, clientID, traceID, environment, version, networkId string, eventID int64) BridgeRequestReceivedEvent {
	return BridgeRequestReceivedEvent{
		BridgeUrl:         bridgeURL,
		ClientEnvironment: environment,
		ClientId:          clientID,
		ClientTimestamp:   int32(time.Now().Unix()),
		EventId:           fmt.Sprintf("%d", eventID),
		EventName:         "bridge-request-received",
		MessageId:         "",
		NetworkId:         networkId,
		RequestType:       "",
		Subsystem:         "bridge",
		TraceId:           traceID,
		UserId:            clientID,
		Version:           version,
	}
}

// CreateBridgeRequestSentEvent creates a BridgeRequestSentEvent with common fields populated
func CreateBridgeRequestSentEvent(bridgeURL, clientID, traceID, requestType, userID, environment, version, networkId string, eventID int64) BridgeRequestSentEvent {
	return BridgeRequestSentEvent{
		BridgeUrl:         bridgeURL,
		ClientEnvironment: environment,
		ClientId:          clientID,
		ClientTimestamp:   int32(time.Now().Unix()),
		EventId:           fmt.Sprintf("%d", eventID),
		EventName:         "bridge-request-sent",
		MessageId:         "",
		NetworkId:         networkId,
		RequestType:       requestType,
		Subsystem:         "bridge",
		TraceId:           traceID,
		UserId:            userID,
		Version:           version,
	}
}

// NoopMetricsClient does nothing when analytics are disabled
type NoopMetricsClient struct{}

// SendBridgeRequestReceivedEvent does nothing for NoopAnalyticsClient
func (n *NoopMetricsClient) SendBridgeRequestReceivedEvent(event BridgeRequestReceivedEvent) {
	// No-op
}

// SendBridgeRequestSentEvent does nothing for NoopAnalyticsClient
func (n *NoopMetricsClient) SendBridgeRequestSentEvent(event BridgeRequestSentEvent) {
	// No-op
}
