package analytics

import "github.com/ton-connect/bridge/tonmetrics"

// Event is a small command that knows how to emit itself via TonMetrics.
type Event struct {
	dispatch func(tonmetrics.AnalyticsClient)
}

// Dispatch sends the event through the provided client.
func (e Event) Dispatch(client tonmetrics.AnalyticsClient) {
	if e.dispatch != nil {
		e.dispatch(client)
	}
}

// NewBridgeMessageExpiredEvent builds an Event for a message expiration.
func NewBridgeMessageExpiredEvent(clientID, traceID string, messageID int64, messageHash string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeMessageExpiredEvent(
				clientID,
				traceID,
				"",
				messageID,
				messageHash,
			))
		},
	}
}

func NewBridgeMessageSentEvent(clientID, traceID string, messageID int64, messageHash string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeMessageSentEvent(
				clientID,
				traceID,
				"",
				messageID,
				messageHash,
			))
		},
	}
}

func NewBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeMessageReceivedEvent(
				clientID,
				traceID,
				requestType,
				messageID,
				messageHash,
			))
		},
	}
}

func NewBridgeMessageValidationFailedEvent(clientID, traceID, requestType, messageHash string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeMessageValidationFailedEvent(
				clientID,
				traceID,
				requestType,
				messageHash,
			))
		},
	}
}

func NewBridgeVerifyEvent(clientID, traceID, verificationResult string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeVerifyEvent(
				clientID,
				traceID,
				verificationResult,
			))
		},
	}
}

func NewBridgeEventsClientSubscribedEvent(clientID, traceID string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeEventsClientSubscribedEvent(
				clientID,
				traceID,
			))
		},
	}
}

func NewBridgeEventsClientUnsubscribedEvent(clientID, traceID string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeEventsClientUnsubscribedEvent(
				clientID,
				traceID,
			))
		},
	}
}

// NewBridgeConnectEstablishedEvent builds an Event for when connection is established.
func NewBridgeConnectEstablishedEvent(clientID, traceID string, connectDuration int) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeConnectEstablishedEvent(
				clientID,
				traceID,
				connectDuration,
			))
		},
	}
}

// NewBridgeRequestSentEvent builds an Event for when request is posted to /message.
func NewBridgeRequestSentEvent(clientID, traceID, requestType string, messageID int64, messageHash string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeRequestSentEvent(
				clientID,
				traceID,
				requestType,
				messageID,
				messageHash,
			))
		},
	}
}

// NewBridgeVerifyValidationFailedEvent builds an Event for when verify validation fails.
func NewBridgeVerifyValidationFailedEvent(clientID, traceID string, errorCode int, errorMessage string) Event {
	return Event{
		dispatch: func(client tonmetrics.AnalyticsClient) {
			client.SendEvent(client.CreateBridgeVerifyValidationFailedEvent(
				clientID,
				traceID,
				errorCode,
				errorMessage,
			))
		},
	}
}
