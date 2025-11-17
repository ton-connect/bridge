package tonmetrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/config"
)

const (
	analyticsURL = "https://analytics.ton.org/events/"
)

// AnalyticsClient defines the interface for analytics clients
type AnalyticsClient interface {
	SendEvent(event interface{})
	CreateBridgeClientConnectStartedEvent(clientID, traceID string) BridgeClientConnectStartedEvent
	CreateBridgeConnectEstablishedEvent(clientID, traceID string, durationMillis int) BridgeConnectEstablishedEvent
	CreateBridgeClientConnectErrorEvent(clientID, traceID string, errorCode int, errorMessage string) BridgeClientConnectErrorEvent
	CreateBridgeEventsClientSubscribedEvent(clientID, traceID string) BridgeEventsClientSubscribedEvent
	CreateBridgeEventsClientUnsubscribedEvent(clientID, traceID string) BridgeEventsClientUnsubscribedEvent
	CreateBridgeClientMessageDecodeErrorEvent(clientID, traceID string, encryptedMessageHash string, errorCode int, errorMessage string) BridgeClientMessageDecodeErrorEvent
	CreateBridgeMessageSentEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageSentEvent
	CreateBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageReceivedEvent
	CreateBridgeMessageExpiredEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageExpiredEvent
	CreateBridgeMessageValidationFailedEvent(clientID, traceID, requestType, encryptedMessageHash string) BridgeMessageValidationFailedEvent
	CreateBridgeVerifyEvent(clientID, traceID, verificationResult string) BridgeVerifyEvent
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
		environment: "bridge", // TODO this is client environment, e.g., "miniapp". No idea how to get it here
		version:     config.Config.BridgeVersion,
		networkId:   config.Config.NetworkId,
	}
}

// sendEvent sends an event to the analytics endpoint
// TODO pass events in batches
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
	req.Header.Set("X-Client-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	_, err = a.client.Do(req)
	if err != nil {
		log.Errorf("failed to send analytics request: %v", err)
	}

	// log.Debugf("analytics request sent successfully: %s", string(analyticsData))
}

// CreateBridgeClientConnectStartedEvent builds a bridge-client-connect-started event.
func (a *TonMetricsClient) CreateBridgeClientConnectStartedEvent(clientID, traceID string) BridgeClientConnectStartedEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeClientConnectStartedEventEventNameBridgeClientConnectStarted
	environment := BridgeClientConnectStartedEventClientEnvironment(a.environment)
	subsystem := BridgeClientConnectStartedEventSubsystem(a.subsystem)

	return BridgeClientConnectStartedEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		TraceId:           optionalString(traceID),
		Version:           &a.version,
	}
}

// CreateBridgeConnectEstablishedEvent builds a bridge-client-connect-established event.
func (a *TonMetricsClient) CreateBridgeConnectEstablishedEvent(clientID, traceID string, durationMillis int) BridgeConnectEstablishedEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeConnectEstablishedEventEventNameBridgeClientConnectEstablished
	environment := BridgeConnectEstablishedEventClientEnvironment(a.environment)
	subsystem := BridgeConnectEstablishedEventSubsystem(a.subsystem)

	return BridgeConnectEstablishedEvent{
		BridgeConnectDuration: optionalInt(durationMillis),
		BridgeUrl:             &a.bridgeURL,
		ClientEnvironment:     &environment,
		ClientId:              &clientID,
		ClientTimestamp:       &timestamp,
		EventId:               newAnalyticsEventID(),
		EventName:             &eventName,
		NetworkId:             &a.networkId,
		Subsystem:             &subsystem,
		TraceId:               optionalString(traceID),
		Version:               &a.version,
	}
}

// CreateBridgeClientConnectErrorEvent builds a bridge-client-connect-error event.
func (a *TonMetricsClient) CreateBridgeClientConnectErrorEvent(clientID, traceID string, errorCode int, errorMessage string) BridgeClientConnectErrorEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeClientConnectErrorEventEventNameBridgeClientConnectError
	environment := BridgeClientConnectErrorEventClientEnvironment(a.environment)
	subsystem := BridgeClientConnectErrorEventSubsystem(a.subsystem)

	return BridgeClientConnectErrorEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		ErrorCode:         optionalInt(errorCode),
		ErrorMessage:      optionalString(errorMessage),
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		TraceId:           optionalString(traceID),
		Version:           &a.version,
	}
}

// CreateBridgeEventsClientSubscribedEvent builds a bridge-events-client-subscribed event.
func (a *TonMetricsClient) CreateBridgeEventsClientSubscribedEvent(clientID, traceID string) BridgeEventsClientSubscribedEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeEventsClientSubscribedEventEventNameBridgeEventsClientSubscribed
	environment := BridgeEventsClientSubscribedEventClientEnvironment(a.environment)
	subsystem := BridgeEventsClientSubscribedEventSubsystem(a.subsystem)

	return BridgeEventsClientSubscribedEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		TraceId:           optionalString(traceID),
		Version:           &a.version,
	}
}

