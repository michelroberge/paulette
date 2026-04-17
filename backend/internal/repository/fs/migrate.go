package fs

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// MigrateProjectDataIfNeeded copies legacy .paulette/ working-state into the
// new DataDir location the first time the server starts after an upgrade.
// It is safe to call repeatedly — it is a no-op when DataDir is already populated.
// The legacy source is left intact so the operator can verify before deleting.
func MigrateProjectDataIfNeeded(project *model.Project) error {
	if project.DataDir == "" {
		return nil // DataDir not computed yet — skip
	}

	// Check if DataDir already has content (e.g. project.json exists).
	if _, err := os.Stat(filepath.Join(project.DataDir, "project.json")); err == nil {
		return nil // already migrated
	}

	legacyDir := filepath.Join(project.HostDir, legacyFactoryDir)
	if _, err := os.Stat(legacyDir); os.IsNotExist(err) {
		return nil // no legacy data — brand-new project, nothing to migrate
	}

	log.Printf("migrate: copying %s → %s", legacyDir, project.DataDir)
	if err := copyDir(legacyDir, project.DataDir); err != nil {
		return fmt.Errorf("migrate project %s: %w", project.ID, err)
	}
	log.Printf("migrate: done for project %s (source left intact at %s)", project.ID, legacyDir)
	return nil
}

// copyDir recursively copies src into dst, creating dst if needed.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
