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
	StageReview       StageName = "review"
	StageComplete     StageName = "complete"
)

type StageInfo struct {
	Name         StageName   `json:"name"`
	Status       StageStatus `json:"status"`
	ArtifactPath string      `json:"artifactPath"`
}

type Project struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Author            string    `json:"author"`
	Version           string    `json:"version"`
	HostDir           string    `json:"hostDir"`
	CurrentStage      StageName `json:"currentStage"`
	Iteration         int       `json:"iteration"`
	EnhancementVision string    `json:"enhancementVision,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type PipelineState struct {
	CurrentStage StageName   `json:"currentStage"`
	Stages       []StageInfo `json:"stages"`
}
