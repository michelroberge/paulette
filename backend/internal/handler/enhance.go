package handler

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/Claudine/backend/internal/git"
	"github.com/michelroberge/Claudine/backend/internal/model"
	"github.com/michelroberge/Claudine/backend/internal/pipeline"
	"github.com/michelroberge/Claudine/backend/internal/repository"
)

type EnhanceHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	git          *git.Service
}

func NewEnhanceHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, gitSvc *git.Service) *EnhanceHandler {
	return &EnhanceHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		git:          gitSvc,
	}
}

type enhanceRequest struct {
	Vision      string `json:"vision"`
	VersionBump string `json:"versionBump"` // "major", "minor", "patch"
}

type enhanceResponse struct {
	Project  model.Project       `json:"project"`
	Pipeline model.PipelineState `json:"pipeline"`
}

func (h *EnhanceHandler) Enhance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	if project.CurrentStage != model.StageComplete {
		http.Error(w, "project must be at complete stage to enhance", http.StatusBadRequest)
		return
	}

	var req enhanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Vision == "" {
		http.Error(w, "vision is required", http.StatusBadRequest)
		return
	}
	if req.VersionBump != "major" && req.VersionBump != "minor" && req.VersionBump != "patch" {
		http.Error(w, "versionBump must be major, minor, or patch", http.StatusBadRequest)
		return
	}

	// Verify summary exists
	summaryPath := filepath.Join(project.HostDir, ".ai-factory", "summary.md")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		http.Error(w, "summary.md not found — complete the pipeline first", http.StatusBadRequest)
		return
	}

	// Archive current iteration
	archiveDir := filepath.Join(project.HostDir, ".ai-factory", "iterations", "v"+project.Version)
	if err := archiveIteration(project.HostDir, archiveDir); err != nil {
		http.Error(w, "failed to archive iteration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Bump version
	newVersion, err := bumpVersion(project.Version, req.VersionBump)
	if err != nil {
		http.Error(w, "failed to bump version: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Clear all stage artifacts and chat histories
	for _, s := range pipeline.StageOrder {
		if s == model.StageComplete {
			break
		}
		stageDir := filepath.Join(project.HostDir, ".ai-factory", string(s))
		chatFile := filepath.Join(stageDir, "chat-history.json")
		artifactFile := filepath.Join(stageDir, string(s)+".md")
		os.Remove(chatFile)
		os.Remove(artifactFile)
		if s == model.StageUX {
			os.Remove(filepath.Join(stageDir, "mock.html"))
			os.Remove(filepath.Join(stageDir, "framework.json"))
		}
		if s == model.StageBuild {
			os.Remove(filepath.Join(stageDir, "beads-graph.json"))
		}
	}

	// Update project
	project.CurrentStage = model.StageVision
	project.Version = newVersion
	project.Iteration++
	project.EnhancementVision = req.Vision
	project.UpdatedAt = time.Now()

	if err := h.registry.Update(project); err != nil {
		http.Error(w, "failed to update registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Tag the completed version before starting the new iteration
	tagName := "v" + project.Version
	if err := h.git.CreateTag(project.HostDir, tagName, fmt.Sprintf("Iteration %d complete", project.Iteration-1)); err != nil {
		log.Printf("git tag %s failed (may already exist): %v", tagName, err)
	}

	// Git commit the enhancement start
	commitMsg := fmt.Sprintf("enhance: start iteration %d (v%s)", project.Iteration, newVersion)
	if err := h.git.AddAllAndCommit(project.HostDir, commitMsg); err != nil {
		log.Printf("git commit enhance failed: %v", err)
	}

	state := pipeline.BuildPipelineState(project.CurrentStage)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enhanceResponse{
		Project:  *project,
		Pipeline: state,
	})
}

// archiveIteration copies current artifacts and summary to the archive directory.
func archiveIteration(hostDir, archiveDir string) error {
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}

	factoryBase := filepath.Join(hostDir, ".ai-factory")

	// Copy summary.md
	summaryPath := filepath.Join(factoryBase, "summary.md")
	if data, err := os.ReadFile(summaryPath); err == nil {
		os.WriteFile(filepath.Join(archiveDir, "summary.md"), data, 0644)
	}

	// Copy stage artifacts
	stages := []string{"vision", "ux", "architecture", "build"}
	for _, stage := range stages {
		srcDir := filepath.Join(factoryBase, stage)
		dstDir := filepath.Join(archiveDir, stage)
		if err := copyDir(srcDir, dstDir); err != nil {
			return fmt.Errorf("archive %s: %w", stage, err)
		}
	}

	return nil
}

// copyDir copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

// bumpVersion increments a semver string.
func bumpVersion(version, bump string) (string, error) {
	var major, minor, patch int
	n, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch)
	if err != nil || n != 3 {
		return "", fmt.Errorf("invalid version format: %s", version)
	}
	switch bump {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}
