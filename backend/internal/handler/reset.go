package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/pipeline"
	"github.com/michelroberge/paulette/backend/internal/repository"
)

type ResetHandler struct {
	registry    repository.RegistryRepo
	projectRepo repository.ProjectRepo
}

func NewResetHandler(registry repository.RegistryRepo, projectRepo repository.ProjectRepo) *ResetHandler {
	return &ResetHandler{registry: registry, projectRepo: projectRepo}
}

// Reset clears chat history and artifacts for the target stage and all subsequent stages,
// then rolls the pipeline back to that stage.
func (h *ResetHandler) Reset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Find the target stage in the ordered list
	targetIdx := -1
	for i, s := range pipeline.StageOrder {
		if s == stage {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 || stage == model.StageComplete {
		http.Error(w, "invalid stage for reset", http.StatusBadRequest)
		return
	}

	// Clear chat history and artifacts for target stage and all that follow
	for _, s := range pipeline.StageOrder[targetIdx:] {
		if s == model.StageComplete {
			break
		}
		// Remove chat history
		chatFile := filepath.Join(project.HostDir, ".paulette", string(s), "chat-history.json")
		os.Remove(chatFile)

		// Remove artifact
		artifactFile := filepath.Join(project.HostDir, ".paulette", string(s), string(s)+".md")
		os.Remove(artifactFile)

		// Remove UX mock and framework config if applicable
		if s == model.StageUX {
			os.Remove(filepath.Join(project.HostDir, ".paulette", "ux", "mock.html"))
			os.Remove(filepath.Join(project.HostDir, ".paulette", "ux", "framework.json"))
		}

		// Remove bead graph if applicable
		if s == model.StageBuild {
			beadsFile := filepath.Join(project.HostDir, ".paulette", "build", "beads-graph.json")
			os.Remove(beadsFile)
		}
	}

	// Roll back pipeline
	project.CurrentStage = stage
	project.UpdatedAt = time.Now()

	if err := h.registry.Update(project); err != nil {
		http.Error(w, "failed to update registry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.projectRepo.Save(project.HostDir, project); err != nil {
		http.Error(w, "failed to save project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	state := pipeline.BuildPipelineState(stage, nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}
