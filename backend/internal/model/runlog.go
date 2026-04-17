package model

import "time"

// RunLogStatus indicates the outcome of a logged run.
type RunLogStatus string

const (
	RunLogRunning RunLogStatus = "running"
	RunLogSuccess RunLogStatus = "success"
	RunLogFailed  RunLogStatus = "failed"
)

// ArtifactRef points to a file produced by a run.
type ArtifactRef struct {
	Name string `json:"name"` // e.g. "mock.html", "vision.md"
	Path string `json:"path"` // relative to project HostDir
}

// StepLogRef records a single agent step's trace file within a run directory.
type StepLogRef struct {
	Step     string `json:"step"`               // e.g. "screen_planner", "component_planner_dashboard"
	File     string `json:"file"`               // filename within the run dir, e.g. "screen_planner.txt"
	Status   string `json:"status"`             // "ok" or "error"
	Detail   string `json:"detail,omitempty"`   // short parse/error summary
	Duration int64  `json:"durationMs,omitempty"`
}

// RunLogEntry records the full lifecycle of a single pipeline operation.
// Persisted at <logBase>/<project-name>/runs/<run-id>/meta.json.
type RunLogEntry struct {
	ID           string        `json:"id"`
	ProjectID    string        `json:"projectId"`
	ProjectName  string        `json:"projectName"`
	Stage        StageName     `json:"stage"`
	Operation    string        `json:"operation"`  // chat | mock | beads-generate | beads-execute | summary
	Attempt      int           `json:"attempt"`    // 1-based retry count for this project+stage+op
	Status       RunLogStatus  `json:"status"`
	StartedAt    time.Time     `json:"startedAt"`
	EndedAt      *time.Time    `json:"endedAt,omitempty"`
	DurationMs   int64         `json:"durationMs,omitempty"`
	Artifacts    []ArtifactRef `json:"artifacts,omitempty"`
	Error        string        `json:"error,omitempty"`
	ErrorLogPath string        `json:"errorLogPath,omitempty"` // relative path within run dir
	TokensTotal  int           `json:"tokensTotal,omitempty"`
	Notes        string        `json:"notes,omitempty"` // e.g. "3 epics, 12 tasks created"
	StepLogs     []StepLogRef  `json:"stepLogs,omitempty"`
}
