package datatype

type SseMessage struct {
	EventId int64
	Message []byte
	To      string
}

type BridgeMessage struct {
	From                string `json:"from"`
	Message             string `json:"message"`
	TraceId             string `json:"trace_id"`
	BridgeRequestSource string `json:"request_source,omitempty"`
}

type BridgeRequestSource struct {
	Origin    string `json:"origin"`
	IP        string `json:"ip"`
	Time      string `json:"time"`
	ClientID  string `json:"client_id"`
	UserAgent string `json:"user_agent"`
}
