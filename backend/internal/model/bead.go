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
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Type        BeadType   `json:"type"`
	Status      BeadStatus `json:"status"`
	Priority    int        `json:"priority"`
	EpicID      string     `json:"epicId,omitempty"` // parent epic ID (tasks only)
	Deps        []string   `json:"deps"`
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
}
