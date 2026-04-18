package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// runDir returns the directory for a specific run's files.
// Layout: <logBase>/<project-name>/runs/<run-id>/
func runDir(logBase, projectName, runID string) string {
	return filepath.Join(logBase, projectName, "runs", runID)
}

// WriteRunMeta creates the run directory (if needed) and writes meta.json.
// It uses entry.ProjectName and entry.ID to derive the path.
func WriteRunMeta(logBase string, entry model.RunLogEntry) error {
	dir := runDir(logBase, entry.ProjectName, entry.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0644)
}

// WriteRunErrorLog writes the full error detail to error.log within the run dir.
func WriteRunErrorLog(logBase, projectName, runID, content string) error {
	dir := runDir(logBase, projectName, runID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "error.log"), []byte(content), 0644)
}

// WriteRunStepLog writes a step trace file into the run directory and returns the filename.
func WriteRunStepLog(logBase, projectName, runID, stepName, content string) (string, error) {
	dir := runDir(logBase, projectName, runID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filename := stepName + ".txt"
	return filename, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}

// WriteRunPromptLog writes a prompt snapshot JSON file into the run directory and returns the filename.
func WriteRunPromptLog(logBase, projectName, runID string, snapshot any) (string, error) {
	dir := runDir(logBase, projectName, runID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	filename := "prompt.json"
	return filename, os.WriteFile(filepath.Join(dir, filename), data, 0644)
}

// ReadProjectRuns reads all run meta.json files for a project, sorted newest-first.
func ReadProjectRuns(logBase, projectName string) ([]model.RunLogEntry, error) {
	pattern := filepath.Join(logBase, projectName, "runs", "*", "meta.json")
	return readByGlob(pattern)
}

// ReadAllRuns reads all run meta.json files across all projects, sorted newest-first.
func ReadAllRuns(logBase string) ([]model.RunLogEntry, error) {
	pattern := filepath.Join(logBase, "*", "runs", "*", "meta.json")
	return readByGlob(pattern)
}

func readByGlob(pattern string) ([]model.RunLogEntry, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	entries := make([]model.RunLogEntry, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}
		var entry model.RunLogEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue // skip corrupt files
		}
		entries = append(entries, entry)
	}

	// Sort newest-first
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartedAt.After(entries[j].StartedAt)
	})

	return entries, nil
}

// CountProjectRunsForOp counts how many runs exist for a given project+stage+operation.
// Used to determine the attempt number for a new run.
func CountProjectRunsForOp(logBase, projectName string, stage model.StageName, op string) int {
	all, err := ReadProjectRuns(logBase, projectName)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range all {
		if e.Stage == stage && e.Operation == op {
			count++
		}
	}
	return count
}

// ReadRunMeta reads a single run's meta.json.
func ReadRunMeta(logBase, projectName, runID string) (model.RunLogEntry, error) {
	path := filepath.Join(runDir(logBase, projectName, runID), "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return model.RunLogEntry{}, err
	}
	var entry model.RunLogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return model.RunLogEntry{}, err
	}
	return entry, nil
}

// ReadRunTraceFile reads a trace file from a run directory.
// The filename is validated to prevent path traversal.
func ReadRunTraceFile(logBase, projectName, runID, filename string) ([]byte, error) {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || filename == ".." {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(runDir(logBase, projectName, runID), filename)
	return os.ReadFile(path)
}

// DeleteRun removes an entire run directory.
func DeleteRun(logBase, projectName, runID string) error {
	dir := runDir(logBase, projectName, runID)
	return os.RemoveAll(dir)
}

// PruneProjectRuns keeps only the most recent `keep` runs for a project,
// deleting older ones. Returns the number of deleted runs.
func PruneProjectRuns(logBase, projectName string, keep int) (int, error) {
	entries, err := ReadProjectRuns(logBase, projectName)
	if err != nil {
		return 0, err
	}
	if len(entries) <= keep {
		return 0, nil
	}
	// entries are already sorted newest-first
	toDelete := entries[keep:]
	deleted := 0
	for _, e := range toDelete {
		if err := DeleteRun(logBase, e.ProjectName, e.ID); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// SanitizeProjectName replaces characters that would be problematic in a directory name.
func SanitizeProjectName(name string) string {
	// Replace slashes, colons, and other problematic chars with dashes
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	result := replacer.Replace(name)
	if result == "" {
		return "unknown"
	}
	return result
}
