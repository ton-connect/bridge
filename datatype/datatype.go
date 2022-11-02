package datatype

type SseMessage struct {
	EventId int64
	Message []byte
}

type BridgeMessage struct {
	From    string
	Message string
}
