package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// writeJSONAtomic marshals v to indented JSON and writes it to path using the
// write-to-temp-then-rename pattern to guarantee atomicity.  perm sets the
// file mode applied to the temp file before the rename.
//
// This function is shared across the provider package so that ConnectionStore
// and StageConfigStore do not each define their own copy.
func writeJSONAtomic(path string, v any, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write to a temp file in the same directory so the rename is on the same
	// filesystem and therefore atomic.
	tmp, err := os.CreateTemp(dir, ".tmp-paulette-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Ensure we clean up on any error path.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to %s: %w", path, err)
	}

	committed = true
	return nil
}
