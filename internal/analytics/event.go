package analytics

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/tonmetrics"
)

// EventBuilder defines methods to create various analytics events.
type EventBuilder interface {
	NewBridgeEventsClientSubscribedEvent(clientID, traceID string) tonmetrics.BridgeEventsClientSubscribedEvent
	NewBridgeEventsClientUnsubscribedEvent(clientID, traceID string) tonmetrics.BridgeEventsClientUnsubscribedEvent
	NewBridgeMessageSentEvent(clientID, traceID string, messageID int64, messageHash string) tonmetrics.BridgeMessageSentEvent
	NewBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) tonmetrics.BridgeMessageReceivedEvent
	NewBridgeMessageExpiredEvent(clientID, traceID string, messageID int64, messageHash string) tonmetrics.BridgeMessageExpiredEvent
	NewBridgeMessageValidationFailedEvent(clientID, traceID, requestType, messageHash string) tonmetrics.BridgeMessageValidationFailedEvent
	NewBridgeVerifyEvent(clientID, traceID, verificationResult string) tonmetrics.BridgeVerifyEvent
	NewBridgeVerifyValidationFailedEvent(clientID, traceID string, errorCode int, errorMessage string) tonmetrics.BridgeVerifyValidationFailedEvent
}

type AnalyticEventBuilder struct {
	bridgeURL   string
	environment string
	subsystem   string
	version     string
	networkId   string
}

func NewEventBuilder(bridgeURL, environment, subsystem, version, networkId string) EventBuilder {
	return &AnalyticEventBuilder{
		bridgeURL:   bridgeURL,
		environment: environment,
		subsystem:   subsystem,
		version:     version,
		networkId:   networkId,
	}
}

// NewBridgeEventsClientSubscribedEvent builds a bridge-events-client-subscribed event.
func (a *AnalyticEventBuilder) NewBridgeEventsClientSubscribedEvent(clientID, traceID string) tonmetrics.BridgeEventsClientSubscribedEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeEventsClientSubscribedEventEventNameBridgeEventsClientSubscribed
	environment := tonmetrics.BridgeEventsClientSubscribedEventClientEnvironment(a.environment)
	subsystem := tonmetrics.BridgeEventsClientSubscribedEventSubsystem(a.subsystem)

	return tonmetrics.BridgeEventsClientSubscribedEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		TraceId:           &traceID,
		ClientTimestamp:   &timestamp,
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		Version:           &a.version,
	}
}

// NewBridgeEventsClientUnsubscribedEvent builds a bridge-events-client-unsubscribed event.
func (a *AnalyticEventBuilder) NewBridgeEventsClientUnsubscribedEvent(clientID, traceID string) tonmetrics.BridgeEventsClientUnsubscribedEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeEventsClientUnsubscribedEventEventNameBridgeEventsClientUnsubscribed
	environment := tonmetrics.BridgeEventsClientUnsubscribedEventClientEnvironment(a.environment)
	subsystem := tonmetrics.BridgeEventsClientUnsubscribedEventSubsystem(a.subsystem)

	return tonmetrics.BridgeEventsClientUnsubscribedEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		TraceId:           &traceID,
		ClientTimestamp:   &timestamp,
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		Version:           &a.version,
	}
}

// NewBridgeMessageSentEvent builds a bridge-message-sent event.
func (a *AnalyticEventBuilder) NewBridgeMessageSentEvent(clientID, traceID string, messageID int64, messageHash string) tonmetrics.BridgeMessageSentEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeMessageSentEventEventNameBridgeMessageSent
	environment := tonmetrics.BridgeMessageSentEventClientEnvironment(a.environment)
	subsystem := tonmetrics.BridgeMessageSentEventSubsystem(a.subsystem)
	messageIDStr := fmt.Sprintf("%d", messageID)

	return tonmetrics.BridgeMessageSentEvent{
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
}

