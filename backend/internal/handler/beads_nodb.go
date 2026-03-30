package handler

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	beadsDir       = ".beads"
	beadsConfigFile = "config.yaml"
	noDBKey        = "no-db:"
)

// parseNoDBValue checks if a config line sets no-db and returns its value.
func parseNoDBValue(line string) (found bool, value bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") || !strings.HasPrefix(trimmed, noDBKey) {
		return false, false
	}
	val := strings.TrimSpace(strings.TrimPrefix(trimmed, noDBKey))
	return true, val == "true"
}

// ensureBeadsNoDB ensures .beads/config.yaml has no-db: true set.
// This makes JSONL the source of truth so beads state travels with the git repo.
// Safe to call even if .beads/ doesn't exist yet (no-op in that case).
func ensureBeadsNoDB(hostDir string) error {
	configPath := filepath.Join(hostDir, beadsDir, beadsConfigFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil // .beads doesn't exist yet
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read beads config: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// Check if already set
	for _, line := range lines {
		if found, val := parseNoDBValue(line); found && val {
			return nil // already enabled
		}
	}

	// Replace existing no-db line or append
	replaced := false
	for i, line := range lines {
		if found, _ := parseNoDBValue(line); found {
			lines[i] = "no-db: true"
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, "no-db: true")
	}

	if err := os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("write beads config: %w", err)
	}
	log.Printf("beads: enabled no-db mode in %s", configPath)
	return nil
}

// detectBeadsNoDB checks if a project's beads is configured in no-db mode.
// Returns (noDb, hasBeads) — hasBeads is true if .beads/ directory exists.
func detectBeadsNoDB(hostDir string) (noDb bool, hasBeads bool) {
	if _, err := os.Stat(filepath.Join(hostDir, beadsDir)); os.IsNotExist(err) {
		return false, false
	}

	data, err := os.ReadFile(filepath.Join(hostDir, beadsDir, beadsConfigFile))
	if err != nil {
		return false, true
	}

	for _, line := range strings.Split(string(data), "\n") {
		if found, val := parseNoDBValue(line); found {
			return val, true
		}
	}
	return false, true
}

