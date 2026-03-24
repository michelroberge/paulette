package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

const beadsGraphRelPath = ".paulette/build/beads-graph.json"

// BeadGraphPath returns the absolute path to beads-graph.json for a project.
func BeadGraphPath(hostDir string) string {
	return filepath.Join(hostDir, beadsGraphRelPath)
}

// ReadBeadGraph reads the beads graph from disk. Returns an empty graph if the file doesn't exist.
func ReadBeadGraph(hostDir string) (*model.BeadGraph, error) {
	p := BeadGraphPath(hostDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.BeadGraph{
				GeneratedAt: time.Time{},
				Beads:       []model.Bead{},
			}, nil
		}
		return nil, fmt.Errorf("read bead graph: %w", err)
	}
	var g model.BeadGraph
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("unmarshal bead graph: %w", err)
	}
	return &g, nil
}

// WriteBeadGraph writes the beads graph atomically (tmp file + rename).
func WriteBeadGraph(hostDir string, graph *model.BeadGraph) error {
	p := BeadGraphPath(hostDir)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	b, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bead graph: %w", err)
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
