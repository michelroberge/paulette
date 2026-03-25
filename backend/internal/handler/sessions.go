package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
)

type SessionHandler struct {
	registry repository.RegistryRepo
}

func NewSessionHandler(registry repository.RegistryRepo) *SessionHandler {
	return &SessionHandler{registry: registry}
}

// ListSessions returns session records for a project, optionally filtered by stage.
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.registry.Get(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	stageFilter := r.URL.Query().Get("stage")

	var sessions []model.Session
	if stageFilter != "" {
		sessions, err = fsrepo.ReadSessionsByStage(project.HostDir, model.StageName(stageFilter))
	} else {
		sessions, err = fsrepo.ReadSessions(project.HostDir)
	}
	if err != nil {
		http.Error(w, "failed to read sessions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	summary := fsrepo.BuildSessionSummary(sessions)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}
