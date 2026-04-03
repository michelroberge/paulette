package handler

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/michelroberge/paulette/backend/internal/model"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
)

// startRunLog creates a new run directory and writes a meta.json with status=running.
// Returns the run log entry ID.
func startRunLog(logBase string, project *model.Project, stage model.StageName, op string) string {
	if logBase == "" {
		return ""
	}
	safeProjectName := fsrepo.SanitizeProjectName(project.Name)
	attempt := fsrepo.CountProjectRunsForOp(logBase, safeProjectName, stage, op) + 1
	entry := model.RunLogEntry{
		ID:          uuid.NewString(),
		ProjectID:   project.ID,
		ProjectName: safeProjectName,
		Stage:       stage,
		Operation:   op,
		Attempt:     attempt,
		Status:      model.RunLogRunning,
		StartedAt:   time.Now(),
	}
	if err := fsrepo.WriteRunMeta(logBase, entry); err != nil {
		log.Printf("runlog: start %s/%s: %v", stage, op, err)
		return ""
	}
	return entry.ID
}

// successRunLog updates a run entry to success with artifacts, token count, and notes.
func successRunLog(logBase, projectName, runID string, artifacts []model.ArtifactRef, tokens int, notes string) {
	if logBase == "" || runID == "" {
		return
	}
	safeProjectName := fsrepo.SanitizeProjectName(projectName)
	entry, err := fsrepo.ReadRunMeta(logBase, safeProjectName, runID)
	if err != nil {
		log.Printf("runlog: read for success %s: %v", runID, err)
		return
	}
	now := time.Now()
	entry.Status = model.RunLogSuccess
	entry.EndedAt = &now
	entry.DurationMs = now.Sub(entry.StartedAt).Milliseconds()
	entry.Artifacts = artifacts
	entry.TokensTotal = tokens
	entry.Notes = notes
	if err := fsrepo.WriteRunMeta(logBase, entry); err != nil {
		log.Printf("runlog: write success %s: %v", runID, err)
	}
}

// failRunLog updates a run entry to failed and writes error.log.
// Returns the relative path to the error log file (e.g. "error.log").
func failRunLog(logBase, projectName, runID, errMsg string) string {
	if logBase == "" || runID == "" {
		return ""
	}
	safeProjectName := fsrepo.SanitizeProjectName(projectName)
	entry, err := fsrepo.ReadRunMeta(logBase, safeProjectName, runID)
	if err != nil {
		log.Printf("runlog: read for fail %s: %v", runID, err)
		return ""
	}
	now := time.Now()
	entry.Status = model.RunLogFailed
	entry.EndedAt = &now
	entry.DurationMs = now.Sub(entry.StartedAt).Milliseconds()
	entry.Error = errMsg
	entry.ErrorLogPath = "error.log"

	// Write detailed error log
	detail := fmt.Sprintf("Time:      %s\nProject:   %s\nStage:     %s\nOperation: %s\nAttempt:   %d\n\nError:\n%s\n",
		now.Format(time.RFC3339),
		entry.ProjectName,
		entry.Stage,
		entry.Operation,
		entry.Attempt,
		errMsg,
	)
	if wErr := fsrepo.WriteRunErrorLog(logBase, safeProjectName, runID, detail); wErr != nil {
		log.Printf("runlog: write error.log %s: %v", runID, wErr)
		entry.ErrorLogPath = ""
	}

	if err := fsrepo.WriteRunMeta(logBase, entry); err != nil {
		log.Printf("runlog: write fail %s: %v", runID, err)
	}
	return entry.ErrorLogPath
}

// expectedArtifacts returns artifact refs that exist on disk for a given stage+operation.
func expectedArtifacts(hostDir string, stage model.StageName, op string) []model.ArtifactRef {
	var refs []model.ArtifactRef
	switch op {
	case "chat":
		rel := filepath.Join(".paulette", string(stage), string(stage)+".md")
		if fileExists(filepath.Join(hostDir, rel)) {
			refs = append(refs, model.ArtifactRef{Name: string(stage) + ".md", Path: rel})
		}
	case "mock":
		rel := filepath.Join(".paulette", "ux", "mock.html")
		if fileExists(filepath.Join(hostDir, rel)) {
			refs = append(refs, model.ArtifactRef{Name: "mock.html", Path: rel})
		}
	case "beads-generate":
		rel := filepath.Join(".beads", "issues.jsonl")
		if fileExists(filepath.Join(hostDir, rel)) {
			refs = append(refs, model.ArtifactRef{Name: "issues.jsonl", Path: rel})
		}
	case "summary":
		rel := filepath.Join(".paulette", "summary.md")
		if fileExists(filepath.Join(hostDir, rel)) {
			refs = append(refs, model.ArtifactRef{Name: "summary.md", Path: rel})
		}
	}
	return refs
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
