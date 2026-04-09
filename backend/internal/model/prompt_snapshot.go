package model

import "time"

// PromptSnapshot is the full interaction record written to prompt.json in each chat run dir.
// It captures everything sent to the LLM and the raw response, enabling full replay/inspection.
type PromptSnapshot struct {
	MessageID    string    `json:"messageId"`              // UUID of the assistant response message
	Stage        string    `json:"stage"`
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model"`
	ConnectionID string    `json:"connectionId,omitempty"`
	SystemPrompt string    `json:"systemPrompt"`
	RAGContext   string    `json:"ragContext,omitempty"`
	RAGSources   any       `json:"ragSources,omitempty"` // []rag.SourceRef, kept as any to avoid import cycle
	History      []Message `json:"history"`              // conversation sent to LLM after context strategy
	UserMessage  string    `json:"userMessage"`
	RawResponse  string    `json:"rawResponse"` // full model output before artifact extraction
}
