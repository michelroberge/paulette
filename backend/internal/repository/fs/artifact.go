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

// WriteStageDoc mirrors a stage artifact to docs/{version}/{stage}/{stage}.md in the target repo.
func WriteStageDoc(hostDir, version string, stage model.StageName, content string) error {
	p := filepath.Join(hostDir, "docs", version, string(stage), string(stage)+".md")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create docs stage dir: %w", err)
	}
	return os.WriteFile(p, []byte(content), 0644)
}

// WriteBeadDoc mirrors a bead execution's full response to docs/{version}/build/execution/{beadID}/execution.md.
func WriteBeadDoc(hostDir, version, beadID, content string) error {
	p := filepath.Join(hostDir, "docs", version, "build", "execution", beadID, "execution.md")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create docs bead dir: %w", err)
	}
	return os.WriteFile(p, []byte(content), 0644)
}
