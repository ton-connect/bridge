package datatype

type SseMessage struct {
	EventId int64
	Message []byte
	TraceId string
}

type BridgeMessage struct {
	From    string `json:"from"`
	Message string `json:"message"`
	TraceId string `json:"trace_id"`
}
