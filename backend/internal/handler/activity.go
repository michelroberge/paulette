package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/claudine/backend/internal/stream"
)

type ActivityHandler struct {
	runs *stream.Manager
}

func NewActivityHandler(runs *stream.Manager) *ActivityHandler {
	return &ActivityHandler{runs: runs}
}

// List returns all active runs for a project.
func (h *ActivityHandler) List(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	active := h.runs.ActiveForProject(id)
	if active == nil {
		active = []stream.RunInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(active)
}

// Stream reconnects to an active or recently-finished run's SSE stream.
func (h *ActivityHandler) Stream(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	from := 0
	if v := r.URL.Query().Get("from"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			from = n
		}
	}

	run := h.runs.Get(runID)
	if run == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	run.StreamTo(w, r, from)
}
