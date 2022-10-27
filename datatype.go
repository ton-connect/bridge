package main

type BridgeMessage struct {
	From    string `json:"from"`
	Message []byte `json:"message"`
}
