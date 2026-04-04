package handler

import (
	"encoding/json"
	"net/http"
	"strings"

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

// GetTraceFile handles GET /api/projects/{id}/run-log/{runId}/trace/{filename}
func (h *RunLogHandler) GetTraceFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	runID := chi.URLParam(r, "runId")
	filename := chi.URLParam(r, "filename")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	safeProjectName := fsrepo.SanitizeProjectName(project.Name)
	data, err := fsrepo.ReadRunTraceFile(h.logBase, safeProjectName, runID, filename)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	if strings.HasSuffix(filename, ".json") {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Write(data)
}

// DeleteRun handles DELETE /api/projects/{id}/run-log/{runId}
func (h *RunLogHandler) DeleteRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	runID := chi.URLParam(r, "runId")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	safeProjectName := fsrepo.SanitizeProjectName(project.Name)
	if err := fsrepo.DeleteRun(h.logBase, safeProjectName, runID); err != nil {
		http.Error(w, "failed to delete run: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PruneRuns handles POST /api/projects/{id}/run-log/prune
// Keeps only the last 10 runs for the project.
func (h *RunLogHandler) PruneRuns(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	safeProjectName := fsrepo.SanitizeProjectName(project.Name)
	deleted, err := fsrepo.PruneProjectRuns(h.logBase, safeProjectName, 10)
	if err != nil {
		http.Error(w, "failed to prune: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"deleted": deleted})
}
