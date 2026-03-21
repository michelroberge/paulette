package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michelroberge/ai-app-factory/backend/internal/model"
)

type ArtifactRepo struct{}

func NewArtifactRepo() *ArtifactRepo {
	return &ArtifactRepo{}
}

func artifactPath(hostDir string, stage model.StageName) string {
	return filepath.Join(hostDir, factoryDir, string(stage), string(stage)+".md")
}

func (r *ArtifactRepo) Read(hostDir string, stage model.StageName) (string, error) {
	b, err := os.ReadFile(artifactPath(hostDir, stage))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read artifact: %w", err)
	}
	return string(b), nil
}

func (r *ArtifactRepo) Write(hostDir string, stage model.StageName, content string) error {
	p := artifactPath(hostDir, stage)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	return os.WriteFile(p, []byte(content), 0644)
}

func (r *ArtifactRepo) Exists(hostDir string, stage model.StageName) (bool, error) {
	content, err := r.Read(hostDir, stage)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(content) != "", nil
}
