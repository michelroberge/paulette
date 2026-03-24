package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const frameworkRelPath = ".paulette/ux/framework.json"

// FrameworkPath returns the absolute path to framework.json for a project.
func FrameworkPath(hostDir string) string {
	return filepath.Join(hostDir, frameworkRelPath)
}

// ReadFramework reads the framework config from disk.
// Returns the default (Tailwind) config when the file doesn't exist.
func ReadFramework(hostDir string) (*model.FrameworkConfig, error) {
	p := FrameworkPath(hostDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.FrameworkConfig{Framework: model.FrameworkTailwind}, nil
		}
		return nil, fmt.Errorf("read framework: %w", err)
	}
	var cfg model.FrameworkConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal framework: %w", err)
	}
	return &cfg, nil
}

// WriteFramework writes the framework config atomically (tmp file + rename).
func WriteFramework(hostDir string, cfg *model.FrameworkConfig) error {
	p := FrameworkPath(hostDir)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal framework: %w", err)
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
