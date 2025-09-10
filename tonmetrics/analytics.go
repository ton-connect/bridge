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
	analyticsURL = "https://analytics.ton.org/events/"
)

// AnalyticsClient defines the interface for analytics clients
type AnalyticsClient interface {
	SendEvent(event interface{})
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

// sendEvent sends an event to the analytics endpoint
func (a *TonMetricsClient) SendEvent(event interface{}) {
	log := logrus.WithField("prefix", "analytics")
	analyticsData, err := json.Marshal(event)
	if err != nil {
		log.Errorf("failed to marshal analytics data: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, analyticsURL, bytes.NewReader(analyticsData))
	if err != nil {
		log.Errorf("failed to create analytics request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	_, err = a.client.Do(req)
	if err != nil {
		log.Errorf("failed to send analytics request: %v", err)
	}

	// log.Debugf("analytics request sent successfully: %s", string(analyticsData))
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
		Version:           version,
	}
}

// CreateBridgeRequestSentEvent creates a BridgeRequestSentEvent with common fields populated
func CreateBridgeRequestSentEvent(bridgeURL, clientID, traceID, requestType, environment, version, networkId string, eventID int64) BridgeRequestSentEvent {
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
		Version:           version,
	}
}

// NoopMetricsClient does nothing when analytics are disabled
type NoopMetricsClient struct{}

// SendEvent does nothing for NoopMetricsClient
func (n *NoopMetricsClient) SendEvent(event interface{}) {
	// No-op
}
