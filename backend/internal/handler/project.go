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

	"github.com/michelroberge/ai-app-factory/backend/internal/model"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
)

type ProjectHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
}

func NewProjectHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo) *ProjectHandler {
	return &ProjectHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
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

	// Git init the host directory if not already a git repo
	if _, err := os.Stat(filepath.Join(req.HostDir, ".git")); os.IsNotExist(err) {
		gitInit := exec.Command("git", "init")
		gitInit.Dir = req.HostDir
		if out, err := gitInit.CombinedOutput(); err != nil {
			log.Printf("git init failed in %s: %v: %s", req.HostDir, err, out)
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

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.registry.Delete(id); err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
