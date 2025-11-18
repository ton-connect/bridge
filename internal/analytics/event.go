package analytics

// EventType identifies the semantic meaning of an analytics signal.
type EventType string

const (
	// EventBridgeMessageExpired is emitted when a stored message expires before delivery.
	EventBridgeMessageExpired           EventType = "bridge_message_expired"
	EventBridgeMessageSent              EventType = "bridge_message_sent"
	EventBridgeMessageReceived          EventType = "bridge_message_received"
	EventBridgeMessageValidationFailed  EventType = "bridge_message_validation_failed"
	EventBridgeVerify                   EventType = "bridge_verify"
	EventBridgeEventsClientUnsubscribed EventType = "bridge_events_client_unsubscribed"
)

// Event represents a single analytics signal.
type Event struct {
	Type    EventType
	Payload any
}

// BridgeMessageExpiredPayload carries data for message-expired events.
type BridgeMessageExpiredPayload struct {
	ClientID    string
	TraceID     string
	RequestType string
	MessageID   int64
	MessageHash string
}

// NewBridgeMessageExpiredEvent builds an Event for a message expiration.
func NewBridgeMessageExpiredEvent(clientID, traceID, requestType string, messageID int64, messageHash string) Event {
	return Event{
		Type: EventBridgeMessageExpired,
		Payload: BridgeMessageExpiredPayload{
			ClientID:    clientID,
			TraceID:     traceID,
			RequestType: requestType,
			MessageID:   messageID,
			MessageHash: messageHash,
		},
	}
}

type BridgeMessageSentPayload struct {
	ClientID    string
	TraceID     string
	RequestType string
	MessageID   int64
	MessageHash string
}

func NewBridgeMessageSentEvent(clientID, traceID, requestType string, messageID int64, messageHash string) Event {
	return Event{
		Type: EventBridgeMessageSent,
		Payload: BridgeMessageSentPayload{
			ClientID:    clientID,
			TraceID:     traceID,
			RequestType: requestType,
			MessageID:   messageID,
			MessageHash: messageHash,
		},
	}
}

type BridgeMessageReceivedPayload struct {
	ClientID    string
	TraceID     string
	RequestType string
	MessageID   int64
	MessageHash string
}

func NewBridgeMessageReceivedEvent(clientID, traceID, requestType string, messageID int64, messageHash string) Event {
	return Event{
		Type: EventBridgeMessageReceived,
		Payload: BridgeMessageReceivedPayload{
			ClientID:    clientID,
			TraceID:     traceID,
			RequestType: requestType,
			MessageID:   messageID,
			MessageHash: messageHash,
		},
	}
}

type BridgeMessageValidationFailedPayload struct {
	ClientID    string
	TraceID     string
	RequestType string
	MessageHash string
}

func NewBridgeMessageValidationFailedEvent(clientID, traceID, requestType, messageHash string) Event {
	return Event{
		Type: EventBridgeMessageValidationFailed,
		Payload: BridgeMessageValidationFailedPayload{
			ClientID:    clientID,
			TraceID:     traceID,
			RequestType: requestType,
			MessageHash: messageHash,
		},
	}
}

type BridgeVerifyPayload struct {
	ClientID           string
	TraceID            string
	VerificationResult string
}

func NewBridgeVerifyEvent(clientID, traceID, verificationResult string) Event {
	return Event{
		Type: EventBridgeVerify,
		Payload: BridgeVerifyPayload{
			ClientID:           clientID,
			TraceID:            traceID,
			VerificationResult: verificationResult,
		},
	}
}

type BridgeEventsClientUnsubscribedPayload struct {
	ClientID string
	TraceID  string
}

func NewBridgeEventsClientUnsubscribedEvent(clientID, traceID string) Event {
	return Event{
		Type: EventBridgeEventsClientUnsubscribed,
		Payload: BridgeEventsClientUnsubscribedPayload{
			ClientID: clientID,
			TraceID:  traceID,
		},
	}
}
