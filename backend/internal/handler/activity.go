package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/repository"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

type ActivityHandler struct {
	runs         *stream.Manager
	activityRepo repository.ActivityRepo
	registry     repository.RegistryRepo
}

func NewActivityHandler(runs *stream.Manager, activityRepo repository.ActivityRepo, registry repository.RegistryRepo) *ActivityHandler {
	return &ActivityHandler{runs: runs, activityRepo: activityRepo, registry: registry}
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

// Summary returns active run counts keyed by projectID — one call for all projects.
// Used by the home page to show per-project activity badges without N+1 requests.
func (h *ActivityHandler) Summary(w http.ResponseWriter, r *http.Request) {
	projects, err := h.registry.List()
	if err != nil {
		http.Error(w, "failed to list projects: "+err.Error(), http.StatusInternalServerError)
		return
	}
	result := make(map[string]int, len(projects))
	for _, p := range projects {
		result[p.ID] = len(h.runs.ActiveForProject(p.ID))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// SendBtw queues a follow-up message for the stage of an active run.
// The message is stored in activity.json and processed after the current run finishes.
func (h *ActivityHandler) SendBtw(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	runID := chi.URLParam(r, "runId")

	run := h.runs.Get(runID)
	if run == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	project, err := h.registry.Get(projectID)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	stage := model.StageName(run.Stage)
	if err := h.activityRepo.AppendBtw(project.HostDir, stage, body.Message, time.Now()); err != nil {
		http.Error(w, "failed to queue message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
