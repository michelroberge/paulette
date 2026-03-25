package model

import "time"

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

type Message struct {
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
}

type ChatHistory struct {
	Messages []Message `json:"messages"`
	NextTurn string    `json:"nextTurn"` // "agent" | "user"
}
