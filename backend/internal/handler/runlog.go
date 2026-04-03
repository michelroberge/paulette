package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
)

// RunLogHandler serves run log entries for one or all projects.
type RunLogHandler struct {
	registry repository.RegistryRepo
	logBase  string
}

func NewRunLogHandler(registry repository.RegistryRepo, logBase string) *RunLogHandler {
	return &RunLogHandler{registry: registry, logBase: logBase}
}

// GetForProject handles GET /api/projects/{id}/run-log
// Returns all run log entries for the project, sorted newest-first.
func (h *RunLogHandler) GetForProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	safeProjectName := fsrepo.SanitizeProjectName(project.Name)
	entries, err := fsrepo.ReadProjectRuns(h.logBase, safeProjectName)
	if err != nil {
		http.Error(w, "failed to read run log: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []model.RunLogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// GetAll handles GET /api/run-log
// Returns run log entries across all projects, sorted newest-first.
func (h *RunLogHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	entries, err := fsrepo.ReadAllRuns(h.logBase)
	if err != nil {
		http.Error(w, "failed to read run log: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []model.RunLogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
