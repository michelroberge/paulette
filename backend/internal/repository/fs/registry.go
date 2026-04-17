package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/michelroberge/paulette/backend/internal/model"
)

type registryData struct {
	Projects []model.Project `json:"projects"`
}

type RegistryRepo struct {
	mu       sync.RWMutex
	filePath string
}

func NewRegistryRepo(registryDir string) (*RegistryRepo, error) {
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return nil, fmt.Errorf("create registry dir: %w", err)
	}

	filePath := filepath.Join(registryDir, "registry.json")

	// Create file if it doesn't exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		data := registryData{Projects: []model.Project{}}
		b, _ := json.MarshalIndent(data, "", "  ")
		if err := os.WriteFile(filePath, b, 0644); err != nil {
			return nil, fmt.Errorf("create registry file: %w", err)
		}
	}

	return &RegistryRepo{filePath: filePath}, nil
}

func (r *RegistryRepo) load() (*registryData, error) {
	b, err := os.ReadFile(r.filePath)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var data registryData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	// Migrate legacy stage names
	for i := range data.Projects {
		if data.Projects[i].CurrentStage == "review" {
			data.Projects[i].CurrentStage = model.StageComplete
		}
		if data.Projects[i].Iteration == 0 {
			data.Projects[i].Iteration = 1
		}
	}
	return &data, nil
}

func (r *RegistryRepo) save(data *registryData) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	return os.WriteFile(r.filePath, b, 0644)
}

func (r *RegistryRepo) List() ([]model.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, err := r.load()
	if err != nil {
		return nil, err
	}
	return data.Projects, nil
}

func (r *RegistryRepo) Get(id string) (*model.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, err := r.load()
	if err != nil {
		return nil, err
	}
	for i := range data.Projects {
		if data.Projects[i].ID == id {
			return &data.Projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %s not found", id)
}

func (r *RegistryRepo) Create(project *model.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return err
	}
	data.Projects = append(data.Projects, *project)
	return r.save(data)
}

func (r *RegistryRepo) Update(project *model.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return err
	}
	for i := range data.Projects {
		if data.Projects[i].ID == project.ID {
			data.Projects[i] = *project
			return r.save(data)
		}
	}
	return fmt.Errorf("project %s not found", project.ID)
}

func (r *RegistryRepo) UpdateFunc(id string, fn func(*model.Project) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return err
	}
	for i := range data.Projects {
		if data.Projects[i].ID == id {
			if err := fn(&data.Projects[i]); err != nil {
				return err
			}
			return r.save(data)
		}
	}
	return fmt.Errorf("project %s not found", id)
}

func (r *RegistryRepo) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return err
	}
	for i := range data.Projects {
		if data.Projects[i].ID == id {
			data.Projects = append(data.Projects[:i], data.Projects[i+1:]...)
			return r.save(data)
		}
	}
	return fmt.Errorf("project %s not found", id)
}
