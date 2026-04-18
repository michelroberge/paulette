package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// BeadGraphPath returns the absolute path to beads-graph.json in the project data dir.
func BeadGraphPath(dataDir string) string {
	return filepath.Join(dataDir, "build", "beads-graph.json")
}

// BeadMeta holds Paulette-specific metadata stored in a bead's bd notes field.
type BeadMeta struct {
	Tags                []string `json:"tags,omitempty"`
	TargetFiles         []string `json:"targetFiles,omitempty"`
	JourneyRefs         []string `json:"journeyRefs,omitempty"`
	ArchRefs            []string `json:"archRefs,omitempty"`
	EpicID              string   `json:"epicId,omitempty"`
	Tokens              int      `json:"tokens,omitempty"`
	PreExecutionCommit  string   `json:"preExecutionCommit,omitempty"`
	PostExecutionCommit string   `json:"postExecutionCommit,omitempty"`
}

// ReadBeadMeta reads Paulette metadata from a bead's bd notes field.
func ReadBeadMeta(ctx context.Context, hostDir, beadID string) (*BeadMeta, error) {
	out, err := runBdFS(ctx, hostDir, "show", beadID, "--json")
	if err != nil {
		return &BeadMeta{}, nil
	}
	var items []struct {
		Notes string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(out), &items); err != nil || len(items) == 0 || items[0].Notes == "" {
		return &BeadMeta{}, nil
	}
	var meta BeadMeta
	if err := json.Unmarshal([]byte(items[0].Notes), &meta); err != nil {
		return &BeadMeta{}, nil
	}
	return &meta, nil
}

// WriteBeadMeta stores Paulette metadata into a bead's bd notes field.
func WriteBeadMeta(ctx context.Context, hostDir, beadID string, meta *BeadMeta) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal bead meta: %w", err)
	}
	_, err = runBdFS(ctx, hostDir, "update", beadID, "--notes", string(b))
	if err == nil {
		CommitBeadsJSONL(hostDir)
	}
	return err
}

// CommitBeadsJSONL stages and commits .beads/issues.jsonl if it has uncommitted changes.
// Non-fatal: logs errors but never returns them. Exported so handler package can call it.
func CommitBeadsJSONL(hostDir string) {
	jsonlPath := filepath.Join(".beads", "issues.jsonl")

	// Check if file has unstaged changes
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--quiet", "--", jsonlPath)
	cmd.Dir = hostDir
	unstaged := cmd.Run() != nil

	// Check if file has staged changes
	cmd2 := exec.CommandContext(context.Background(), "git", "diff", "--cached", "--quiet", "--", jsonlPath)
	cmd2.Dir = hostDir
	staged := cmd2.Run() != nil

	if !unstaged && !staged {
		return // clean
	}

	cmd = exec.CommandContext(context.Background(), "git", "add", "--", jsonlPath)
	cmd.Dir = hostDir
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("beads: git add issues.jsonl failed: %v: %s\n", err, out)
		return
	}

	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "chore: sync beads state [skip ci]",
		"--", jsonlPath)
	cmd.Dir = hostDir
	if out, err := cmd.CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		if !strings.Contains(outStr, "nothing to commit") {
			fmt.Printf("beads: git commit issues.jsonl failed: %v: %s\n", err, outStr)
		}
	}
}

// bdListEntry is the shape returned by bd list --all --json.
type bdListEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
	Dependencies []struct {
		IssueID   string `json:"issue_id"`
		DependsOn string `json:"depends_on_id"`
		Type      string `json:"type"`
	} `json:"dependencies"`
}

// bdShowEntry is the shape returned by bd show --json (superset of list, adds notes).
type bdShowEntry struct {
	bdListEntry
	Notes string `json:"notes"`
}

