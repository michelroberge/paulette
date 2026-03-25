package model

import "time"

type BeadStatus string

const (
	BeadStatusOpen       BeadStatus = "open"
	BeadStatusInProgress BeadStatus = "in_progress"
	BeadStatusReviewing  BeadStatus = "reviewing"
	BeadStatusClosed     BeadStatus = "closed"
	BeadStatusBlocked    BeadStatus = "blocked"
)

type BeadType string

const (
	BeadTypeEpic    BeadType = "epic"
	BeadTypeTask    BeadType = "task"
	BeadTypeFeature BeadType = "feature"
)

type Bead struct {
	ID                  string     `json:"id"`
	Title               string     `json:"title"`
	Description         string     `json:"description,omitempty"`
	Type                BeadType   `json:"type"`
	Status              BeadStatus `json:"status"`
	Priority            int        `json:"priority"`
	EpicID              string     `json:"epicId,omitempty"` // parent epic ID (tasks only)
	Deps                []string   `json:"deps"`
	Tags                []string   `json:"tags,omitempty"`
	TargetFiles         []string   `json:"targetFiles,omitempty"`
	JourneyRefs         []string   `json:"journeyRefs,omitempty"`
	ArchRefs            []string   `json:"archRefs,omitempty"`
	Tokens              int        `json:"tokens,omitempty"`
	PreExecutionCommit  string     `json:"preExecutionCommit,omitempty"` // git commit hash before execution started
}

type BeadGraph struct {
	GeneratedAt time.Time `json:"generatedAt"`
	ProjectID   string    `json:"projectId"`
	Beads       []Bead    `json:"beads"`
}

// ParsedBuildPlan is the intermediate structure Claude produces when parsing build.md.
type ParsedBuildPlan struct {
	Epics []ParsedEpic `json:"epics"`
}

type ParsedEpic struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Tasks       []ParsedTask `json:"tasks"`
}

type ParsedTask struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	DepsOn      []string `json:"depsOn"` // titles of tasks this task depends on
	Priority    int      `json:"priority"`
	Tags        []string `json:"tags"`
	TargetFiles []string `json:"targetFiles"`
	JourneyRefs []string `json:"journeyRefs"`
	ArchRefs    []string `json:"archRefs"`
}

// RelevantStages returns the stage artifacts that should be injected for a bead
// based on its tags. Empty tags returns all stages (safe fallback).
func RelevantStages(tags []string) []StageName {
	if len(tags) == 0 {
		return []StageName{StageVision, StageUX, StageArchitecture, StageBuild}
	}

	hasBackend := false
	hasFrontend := false
	hasInfra := false
	hasTesting := false

	for _, tag := range tags {
		switch tag {
		case "backend", "api", "database":
			hasBackend = true
		case "frontend", "styling":
			hasFrontend = true
		case "config", "devops":
			hasInfra = true
		case "testing":
			hasTesting = true
		}
	}

	// Mixed tags or unrecognized → all stages
	count := 0
	if hasBackend {
		count++
	}
	if hasFrontend {
		count++
	}
	if hasInfra {
		count++
	}
	if hasTesting {
		count++
	}
	if count > 1 {
		return []StageName{StageVision, StageUX, StageArchitecture, StageBuild}
	}

	// Vision and Build are always included
	switch {
	case hasBackend:
		return []StageName{StageVision, StageArchitecture, StageBuild}
	case hasFrontend:
		return []StageName{StageVision, StageUX, StageBuild}
	case hasInfra:
		return []StageName{StageBuild}
	case hasTesting:
		return []StageName{StageArchitecture, StageBuild}
	default:
		return []StageName{StageVision, StageUX, StageArchitecture, StageBuild}
	}
}