// CreateBridgeEventsClientUnsubscribedEvent builds a bridge-events-client-unsubscribed event.
func (a *TonMetricsClient) CreateBridgeEventsClientUnsubscribedEvent(clientID, traceID string) BridgeEventsClientUnsubscribedEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeEventsClientUnsubscribedEventEventNameBridgeEventsClientUnsubscribed
	environment := BridgeEventsClientUnsubscribedEventClientEnvironment(a.environment)
	subsystem := BridgeEventsClientUnsubscribedEventSubsystem(a.subsystem)

	return BridgeEventsClientUnsubscribedEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		TraceId:           optionalString(traceID),
		Version:           &a.version,
	}
}

// CreateBridgeClientMessageDecodeErrorEvent builds a bridge-client-message-decode-error event.
func (a *TonMetricsClient) CreateBridgeClientMessageDecodeErrorEvent(clientID, traceID string, encryptedMessageHash string, errorCode int, errorMessage string) BridgeClientMessageDecodeErrorEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeClientMessageDecodeErrorEventEventNameBridgeClientMessageDecodeError
	environment := BridgeClientMessageDecodeErrorEventClientEnvironment(a.environment)
	subsystem := BridgeClientMessageDecodeErrorEventSubsystem(a.subsystem)

	return BridgeClientMessageDecodeErrorEvent{
		BridgeUrl:            &a.bridgeURL,
		ClientEnvironment:    &environment,
		ClientId:             &clientID,
		ClientTimestamp:      &timestamp,
		EncryptedMessageHash: optionalString(encryptedMessageHash),
		ErrorCode:            optionalInt(errorCode),
		ErrorMessage:         optionalString(errorMessage),
		EventId:              newAnalyticsEventID(),
		EventName:            &eventName,
		NetworkId:            &a.networkId,
		Subsystem:            &subsystem,
		TraceId:              optionalString(traceID),
		Version:              &a.version,
	}
}

// CreateBridgeMessageSentEvent builds a bridge-message-sent event.
func (a *TonMetricsClient) CreateBridgeMessageSentEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageSentEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeMessageSentEventEventNameBridgeMessageSent
	environment := BridgeMessageSentEventClientEnvironment(a.environment)
	subsystem := BridgeMessageSentEventSubsystem(a.subsystem)
	messageIDStr := fmt.Sprintf("%d", messageID)

	event := BridgeMessageSentEvent{
		BridgeUrl:            &a.bridgeURL,
		ClientEnvironment:    &environment,
		ClientId:             &clientID,
		ClientTimestamp:      &timestamp,
		EncryptedMessageHash: &messageHash,
		EventId:              newAnalyticsEventID(),
		EventName:            &eventName,
		MessageId:            &messageIDStr,
		NetworkId:            &a.networkId,
		Subsystem:            &subsystem,
		TraceId:              optionalString(traceID),
		Version:              &a.version,
	}

	if requestType != "" {
		event.RequestType = &requestType
	}

	return event
}

// CreateBridgeMessageReceivedEvent builds a bridge message received event (wallet-connect-request-received).
func (a *TonMetricsClient) CreateBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageReceivedEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeMessageReceivedEventEventNameWalletConnectRequestReceived
	environment := BridgeMessageReceivedEventClientEnvironment(a.environment)
	subsystem := BridgeMessageReceivedEventSubsystem(a.subsystem)
	messageIDStr := fmt.Sprintf("%d", messageID)

	event := BridgeMessageReceivedEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		MessageId:         &messageIDStr,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		TraceId:           optionalString(traceID),
		Version:           &a.version,
		// TODO BridgeMessageReceivedEvent misses MessageHash field
		// MessageHash:      &messageHash,
	}
	if requestType != "" {
		event.RequestType = &requestType
	}
	return event
}

// CreateBridgeMessageExpiredEvent builds a bridge-message-expired event.
func (a *TonMetricsClient) CreateBridgeMessageExpiredEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageExpiredEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeMessageExpiredEventEventNameBridgeMessageExpired
	environment := BridgeMessageExpiredEventClientEnvironment(a.environment)
	subsystem := BridgeMessageExpiredEventSubsystem(a.subsystem)
	messageIdStr := fmt.Sprintf("%d", messageID)

	event := BridgeMessageExpiredEvent{
		BridgeUrl:            &a.bridgeURL,
		ClientEnvironment:    &environment,
		ClientId:             &clientID,
		ClientTimestamp:      &timestamp,
		EncryptedMessageHash: &messageHash,
		EventId:              newAnalyticsEventID(),
		EventName:            &eventName,
		MessageId:            &messageIdStr,
		NetworkId:            &a.networkId,
		Subsystem:            &subsystem,
		TraceId:              optionalString(traceID),
		Version:              &a.version,
	}
	if requestType != "" {
		event.RequestType = &requestType
	}
	return event
}