// ReadBdBeadGraph assembles a BeadGraph from bd: two CLI calls, no file.
func ReadBdBeadGraph(ctx context.Context, hostDir string) (*model.BeadGraph, error) {
	// Step 1: list all issues
	listOut, err := runBdFS(ctx, hostDir, "list", "--all", "--limit", "0", "--json")
	if err != nil || strings.TrimSpace(listOut) == "" || strings.TrimSpace(listOut) == "[]" {
		return &model.BeadGraph{Beads: []model.Bead{}}, nil
	}
	var entries []bdListEntry
	if err := json.Unmarshal([]byte(listOut), &entries); err != nil {
		return nil, fmt.Errorf("parse bd list: %w", err)
	}
	if len(entries) == 0 {
		return &model.BeadGraph{Beads: []model.Bead{}}, nil
	}

	// Step 2: show all issues to get notes (one call with all IDs)
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	showArgs := append([]string{"show", "--json"}, ids...)
	notesByID := make(map[string]string, len(entries))
	if showOut, err := runBdFS(ctx, hostDir, showArgs...); err == nil && strings.TrimSpace(showOut) != "" {
		var showEntries []bdShowEntry
		if json.Unmarshal([]byte(showOut), &showEntries) == nil {
			for _, se := range showEntries {
				notesByID[se.ID] = se.Notes
			}
		}
	}

	// Step 3: assemble Bead slice
	beads := make([]model.Bead, 0, len(entries))
	for _, e := range entries {
		b := model.Bead{
			ID:          e.ID,
			Title:       e.Title,
			Description: e.Description,
			Type:        model.BeadType(e.IssueType),
			Status:      model.BeadStatus(e.Status),
			Priority:    e.Priority,
			Deps:        []string{},
		}
		for _, dep := range e.Dependencies {
			switch dep.Type {
			case "parent-child":
				b.EpicID = dep.DependsOn
			case "blocks":
				b.Deps = append(b.Deps, dep.DependsOn)
			}
		}
		if notesStr := notesByID[e.ID]; notesStr != "" {
			var meta BeadMeta
			if json.Unmarshal([]byte(notesStr), &meta) == nil {
				b.Tags = meta.Tags
				b.TargetFiles = meta.TargetFiles
				b.JourneyRefs = meta.JourneyRefs
				b.ArchRefs = meta.ArchRefs
				b.Tokens = meta.Tokens
				b.PreExecutionCommit = meta.PreExecutionCommit
				b.PostExecutionCommit = meta.PostExecutionCommit
				if b.EpicID == "" {
					b.EpicID = meta.EpicID
				}
			}
		}
		beads = append(beads, b)
	}

	return &model.BeadGraph{
		GeneratedAt: time.Now(),
		Beads:       beads,
	}, nil
}

// ReadSingleBead reads a single bead from bd by ID, enriched with notes metadata.
func ReadSingleBead(ctx context.Context, hostDir, beadID string) (*model.Bead, error) {
	out, err := runBdFS(ctx, hostDir, "show", beadID, "--json")
	if err != nil || strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("bead not found: %s", beadID)
	}
	var items []bdShowEntry
	if err := json.Unmarshal([]byte(out), &items); err != nil || len(items) == 0 {
		return nil, fmt.Errorf("bead not found: %s", beadID)
	}
	e := items[0]
	b := model.Bead{
		ID:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		Type:        model.BeadType(e.IssueType),
		Status:      model.BeadStatus(e.Status),
		Priority:    e.Priority,
		Deps:        []string{},
	}
	for _, dep := range e.Dependencies {
		switch dep.Type {
		case "parent-child":
			b.EpicID = dep.DependsOn
		case "blocks":
			b.Deps = append(b.Deps, dep.DependsOn)
		}
	}
	if e.Notes != "" {
		var meta BeadMeta
		if json.Unmarshal([]byte(e.Notes), &meta) == nil {
			b.Tags = meta.Tags
			b.TargetFiles = meta.TargetFiles
			b.JourneyRefs = meta.JourneyRefs
			b.ArchRefs = meta.ArchRefs
			b.Tokens = meta.Tokens
			b.PreExecutionCommit = meta.PreExecutionCommit
			b.PostExecutionCommit = meta.PostExecutionCommit
			if b.EpicID == "" {
				b.EpicID = meta.EpicID
			}
		}
	}
	return &b, nil
}

// MigrateBeadGraphIfNeeded migrates beads-graph.json to bd notes if the file exists.
// Checks both the new dataDir location and the legacy hostDir/.paulette location.
// This is a one-time migration; the file is deleted on success.
func MigrateBeadGraphIfNeeded(ctx context.Context, hostDir, dataDir string) {
	// Prefer new location; fall back to legacy location inside the git repo.
	p := BeadGraphPath(dataDir)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		p = filepath.Join(hostDir, legacyFactoryDir, "build", "beads-graph.json")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return // file doesn't exist, nothing to migrate
	}

	var g struct {
		Beads []struct {
			ID                  string   `json:"id"`
			EpicID              string   `json:"epicId"`
			Tags                []string `json:"tags"`
			TargetFiles         []string `json:"targetFiles"`
			JourneyRefs         []string `json:"journeyRefs"`
			ArchRefs            []string `json:"archRefs"`
			Tokens              int      `json:"tokens"`
			PreExecutionCommit  string   `json:"preExecutionCommit"`
			PostExecutionCommit string   `json:"postExecutionCommit"`
		} `json:"beads"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		return
	}

	for _, bead := range g.Beads {
		meta := &BeadMeta{
			Tags:                bead.Tags,
			TargetFiles:         bead.TargetFiles,
			JourneyRefs:         bead.JourneyRefs,
			ArchRefs:            bead.ArchRefs,
			EpicID:              bead.EpicID,
			Tokens:              bead.Tokens,
			PreExecutionCommit:  bead.PreExecutionCommit,
			PostExecutionCommit: bead.PostExecutionCommit,
		}
		if meta.EpicID == "" && len(meta.Tags) == 0 && len(meta.TargetFiles) == 0 &&
			meta.Tokens == 0 && meta.PreExecutionCommit == "" {
			continue
		}
		WriteBeadMeta(ctx, hostDir, bead.ID, meta) //nolint:errcheck — best-effort migration
	}

	os.Remove(p)
}

func runBdFS(ctx context.Context, hostDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.Dir = hostDir
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}
