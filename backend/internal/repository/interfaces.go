package repository

import (
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// RegistryRepo manages the central project registry.
// The registry is an index of all known projects with their metadata.
type RegistryRepo interface {
	List() ([]model.Project, error)
	Get(id string) (*model.Project, error)
	Create(project *model.Project) error
	Update(project *model.Project) error
	// UpdateFunc atomically reads the project, applies fn, and writes it back
	// under a single lock. Use this to avoid lost-update races when multiple
	// goroutines modify the same project concurrently.
	UpdateFunc(id string, fn func(*model.Project) error) error
	Delete(id string) error
}

// ProjectRepo manages project-level data within a project's data directory.
type ProjectRepo interface {
	Init(dataDir string) error
	Load(dataDir string) (*model.Project, error)
	Save(dataDir string, project *model.Project) error
}

// ArtifactRepo manages stage artifacts (markdown files) within a project.
type ArtifactRepo interface {
	Read(dataDir string, stage model.StageName) (string, error)
	// ReadWithFallback reads from dataDir first; if not found, falls back to hostDir/docs/{version}/.
	ReadWithFallback(dataDir, hostDir, version string, stage model.StageName) (string, error)
	Write(dataDir string, stage model.StageName, content string) error
	Exists(dataDir string, stage model.StageName) (bool, error)
}

// ChatRepo manages chat history for each stage within a project.
type ChatRepo interface {
	GetHistory(dataDir string, stage model.StageName) ([]model.Message, error)
	AppendMessage(dataDir string, stage model.StageName, msg model.Message) error
}

// ActivityRepo persists per-stage activity state to {dataDir}/activity.json.
// Activity state tracks in-progress and failed operations, and queued /btw messages.
type ActivityRepo interface {
	ReadActivity(dataDir string) (map[model.StageName]*model.StageActivity, error)
	SetActivity(dataDir string, stage model.StageName, a *model.StageActivity) error
	ClearActivity(dataDir string, stage model.StageName) error
	AppendBtw(dataDir string, stage model.StageName, message string, sentAt time.Time) error
	ClearBtw(dataDir string, stage model.StageName) ([]model.BtwMessage, error)
}
