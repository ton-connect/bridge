package datatype

type SseMessage struct {
	EventId int64
	Message []byte
	To      string
}

type BridgeMessage struct {
	From    string `json:"from"`
	Message string `json:"message"`
}
