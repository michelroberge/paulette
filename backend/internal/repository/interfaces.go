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
	Delete(id string) error
}

// ProjectRepo manages project-level data within a project's host directory.
type ProjectRepo interface {
	Init(hostDir string) error
	Load(hostDir string) (*model.Project, error)
	Save(hostDir string, project *model.Project) error
}

// ArtifactRepo manages stage artifacts (markdown files) within a project.
type ArtifactRepo interface {
	Read(hostDir string, stage model.StageName) (string, error)
	ReadWithFallback(hostDir, version string, stage model.StageName) (string, error)
	Write(hostDir string, stage model.StageName, content string) error
	Exists(hostDir string, stage model.StageName) (bool, error)
}

// ChatRepo manages chat history for each stage within a project.
type ChatRepo interface {
	GetHistory(hostDir string, stage model.StageName) ([]model.Message, error)
	AppendMessage(hostDir string, stage model.StageName, msg model.Message) error
}

// ActivityRepo persists per-stage activity state to .paulette/activity.json.
// Activity state tracks in-progress and failed operations, and queued /btw messages.
type ActivityRepo interface {
	ReadActivity(hostDir string) (map[model.StageName]*model.StageActivity, error)
	SetActivity(hostDir string, stage model.StageName, a *model.StageActivity) error
	ClearActivity(hostDir string, stage model.StageName) error
	AppendBtw(hostDir string, stage model.StageName, message string, sentAt time.Time) error
	ClearBtw(hostDir string, stage model.StageName) ([]model.BtwMessage, error)
}
