package rag

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// stageToCollection maps pipeline stages to RAG collection names.
func stageToCollection(stage model.StageName) string {
	switch stage {
	case model.StageVision, model.StageUX:
		return "specs"
	case model.StageArchitecture:
		return "specs"
	case model.StageBuild:
		return "docs"
	default:
		return "docs"
	}
}

// IngestArtifact sends an approved stage artifact to the RAG pipeline.
// The artifact content is written to a temp file so the RAG pipeline can
// ingest it via its filesystem path endpoint.
func IngestArtifact(ctx context.Context, client *Client, project *model.Project, stage model.StageName, content string) error {
	if client == nil || !client.IsHealthy() {
		return nil
	}

	// Write artifact to a temp file for the RAG pipeline to read.
	tmpDir, err := os.MkdirTemp("", "paulette-rag-*")
	if err != nil {
		return fmt.Errorf("rag ingest: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	filename := string(stage) + ".md"
	tmpFile := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("rag ingest: write temp file: %w", err)
	}

	collection := stageToCollection(stage)
	resp, err := client.Ingest(ctx, IngestRequest{
		Path: tmpDir,
		Metadata: map[string]any{
			"project":             project.Name,
			"stage":               string(stage),
			"version":             project.Version,
			"type":                "artifact",
			"paulette_project_id": project.ID,
		},
		Options: &IngestOptions{
			Collection: collection,
			Recursive:  boolPtr(false),
		},
	})
	if err != nil {
		return fmt.Errorf("rag ingest artifact %s/%s: %w", project.Name, stage, err)
	}

	if resp != nil {
		log.Printf("RAG ingest started for %s/%s: job=%s collection=%s", project.Name, stage, resp.JobID, collection)
	}
	return nil
}

// IngestProjectCode sends the project's source code directory to the RAG pipeline.
// This is typically called when the pipeline reaches StageComplete.
func IngestProjectCode(ctx context.Context, client *Client, project *model.Project) error {
	if client == nil || !client.IsHealthy() {
		return nil
	}

	resp, err := client.Ingest(ctx, IngestRequest{
		Path: project.HostDir,
		Metadata: map[string]any{
			"project":             project.Name,
			"version":             project.Version,
			"type":                "code",
			"paulette_project_id": project.ID,
		},
		Options: &IngestOptions{
			Collection: "code",
			Recursive:  boolPtr(true),
			ExcludePatterns: []string{
				"**/.paulette/**",
				"**/.git/**",
				"**/node_modules/**",
				"**/__pycache__/**",
				"**/dist/**",
				"**/build/**",
				"**/.venv/**",
				"**/vendor/**",
			},
			EnableRouting: boolPtr(true),
		},
	})
	if err != nil {
		return fmt.Errorf("rag ingest project code %s: %w", project.Name, err)
	}

	if resp != nil {
		log.Printf("RAG bulk code ingest started for %s: job=%s", project.Name, resp.JobID)
	}
	return nil
}

// IngestBeadCode sends the target files for a completed bead to the RAG pipeline.
func IngestBeadCode(ctx context.Context, client *Client, project *model.Project, bead model.Bead) error {
	if client == nil || !client.IsHealthy() {
		return nil
	}

	if len(bead.TargetFiles) == 0 {
		return nil
	}

	// Ingest the project directory but filter to just the bead's target files.
	// Since the RAG pipeline ingests directories, we ingest the full project
	// dir and rely on the file-level dedup (content hash) to skip unchanged files.
	resp, err := client.Ingest(ctx, IngestRequest{
		Path: project.HostDir,
		Metadata: map[string]any{
			"project":             project.Name,
			"version":             project.Version,
			"bead_id":             bead.ID,
			"bead_title":          bead.Title,
			"type":                "bead_code",
			"paulette_project_id": project.ID,
		},
		Options: &IngestOptions{
			Collection:    "code",
			Recursive:     boolPtr(true),
			EnableRouting: boolPtr(true),
			ExcludePatterns: []string{
				"**/.paulette/**",
				"**/.git/**",
				"**/node_modules/**",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("rag ingest bead code %s/%s: %w", project.Name, bead.ID, err)
	}

	if resp != nil {
		log.Printf("RAG bead code ingest started for %s/%s: job=%s", project.Name, bead.ID, resp.JobID)
	}
	return nil
}

func boolPtr(b bool) *bool { return &b }
