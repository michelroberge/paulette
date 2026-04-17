package model

import "time"

// BtwMessage is a queued follow-up message the user sent while an agent was running.
type BtwMessage struct {
	Message string    `json:"message"`
	SentAt  time.Time `json:"sentAt"`
}

// StageActivity records the current operation running (or last failed) within a stage.
// Persisted to .paulette/activity.json so it survives server restarts.
type StageActivity struct {
	Operation  string       `json:"operation"`          // "chat", "mock", "beads-generate", "beads-execute", "summary"
	Status     string       `json:"status"`             // "running" | "failed"
	StartedAt  time.Time    `json:"startedAt"`
	Error      string       `json:"error,omitempty"`
	PendingBtw []BtwMessage `json:"pendingBtw,omitempty"` // queued follow-up messages
}

type StageStatus string

const (
	StageStatusLocked   StageStatus = "locked"
	StageStatusActive   StageStatus = "active"
	StageStatusApproved StageStatus = "approved"
)

type StageName string

const (
	StageVision       StageName = "vision"
	StageUX           StageName = "ux"
	StageUI           StageName = "ui"
	StageArchitecture StageName = "architecture"
	StageBuild        StageName = "build"
	StageComplete     StageName = "complete"
)

type StageInfo struct {
	Name         StageName      `json:"name"`
	Status       StageStatus    `json:"status"`
	ArtifactPath string         `json:"artifactPath"`
	Activity     *StageActivity `json:"activity,omitempty"`
}

type Project struct {
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Author            string              `json:"author"`
	Version           string              `json:"version"`
	HostDir           string              `json:"hostDir"`
	CurrentStage      StageName           `json:"currentStage"`
	Iteration         int                 `json:"iteration"`
	EnhancementVision string              `json:"enhancementVision,omitempty"`
	Imported          bool                `json:"imported,omitempty"`
	SummaryReady      bool                `json:"summaryReady"`
	SummaryApproved   bool                `json:"summaryApproved,omitempty"`
	Autonomous        bool                `json:"autonomous"`
	BaseBranch        string              `json:"baseBranch,omitempty"`
	DevCommands       []string            `json:"devCommands,omitempty"`
	BuildCommands     []string            `json:"buildCommands,omitempty"`
	RunCommands       []string            `json:"runCommands,omitempty"`
	SummaryTokens     int                 `json:"summaryTokens,omitempty"`
	StageTokens       map[StageName]int   `json:"stageTokens,omitempty"`
	CreatedAt         time.Time           `json:"createdAt"`
	UpdatedAt         time.Time           `json:"updatedAt"`
}

// AddStageTokens atomically adds tokens to a stage counter.
// It initialises the map if nil.
func (p *Project) AddStageTokens(stage StageName, n int) {
	if n <= 0 {
		return
	}
	if p.StageTokens == nil {
		p.StageTokens = make(map[StageName]int)
	}
	p.StageTokens[stage] += n
}

type PipelineState struct {
	CurrentStage StageName   `json:"currentStage"`
	Stages       []StageInfo `json:"stages"`
}
