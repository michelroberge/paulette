package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/michelroberge/claudette/backend/internal/git"
	"github.com/michelroberge/claudette/backend/internal/model"
	"github.com/michelroberge/claudette/backend/internal/repository"
)

type ProjectHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
	git          *git.Service
}

func NewProjectHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo, gitSvc *git.Service) *ProjectHandler {
	return &ProjectHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
		git:          gitSvc,
	}
}

type createProjectRequest struct {
	Name    string `json:"name"`
	Author  string `json:"author"`
	HostDir string `json:"hostDir"`
	Version string `json:"version"`
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.HostDir == "" {
		http.Error(w, "name and hostDir are required", http.StatusBadRequest)
		return
	}

	version := req.Version
	if version == "" {
		version = "0.1.0"
	}

	now := time.Now()
	project := &model.Project{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Author:       req.Author,
		Version:      version,
		HostDir:      req.HostDir,
		CurrentStage: model.StageVision,
		Iteration:    1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Ensure host directory exists before git init
	if err := os.MkdirAll(req.HostDir, 0755); err != nil {
		http.Error(w, "failed to create host directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Git init the host directory if not already a git repo
	if _, err := os.Stat(filepath.Join(req.HostDir, ".git")); os.IsNotExist(err) {
		if err := h.git.Init(req.HostDir); err != nil {
			http.Error(w, "git init failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Initialize beads issue tracking
	if _, err := os.Stat(filepath.Join(req.HostDir, ".beads")); os.IsNotExist(err) {
		cmd := exec.Command("bd", "init")
		cmd.Dir = req.HostDir
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("bd init failed in %s: %v: %s", req.HostDir, err, out)
		}
	}

	// Initialize .ai-factory directory structure
	if err := h.projectRepo.Init(req.HostDir); err != nil {
		http.Error(w, "failed to init project directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Save project.json inside the host dir
	if err := h.projectRepo.Save(req.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Register in central registry
	if err := h.registry.Create(project); err != nil {
		http.Error(w, "failed to register project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(project)
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	projects, err := h.registry.List()
	if err != nil {
		http.Error(w, "failed to list projects: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

type patchProjectRequest struct {
	Autonomous *bool `json:"autonomous"`
}

func (h *ProjectHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req patchProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Autonomous != nil {
		project.Autonomous = *req.Autonomous
	}

	project.UpdatedAt = time.Now()

	if err := h.registry.Update(project); err != nil {
		http.Error(w, "failed to update registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.registry.Delete(id); err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
