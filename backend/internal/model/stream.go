package model

// StreamEvent represents an event sent to the client via SSE.
// Event types: "chunk", "artifact", "done", "error", "tokens", "plan_limit", "log".
type StreamEvent struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Tokens  int    `json:"tokens,omitempty"`
}
