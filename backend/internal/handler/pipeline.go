package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/ai-app-factory/backend/internal/pipeline"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
)

type PipelineHandler struct {
	registry     repository.RegistryRepo
	projectRepo  repository.ProjectRepo
	artifactRepo repository.ArtifactRepo
}

func NewPipelineHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo, artifactRepo repository.ArtifactRepo) *PipelineHandler {
	return &PipelineHandler{
		registry:     registry,
		projectRepo:  projectRepo,
		artifactRepo: artifactRepo,
	}
}

func (h *PipelineHandler) GetPipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	state := pipeline.BuildPipelineState(project.CurrentStage)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

type approveResponse struct {
	PreviousStage string `json:"previousStage"`
	CurrentStage  string `json:"currentStage"`
}

func (h *PipelineHandler) Approve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Check artifact exists for current stage
	exists, err := h.artifactRepo.Exists(project.HostDir, project.CurrentStage)
	if err != nil {
		http.Error(w, "failed to check artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "cannot approve: no artifact for current stage", http.StatusBadRequest)
		return
	}

	// Advance to next stage
	previousStage := project.CurrentStage
	nextStage, err := pipeline.NextStage(project.CurrentStage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	project.CurrentStage = nextStage
	project.UpdatedAt = time.Now()

	// Update both registry and project file
	if err := h.registry.Update(project); err != nil {
		http.Error(w, "failed to update registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approveResponse{
		PreviousStage: string(previousStage),
		CurrentStage:  string(nextStage),
	})
}
