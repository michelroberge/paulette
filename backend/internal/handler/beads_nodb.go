package handler

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
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

// MigrateAllBeadsNoDB iterates all registered projects and migrates any
// Dolt-backed beads to JSONL-only mode. This is idempotent — projects
// already in no-db mode are skipped. Run once at startup.
func MigrateAllBeadsNoDB(registry repository.RegistryRepo) {
	projects, err := registry.List()
	if err != nil {
		log.Printf("beads migration: failed to list projects: %v", err)
		return
	}

	migrated := 0
	for _, p := range projects {
		if migrateProjectBeadsNoDB(p) {
			migrated++
		}
	}
	if migrated > 0 {
		log.Printf("beads migration: migrated %d project(s) to no-db mode", migrated)
	}
}

// migrateProjectBeadsNoDB exports a single project's Dolt beads to JSONL,
// flips to no-db mode, and commits the JSONL to git. Returns true if migrated.
func migrateProjectBeadsNoDB(p model.Project) bool {
	noDb, hasBeads := detectBeadsNoDB(p.HostDir)
	if !hasBeads || noDb {
		return false // nothing to migrate
	}

	log.Printf("beads migration: migrating project %s (%s) to no-db mode", p.Name, p.ID)

	// Step 1: Export from Dolt while still in Dolt mode.
	// bd export writes issues to .beads/issues.jsonl from the current backend.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "export")
	cmd.Dir = p.HostDir
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("beads migration: bd export failed for %s: %v: %s", p.Name, err, out)
		// Fall back: try bd list to verify JSONL already has data
		cmd2 := exec.CommandContext(ctx, "bd", "list", "--all", "--json")
		cmd2.Dir = p.HostDir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil || strings.TrimSpace(string(out2)) == "[]" {
			log.Printf("beads migration: skipping %s — no beads data to export", p.Name)
			// Still flip to no-db even if empty, so future operations use JSONL
		}
	}

	// Step 2: Flip to no-db mode
	if err := ensureBeadsNoDB(p.HostDir); err != nil {
		log.Printf("beads migration: failed to set no-db for %s: %v", p.Name, err)
		return false
	}

	// Step 3: Commit the JSONL + config change to git
	fsrepo.CommitBeadsJSONL(p.HostDir)

	// Also commit the config.yaml change
	cmd = exec.CommandContext(context.Background(), "git", "add", "--",
		filepath.Join(beadsDir, beadsConfigFile))
	cmd.Dir = p.HostDir
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("beads migration: git add config.yaml failed for %s: %v: %s", p.Name, err, out)
	}
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m",
		"chore: migrate beads to no-db (JSONL-only) mode [skip ci]")
	cmd.Dir = p.HostDir
	if out, err := cmd.CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		if !strings.Contains(outStr, "nothing to commit") {
			log.Printf("beads migration: git commit failed for %s: %v: %s", p.Name, err, outStr)
		}
	}

	log.Printf("beads migration: project %s migrated successfully", p.Name)
	return true
}