// NewBridgeMessageReceivedEvent builds a bridge message received event.
func (a *AnalyticEventBuilder) NewBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) tonmetrics.BridgeMessageReceivedEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeMessageReceivedEventEventNameBridgeMessageReceived
	environment := tonmetrics.BridgeMessageReceivedEventClientEnvironment(a.environment)
	subsystem := tonmetrics.BridgeMessageReceivedEventSubsystem(a.subsystem)
	messageIDStr := fmt.Sprintf("%d", messageID)

	event := tonmetrics.BridgeMessageReceivedEvent{
		BridgeUrl:            &a.bridgeURL,
		ClientEnvironment:    &environment,
		ClientId:             &clientID,
		ClientTimestamp:      &timestamp,
		EventId:              newAnalyticsEventID(),
		EventName:            &eventName,
		MessageId:            &messageIDStr,
		EncryptedMessageHash: &messageHash,
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

// NewBridgeMessageExpiredEvent builds a bridge-message-expired event.
func (a *AnalyticEventBuilder) NewBridgeMessageExpiredEvent(clientID, traceID string, messageID int64, messageHash string) tonmetrics.BridgeMessageExpiredEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeMessageExpiredEventEventNameBridgeMessageExpired
	environment := tonmetrics.BridgeMessageExpiredEventClientEnvironment(a.environment)
	subsystem := tonmetrics.BridgeMessageExpiredEventSubsystem(a.subsystem)
	messageIdStr := fmt.Sprintf("%d", messageID)

	return tonmetrics.BridgeMessageExpiredEvent{
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
}

// NewBridgeMessageValidationFailedEvent builds a bridge-message-validation-failed event.
func (a *AnalyticEventBuilder) NewBridgeMessageValidationFailedEvent(clientID, traceID, requestType, messageHash string) tonmetrics.BridgeMessageValidationFailedEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeMessageValidationFailedEventEventNameBridgeMessageValidationFailed
	environment := tonmetrics.BridgeMessageValidationFailedEventClientEnvironment(a.environment)
	subsystem := tonmetrics.BridgeMessageValidationFailedEventSubsystem(a.subsystem)

	event := tonmetrics.BridgeMessageValidationFailedEvent{
		BridgeUrl:            &a.bridgeURL,
		ClientEnvironment:    &environment,
		ClientId:             &clientID,
		ClientTimestamp:      &timestamp,
		EncryptedMessageHash: optionalString(messageHash),
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

// NewBridgeVerifyEvent builds a bridge-verify event.
func (a *AnalyticEventBuilder) NewBridgeVerifyEvent(clientID, traceID, verificationResult string) tonmetrics.BridgeVerifyEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeVerifyEventEventNameBridgeVerify
	environment := tonmetrics.BridgeVerifyEventClientEnvironment(a.environment)
	subsystem := tonmetrics.BridgeVerifyEventSubsystem(a.subsystem)
	verifyType := tonmetrics.BridgeVerifyEventVerifyTypeConnect

	return tonmetrics.BridgeVerifyEvent{
		BridgeUrl:          &a.bridgeURL,
		ClientEnvironment:  &environment,
		ClientId:           &clientID,
		ClientTimestamp:    &timestamp,
		EventId:            newAnalyticsEventID(),
		EventName:          &eventName,
		NetworkId:          &a.networkId,
		Subsystem:          &subsystem,
		VerificationResult: optionalString(verificationResult),
		VerifyType:         &verifyType,
		Version:            &a.version,
	}
}

// NewBridgeVerifyValidationFailedEvent builds a bridge-verify-validation-failed event.
func (a *AnalyticEventBuilder) NewBridgeVerifyValidationFailedEvent(clientID, traceID string, errorCode int, errorMessage string) tonmetrics.BridgeVerifyValidationFailedEvent {
	timestamp := int(time.Now().Unix())
	eventName := tonmetrics.BridgeVerifyValidationFailed
	environment := tonmetrics.BridgeVerifyValidationFailedEventClientEnvironment(a.environment)
	subsystem := tonmetrics.Bridge
	verifyType := tonmetrics.BridgeVerifyValidationFailedEventVerifyTypeConnect

	return tonmetrics.BridgeVerifyValidationFailedEvent{
		BridgeUrl:         &a.bridgeURL,
		ClientEnvironment: &environment,
		ClientId:          &clientID,
		ClientTimestamp:   &timestamp,
		ErrorCode:         &errorCode,
		ErrorMessage:      &errorMessage,
		EventId:           newAnalyticsEventID(),
		EventName:         &eventName,
		NetworkId:         &a.networkId,
		Subsystem:         &subsystem,
		TraceId:           optionalString(traceID),
		VerifyType:        &verifyType,
		Version:           &a.version,
	}
}

func optionalString(value string) *string {
	if value == "" {
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
