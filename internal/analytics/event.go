package analytics

import "github.com/ton-connect/bridge/tonmetrics"

// NewBridgeMessageExpiredEvent builds a bridge-message-expired event.
func NewBridgeMessageExpiredEvent(clientID, traceID string, messageID int64, messageHash string) interface{} {
	// Event creation is delegated to a temporary client to fill common fields.
	// In production, these events are collected and sent in batches.
	client := getEventBuilder()
	return client.CreateBridgeMessageExpiredEvent(clientID, traceID, "", messageID, messageHash)
}

func NewBridgeMessageSentEvent(clientID, traceID string, messageID int64, messageHash string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeMessageSentEvent(clientID, traceID, "", messageID, messageHash)
}

func NewBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeMessageReceivedEvent(clientID, traceID, requestType, messageID, messageHash)
}

func NewBridgeMessageValidationFailedEvent(clientID, traceID, requestType, messageHash string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeMessageValidationFailedEvent(clientID, traceID, requestType, messageHash)
}

func NewBridgeVerifyEvent(clientID, traceID, verificationResult string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeVerifyEvent(clientID, traceID, verificationResult)
}

func NewBridgeEventsClientSubscribedEvent(clientID, traceID string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeEventsClientSubscribedEvent(clientID, traceID)
}

func NewBridgeEventsClientUnsubscribedEvent(clientID, traceID string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeEventsClientUnsubscribedEvent(clientID, traceID)
}

// NewBridgeConnectEstablishedEvent builds a bridge-connect-established event.
func NewBridgeConnectEstablishedEvent(clientID, traceID string, connectDuration int) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeConnectEstablishedEvent(clientID, traceID, connectDuration)
}

// NewBridgeRequestSentEvent builds a bridge-request-sent event.
func NewBridgeRequestSentEvent(clientID, traceID, requestType string, messageID int64, messageHash string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeRequestSentEvent(clientID, traceID, requestType, messageID, messageHash)
}

// NewBridgeVerifyValidationFailedEvent builds a bridge-verify-validation-failed event.
func NewBridgeVerifyValidationFailedEvent(clientID, traceID string, errorCode int, errorMessage string) interface{} {
	client := getEventBuilder()
	return client.CreateBridgeVerifyValidationFailedEvent(clientID, traceID, errorCode, errorMessage)
}

// getEventBuilder returns the global analytics client for building events.
// Events are created with common fields (bridge URL, version, etc.) filled in.
func getEventBuilder() tonmetrics.AnalyticsClient {
	return tonmetrics.NewAnalyticsClient()
}
