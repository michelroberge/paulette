package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const summaryFile = "summary.md"

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

// ReadWithFallback reads from .paulette first; if not found, falls back to docs/{version}/.
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

// WriteSummaryDoc writes the iteration summary to docs/{version}/summary.md.
func WriteSummaryDoc(hostDir, version, content string) error {
	p := filepath.Join(hostDir, "docs", version, summaryFile)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create docs version dir: %w", err)
	}
	return os.WriteFile(p, []byte(content), 0644)
}

// ReadSummaryDoc reads the iteration summary from docs/{version}/summary.md,
// falling back to .paulette/summary.md for backward compatibility.
func ReadSummaryDoc(hostDir, version string) (string, error) {
	docsPath := filepath.Join(hostDir, "docs", version, summaryFile)
	if b, err := os.ReadFile(docsPath); err == nil {
		return string(b), nil
	}
	fallbackPath := filepath.Join(hostDir, ".paulette", summaryFile)
	b, err := os.ReadFile(fallbackPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read summary: %w", err)
	}
	return string(b), nil
}

// WriteMockDoc copies the UX mock HTML to docs/{version}/ux/mock.html.
func WriteMockDoc(hostDir, version string, content []byte) error {
	p := filepath.Join(hostDir, "docs", version, "ux", "mock.html")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create docs ux dir: %w", err)
	}
	return os.WriteFile(p, content, 0644)
}

// ReadMockDoc reads the UX mock HTML from docs/{version}/ux/mock.html.
func ReadMockDoc(hostDir, version string) ([]byte, error) {
	p := filepath.Join(hostDir, "docs", version, "ux", "mock.html")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read mock doc: %w", err)
	}
	return b, nil
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

// WriteReadme generates a README.md at the project root with metadata and links to docs.
func WriteReadme(project *model.Project) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", project.Name)
	fmt.Fprintf(&b, "**Version:** %s  \n", project.Version)
	fmt.Fprintf(&b, "**Author:** %s\n", project.Author)

	hasCommands := len(project.DevCommands) > 0 || len(project.BuildCommands) > 0 || len(project.RunCommands) > 0
	if hasCommands {
		b.WriteString("\n## How to Use\n")
		writeCommandSection(&b, "Development", project.DevCommands)
		writeCommandSection(&b, "Build", project.BuildCommands)
		writeCommandSection(&b, "Run", project.RunCommands)
	}

	v := project.Version
	b.WriteString("\n## Documentation\n\n")
	fmt.Fprintf(&b, "- [Vision](docs/%s/vision/vision.md)\n", v)
	fmt.Fprintf(&b, "- [UX Design](docs/%s/ux/ux.md)\n", v)
	fmt.Fprintf(&b, "- [UX Mock](docs/%s/ux/mock.html)\n", v)
	fmt.Fprintf(&b, "- [Architecture](docs/%s/architecture/architecture.md)\n", v)
	fmt.Fprintf(&b, "- [Iteration Summary](docs/%s/summary.md)\n", v)

	p := filepath.Join(project.HostDir, "README.md")
	return os.WriteFile(p, []byte(b.String()), 0644)
}

func writeCommandSection(b *strings.Builder, title string, cmds []string) {
	if len(cmds) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n```sh\n", title)
	for _, c := range cmds {
		fmt.Fprintln(b, c)
	}
	b.WriteString("```\n")
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
