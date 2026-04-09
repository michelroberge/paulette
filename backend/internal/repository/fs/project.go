package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// legacyFactoryDir is the old working-state directory inside the git repo.
// Used only for migration — new code uses project.DataDir exclusively.
const legacyFactoryDir = ".paulette"

type ProjectRepo struct{}

func NewProjectRepo() *ProjectRepo {
	return &ProjectRepo{}
}

func projectFilePath(dataDir string) string {
	return filepath.Join(dataDir, "project.json")
}

func (r *ProjectRepo) Init(dataDir string) error {
	stages := []string{"vision", "ux", "architecture", "build", "skills"}
	for _, stage := range stages {
		dir := filepath.Join(dataDir, stage)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create stage dir %s: %w", stage, err)
		}
	}
	return nil
}

func (r *ProjectRepo) Load(dataDir string) (*model.Project, error) {
	b, err := os.ReadFile(projectFilePath(dataDir))
	if err != nil {
		return nil, fmt.Errorf("read project.json: %w", err)
	}
	var project model.Project
	if err := json.Unmarshal(b, &project); err != nil {
		return nil, fmt.Errorf("parse project.json: %w", err)
	}
	// Backward compatibility: default iteration to 1 for pre-enhancement projects
	if project.Iteration == 0 {
		project.Iteration = 1
	}
	// Migration: review stage was removed — advance to complete
	if project.CurrentStage == "review" {
		project.CurrentStage = model.StageComplete
	}
	return &project, nil
}

func (r *ProjectRepo) Save(dataDir string, project *model.Project) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	b, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}
	return os.WriteFile(projectFilePath(dataDir), b, 0644)
}
