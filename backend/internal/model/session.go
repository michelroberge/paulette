package model

import "time"

// SessionKind identifies the type of operation that created a session.
type SessionKind string

const (
	SessionChat         SessionKind = "chat"
	SessionMock         SessionKind = "mock"
	SessionBeadGenerate SessionKind = "beads-generate"
	SessionBeadExecute  SessionKind = "beads-execute"
	SessionSummaryKind   SessionKind = "summary"
	SessionSkillAnalyze  SessionKind = "skills-analyze"
	SessionSkillObserve  SessionKind = "skills-observe"
)

// Session records the token usage of a single operation (chat, mock, bead run, etc.).
type Session struct {
	ID           string      `json:"id"`
	Stage        StageName   `json:"stage"`
	Kind         SessionKind `json:"kind"`
	Iteration    int         `json:"iteration"`
	StartedAt    time.Time   `json:"startedAt"`
	EndedAt      time.Time   `json:"endedAt"`
	InputTokens  int         `json:"inputTokens"`
	OutputTokens int         `json:"outputTokens"`
	TotalTokens  int         `json:"totalTokens"`
}

// SessionLog is the on-disk container for session records.
type SessionLog struct {
	Sessions []Session `json:"sessions"`
}

// StageSummary aggregates session data for a single stage.
type StageSummary struct {
	Count  int `json:"count"`
	Tokens int `json:"tokens"`
}

// SessionSummary is the API response for session queries.
type SessionSummary struct {
	Sessions   []Session                `json:"sessions"`
	ByStage    map[StageName]StageSummary `json:"byStage"`
	GrandTotal int                      `json:"grandTotal"`
}
