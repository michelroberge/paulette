package model

import "time"

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
	StageArchitecture StageName = "architecture"
	StageBuild        StageName = "build"
	StageComplete     StageName = "complete"
)

type StageInfo struct {
	Name         StageName   `json:"name"`
	Status       StageStatus `json:"status"`
	ArtifactPath string      `json:"artifactPath"`
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
	Autonomous        bool                `json:"autonomous"`
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
