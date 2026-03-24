package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michelroberge/Claudine/backend/internal/model"
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

// ReadWithFallback reads from .ai-factory first; if not found, falls back to docs/{version}/.
func (r *ArtifactRepo) ReadWithFallback(hostDir, version string, stage model.StageName) (string, error) {
	content, err := r.Read(hostDir, stage)
	if err != nil {
		return "", err
	}
	if content != "" {
		return content, nil
	}
	return ReadStageDoc(hostDir, version, stage)
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

// ReadStageDoc reads a stage artifact from docs/{version}/{stage}/{stage}.md.
func ReadStageDoc(hostDir, version string, stage model.StageName) (string, error) {
	p := filepath.Join(hostDir, "docs", version, string(stage), string(stage)+".md")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read stage doc: %w", err)
	}
	return string(b), nil
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

// ReadBeadDoc reads a bead's execution output from docs/{version}/build/execution/{beadID}/execution.md.
func ReadBeadDoc(hostDir, version, beadID string) (string, error) {
	p := filepath.Join(hostDir, "docs", version, "build", "execution", beadID, "execution.md")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read bead doc: %w", err)
	}
	return string(b), nil
}

// ReadBeadNotes reads user notes for a bead.
func ReadBeadNotes(hostDir, version, beadID string) (string, error) {
	p := filepath.Join(hostDir, "docs", version, "build", "execution", beadID, "notes.md")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read bead notes: %w", err)
	}
	return string(b), nil
}

// WriteBeadNotes writes user notes for a bead.
func WriteBeadNotes(hostDir, version, beadID, content string) error {
	p := filepath.Join(hostDir, "docs", version, "build", "execution", beadID, "notes.md")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create docs bead dir: %w", err)
	}
	return os.WriteFile(p, []byte(content), 0644)
}

// ReadBeadChatHistory reads per-bead chat history.
func ReadBeadChatHistory(hostDir, version, beadID string) ([]model.Message, error) {
	p := filepath.Join(hostDir, "docs", version, "build", "execution", beadID, "chat.json")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read bead chat: %w", err)
	}
	var msgs []model.Message
	if err := json.Unmarshal(b, &msgs); err != nil {
		return nil, fmt.Errorf("parse bead chat: %w", err)
	}
	return msgs, nil
}

// AppendBeadChatMessage appends a message to per-bead chat history.
func AppendBeadChatMessage(hostDir, version, beadID string, msg model.Message) error {
	msgs, _ := ReadBeadChatHistory(hostDir, version, beadID)
	msgs = append(msgs, msg)
	b, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Errorf("marshal bead chat: %w", err)
	}
	p := filepath.Join(hostDir, "docs", version, "build", "execution", beadID, "chat.json")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create bead chat dir: %w", err)
	}
	return os.WriteFile(p, b, 0644)
}
