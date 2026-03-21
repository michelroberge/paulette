package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/ai-app-factory/backend/internal/model"
	"github.com/michelroberge/ai-app-factory/backend/internal/repository"
)

type ArtifactHandler struct {
	registry     repository.RegistryRepo
	artifactRepo repository.ArtifactRepo
}

func NewArtifactHandler(registry repository.RegistryRepo, artifactRepo repository.ArtifactRepo) *ArtifactHandler {
	return &ArtifactHandler{
		registry:     registry,
		artifactRepo: artifactRepo,
	}
}

type artifactResponse struct {
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

func (h *ArtifactHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	content, err := h.artifactRepo.Read(project.HostDir, stage)
	if err != nil {
		http.Error(w, "failed to read artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := artifactResponse{
		Content: content,
		Exists:  content != "",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