// CreateBridgeMessageValidationFailedEvent builds a bridge-message-validation-failed event.
func (a *TonMetricsClient) CreateBridgeMessageValidationFailedEvent(clientID, traceID, requestType, encryptedMessageHash string) BridgeMessageValidationFailedEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeMessageValidationFailedEventEventNameBridgeMessageValidationFailed
	environment := BridgeMessageValidationFailedEventClientEnvironment(a.environment)
	subsystem := BridgeMessageValidationFailedEventSubsystem(a.subsystem)

	event := BridgeMessageValidationFailedEvent{
		BridgeUrl:            &a.bridgeURL,
		ClientEnvironment:    &environment,
		ClientId:             &clientID,
		ClientTimestamp:      &timestamp,
		EncryptedMessageHash: optionalString(encryptedMessageHash),
		EventId:              newAnalyticsEventID(),
		EventName:            &eventName,
		NetworkId:            &a.networkId,
		Subsystem:            &subsystem,
		TraceId:              optionalString(traceID),
		Version:              &a.version,
	}
	if requestType != "" {
		event.RequestType = &requestType
	}
	return event
}

// CreateBridgeVerifyEvent builds a bridge-verify event.
func (a *TonMetricsClient) CreateBridgeVerifyEvent(clientID, traceID, verificationResult string) BridgeVerifyEvent {
	timestamp := int(time.Now().Unix())
	eventName := BridgeVerifyEventEventName("") // TODO fix missing constant in specs
	environment := BridgeVerifyEventClientEnvironment(a.environment)
	subsystem := BridgeVerifyEventSubsystem(a.subsystem)

	return BridgeVerifyEvent{
		BridgeUrl:          &a.bridgeURL,
		ClientEnvironment:  &environment,
		ClientId:           &clientID,
		ClientTimestamp:    &timestamp,
		EventId:            newAnalyticsEventID(),
		EventName:          &eventName,
		NetworkId:          &a.networkId,
		Subsystem:          &subsystem,
		TraceId:            optionalString(traceID),
		VerificationResult: optionalString(verificationResult),
		Version:            &a.version,
	}
}

// NoopMetricsClient does nothing when analytics are disabled
type NoopMetricsClient struct{}

// SendEvent does nothing for NoopMetricsClient
func (n *NoopMetricsClient) SendEvent(event interface{}) {
	// No-op
}

func (n *NoopMetricsClient) CreateBridgeClientConnectStartedEvent(clientID, traceID string) BridgeClientConnectStartedEvent {
	return BridgeClientConnectStartedEvent{}
}

func (n *NoopMetricsClient) CreateBridgeConnectEstablishedEvent(clientID, traceID string, durationMillis int) BridgeConnectEstablishedEvent {
	return BridgeConnectEstablishedEvent{}
}

func (n *NoopMetricsClient) CreateBridgeClientConnectErrorEvent(clientID, traceID string, errorCode int, errorMessage string) BridgeClientConnectErrorEvent {
	return BridgeClientConnectErrorEvent{}
}

func (n *NoopMetricsClient) CreateBridgeEventsClientSubscribedEvent(clientID, traceID string) BridgeEventsClientSubscribedEvent {
	return BridgeEventsClientSubscribedEvent{}
}

func (n *NoopMetricsClient) CreateBridgeEventsClientUnsubscribedEvent(clientID, traceID string) BridgeEventsClientUnsubscribedEvent {
	return BridgeEventsClientUnsubscribedEvent{}
}

func (n *NoopMetricsClient) CreateBridgeClientMessageDecodeErrorEvent(clientID, traceID string, encryptedMessageHash string, errorCode int, errorMessage string) BridgeClientMessageDecodeErrorEvent {
	return BridgeClientMessageDecodeErrorEvent{}
}

func (n *NoopMetricsClient) CreateBridgeMessageSentEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageSentEvent {
	return BridgeMessageSentEvent{}
}

func (n *NoopMetricsClient) CreateBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageReceivedEvent {
	return BridgeMessageReceivedEvent{}
}

func (n *NoopMetricsClient) CreateBridgeMessageExpiredEvent(clientID, traceID, requestType string, messageID int64, messageHash string) BridgeMessageExpiredEvent {
	return BridgeMessageExpiredEvent{}
}

func (n *NoopMetricsClient) CreateBridgeMessageValidationFailedEvent(clientID, traceID, requestType, encryptedMessageHash string) BridgeMessageValidationFailedEvent {
	return BridgeMessageValidationFailedEvent{}
}

func (n *NoopMetricsClient) CreateBridgeVerifyEvent(clientID, traceID, verificationResult string) BridgeVerifyEvent {
	return BridgeVerifyEvent{}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func newAnalyticsEventID() *string {
	id, err := uuid.NewV7()
	if err != nil {
		logrus.WithError(err).Warn("Failed to generate UUIDv7, falling back to UUIDv4")
		str := uuid.New().String()
		return &str
	}
	str := id.String()
	return &str
}
