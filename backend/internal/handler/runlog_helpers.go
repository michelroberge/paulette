package handler

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
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
	// Auto-prune to keep only the last 10 runs per project.
	if _, err := fsrepo.PruneProjectRuns(logBase, safeProjectName, 10); err != nil {
		log.Printf("runlog: prune %s: %v", safeProjectName, err)
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

// runTrace carries run-log context through an operation so each agent step
// can write its raw LLM output and parse diagnostics into the run directory.
// Safe for concurrent use from parallel goroutines.
type runTrace struct {
	logBase     string
	projectName string
	runID       string
	mu          sync.Mutex
	steps       []model.StepLogRef
}

// newRunTrace creates a trace context. Returns nil if logging is disabled.
func newRunTrace(logBase, projectName, runID string) *runTrace {
	if logBase == "" || runID == "" {
		return nil
	}
	return &runTrace{
		logBase:     logBase,
		projectName: fsrepo.SanitizeProjectName(projectName),
		runID:       runID,
	}
}

// logStep writes a step trace file and records it in the step list.
// content is the raw LLM response; detail is a short parse summary (ok or error info).
func (t *runTrace) logStep(stepName, status, detail, content string, dur time.Duration) {
	if t == nil {
		return
	}
	filename, err := fsrepo.WriteRunStepLog(t.logBase, t.projectName, t.runID, stepName, content)
	if err != nil {
		log.Printf("runlog: step %s: %v", stepName, err)
		return
	}
	ref := model.StepLogRef{
		Step:     stepName,
		File:     filename,
		Status:   status,
		Detail:   detail,
		Duration: dur.Milliseconds(),
	}
	t.mu.Lock()
	t.steps = append(t.steps, ref)
	t.mu.Unlock()
}

// flush writes the accumulated step logs into the run's meta.json.
func (t *runTrace) flush() {
	if t == nil {
		return
	}
	t.mu.Lock()
	steps := make([]model.StepLogRef, len(t.steps))
	copy(steps, t.steps)
	t.mu.Unlock()

	if len(steps) == 0 {
		return
	}

	entry, err := fsrepo.ReadRunMeta(t.logBase, t.projectName, t.runID)
	if err != nil {
		log.Printf("runlog: flush steps %s: %v", t.runID, err)
		return
	}
	entry.StepLogs = steps
	if err := fsrepo.WriteRunMeta(t.logBase, entry); err != nil {
		log.Printf("runlog: flush steps write %s: %v", t.runID, err)
	}
}
