package tonmetrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/config"
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

// CreateBridgeRequestReceivedEvent creates a BridgeClientMessageReceivedEvent with common fields populated
func CreateBridgeRequestReceivedEvent(bridgeURL, clientID, traceID, environment, version, networkId string, eventID int64) BridgeClientMessageReceivedEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeClientMessageReceivedEventEventNameBridgeClientMessageReceived
	subsystem := BridgeClientMessageReceivedEventSubsystemBridge
	clientEnv := BridgeClientMessageReceivedEventClientEnvironment(environment)
	eventIDStr := fmt.Sprintf("%d", eventID)

	return BridgeClientMessageReceivedEvent{
		BridgeUrl:         &bridgeURL,
		ClientEnvironment: &clientEnv,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		EventId:           &eventIDStr,
		EventName:         &eventName,
		NetworkId:         &networkId,
		Subsystem:         &subsystem,
		TraceId:           &traceID,
		Version:           &version,
	}
}

// CreateBridgeRequestSentEvent creates a BridgeRequestSentEvent with common fields populated
func CreateBridgeRequestSentEvent(bridgeURL, clientID, traceID, requestType, environment, version, networkId string, eventID int64) BridgeRequestSentEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeRequestSentEventEventNameBridgeClientMessageSent
	subsystem := BridgeRequestSentEventSubsystemBridge
	clientEnv := BridgeRequestSentEventClientEnvironment(environment)
	eventIDStr := fmt.Sprintf("%d", eventID)

	return BridgeRequestSentEvent{
		BridgeUrl:         &bridgeURL,
		ClientEnvironment: &clientEnv,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		EventId:           &eventIDStr,
		EventName:         &eventName,
		NetworkId:         &networkId,
		RequestType:       &requestType,
		Subsystem:         &subsystem,
		TraceId:           &traceID,
		Version:           &version,
	}
}

// NoopMetricsClient does nothing when analytics are disabled
type NoopMetricsClient struct{}

// SendEvent does nothing for NoopMetricsClient
func (n *NoopMetricsClient) SendEvent(event interface{}) {
	// No-op
}
