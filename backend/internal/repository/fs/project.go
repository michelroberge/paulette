package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const factoryDir = ".paulette"

type ProjectRepo struct{}

func NewProjectRepo() *ProjectRepo {
	return &ProjectRepo{}
}

func projectFilePath(hostDir string) string {
	return filepath.Join(hostDir, factoryDir, "project.json")
}

func (r *ProjectRepo) Init(hostDir string) error {
	stages := []string{"vision", "ux", "architecture", "build"}
	for _, stage := range stages {
		dir := filepath.Join(hostDir, factoryDir, stage)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create stage dir %s: %w", stage, err)
		}
	}
	return nil
}

func (r *ProjectRepo) Load(hostDir string) (*model.Project, error) {
	b, err := os.ReadFile(projectFilePath(hostDir))
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

func (r *ProjectRepo) Save(hostDir string, project *model.Project) error {
	b, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}
	return os.WriteFile(projectFilePath(hostDir), b, 0644)
}
