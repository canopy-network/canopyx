package types

// WSClientMessage represents a message from WebSocket client
type WSClientMessage struct {
	Action  string `json:"action"`
	ChainID string `json:"chainId"`
}

// WSServerMessage represents a message to WebSocket client
type WSServerMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}
