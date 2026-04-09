package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/git"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/pipeline"
	"github.com/michelroberge/paulette/backend/internal/repository"
)

type EnhanceHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	git          *git.Service
	dataPath     string
}

func NewEnhanceHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, gitSvc *git.Service, dataPath string) *EnhanceHandler {
	return &EnhanceHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		git:          gitSvc,
		dataPath:     dataPath,
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

	project, err := h.EnhanceInternal(id, req.Vision, req.VersionBump)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "failed to") {
			status = http.StatusInternalServerError
		}
		http.Error(w, err.Error(), status)
		return
	}

	state := pipeline.BuildPipelineState(project.CurrentStage, nil, project.SummaryApproved)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enhanceResponse{
		Project:  *project,
		Pipeline: state,
	})
}

// EnhanceInternal runs the enhancement logic without HTTP plumbing.
func (h *EnhanceHandler) EnhanceInternal(projectID, vision, versionBump string) (*model.Project, error) {
	project, err := h.registry.Get(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	if project.CurrentStage != model.StageComplete {
		return nil, fmt.Errorf("project must be at complete stage to enhance")
	}

	summaryPath := filepath.Join(project.DataDir, "summary.md")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("summary.md not found — complete the pipeline first")
	}

	newVersion, err := bumpVersion(project.Version, versionBump)
	if err != nil {
		return nil, fmt.Errorf("failed to bump version: %w", err)
	}

	// Version isolation is now implicit: the old DataDir stays as the archive.
	// Create a fresh DataDir for the new version.
	newDataDir := model.ComputeDataDir(h.dataPath, project.ID, newVersion)
	if err := h.projectRepo.Init(newDataDir); err != nil {
		return nil, fmt.Errorf("failed to init new data dir: %w", err)
	}

	project.CurrentStage = model.StageVision
	project.Version = newVersion
	project.DataDir = newDataDir
	project.Iteration++
	project.EnhancementVision = vision
	project.SummaryReady = false
	project.SummaryApproved = false
	project.UpdatedAt = time.Now()

	if err := h.registry.Update(project); err != nil {
		return nil, fmt.Errorf("failed to update registry: %w", err)
	}
	if err := h.projectRepo.Save(project.DataDir, project); err != nil {
		return nil, fmt.Errorf("failed to save project: %w", err)
	}

	tagName := "v" + project.Version
	if err := h.git.CreateTag(project.HostDir, tagName, fmt.Sprintf("Iteration %d complete", project.Iteration-1)); err != nil {
		log.Printf("git tag %s failed (may already exist): %v", tagName, err)
	}

	commitMsg := fmt.Sprintf("enhance: start iteration %d (v%s)", project.Iteration, newVersion)
	if err := h.git.AddAllAndCommit(project.HostDir, commitMsg); err != nil {
		log.Printf("git commit enhance failed: %v", err)
	}

	branchName := fmt.Sprintf("pauline/iteration-%d", project.Iteration)
	if err := h.git.CreateAndCheckoutBranch(project.HostDir, branchName); err != nil {
		log.Printf("git create branch %s failed: %v", branchName, err)
	}

	return project, nil
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
