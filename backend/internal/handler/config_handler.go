package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
)

// StageConfigHandler handles HTTP requests for global and per-project stage
// configuration (which connection + model to use for each pipeline stage).
//
// Routes handled:
//
//	GET  /api/config/stages                           → GetGlobalDefaults
//	PUT  /api/config/stages/:stage                    → SetGlobalStageDefault
//	GET  /api/projects/:id/config/stages              → GetProjectOverrides
//	PUT  /api/projects/:id/config/stages/:stage       → SetProjectStageOverride
//	POST /api/projects/:id/config/stages/reset        → ResetProjectOverrides
//
// Note: the struct is named StageConfigHandler (not ConfigHandler) to avoid a
// naming clash with the existing ConfigHandler in config.go, which handles the
// /api/config info endpoint.
type StageConfigHandler struct {
	stageConfig *provider.StageConfigStore
	// connStore is used to validate that connectionId values in stage
	// assignments refer to real, stored connections.
	connStore *provider.ConnectionStore
	registry  repository.RegistryRepo
}

// NewStageConfigHandler constructs a StageConfigHandler with its dependencies.
func NewStageConfigHandler(
	stageConfig *provider.StageConfigStore,
	connStore *provider.ConnectionStore,
	registry repository.RegistryRepo,
) *StageConfigHandler {
	return &StageConfigHandler{
		stageConfig: stageConfig,
		connStore:   connStore,
		registry:    registry,
	}
}

// RegisterRoutes mounts all stage-config routes onto the provided router.
// The project-scoped routes expect {id} to already be in scope (i.e. mounted
// inside a "/api/projects/{id}" sub-router).
func (h *StageConfigHandler) RegisterRoutes(global chi.Router, project chi.Router) {
	// Global defaults
	global.Get("/api/config/stages", h.GetGlobalDefaults)
	global.Put("/api/config/stages/{stage}", h.SetGlobalStageDefault)

	// Per-project overrides — registered inside the /{id} sub-router
	project.Get("/config/stages", h.GetProjectOverrides)
	project.Put("/config/stages/{stage}", h.SetProjectStageOverride)
	project.Post("/config/stages/reset", h.ResetProjectOverrides)
}

// ---------------------------------------------------------------------------
// Global defaults
// ---------------------------------------------------------------------------

// GetGlobalDefaults returns the global stage defaults stored in
// ~/.paulette/config.json.
//
// GET /api/config/stages
//
// Response 200:
//
//	{
//	  "stageDefaults": {
//	    "vision":       { "connectionId": "uuid", "model": "llama3:8b" },
//	    "ux":           { "connectionId": "uuid", "model": "llama3:8b" },
//	    "architecture": { "connectionId": "uuid", "model": "gpt-4o" },
//	    "build":        { "connectionId": "uuid", "model": "gpt-4o" },
//	    "complete":     { "connectionId": "uuid", "model": "llama3:8b" }
//	  }
//	}
func (h *StageConfigHandler) GetGlobalDefaults(w http.ResponseWriter, r *http.Request) {
	cfg := h.stageConfig.GetGlobalDefaults()
	writeJSON(w, cfg)
}

// SetGlobalStageDefault writes a connection + model assignment for a single
// pipeline stage into the global defaults (~/.paulette/config.json).
//
// PUT /api/config/stages/:stage
//
// Request body:
//
//	{ "connectionId": "uuid", "model": "llama3:8b" }
//
// If connectionId is non-empty it must refer to an existing connection.
// An empty connectionId is allowed and reverts the stage to the built-in
// Claude CLI fallback.
//
// Response 200 — full updated GlobalConfig (same shape as GetGlobalDefaults).
func (h *StageConfigHandler) SetGlobalStageDefault(w http.ResponseWriter, r *http.Request) {
	stage := model.StageName(chi.URLParam(r, "stage"))

	var assignment provider.StageAssignment
	if err := json.NewDecoder(r.Body).Decode(&assignment); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate that the referenced connection actually exists (if specified).
	if assignment.ConnectionID != "" {
		if _, err := h.connStore.Get(assignment.ConnectionID); err != nil {
			jsonError(w, http.StatusBadRequest, "unknown connectionId: "+assignment.ConnectionID)
			return
		}
	}

	if err := h.stageConfig.SetGlobalStageDefault(stage, assignment); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg := h.stageConfig.GetGlobalDefaults()
	writeJSON(w, cfg)
}

// ---------------------------------------------------------------------------
// Per-project overrides
// ---------------------------------------------------------------------------

// GetProjectOverrides returns the per-project stage overrides for the project
// identified by {id}. Stages not present in the response (or set to null)
// inherit the global default.
//
// GET /api/projects/:id/config/stages
//
// Response 200:
//
//	{
//	  "overrides": {
//	    "architecture": { "connectionId": "uuid", "model": "gpt-4o-mini" },
//	    "build":        null
//	  }
//	}
func (h *StageConfigHandler) GetProjectOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.registry.Get(projectID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	overrides := h.stageConfig.GetProjectOverrides(project.HostDir)
	writeJSON(w, overrides)
}

// SetProjectStageOverride sets or clears a per-stage override for the project
// identified by {id}.
//
// Sending a JSON null body, an empty HTTP body, or omitting the Content-Type
// header all clear the override for the stage, reverting it to inherit the
// global default.
//
// PUT /api/projects/:id/config/stages/:stage
//
// Request body (set override):
//
//	{ "connectionId": "uuid", "model": "gpt-4o" }
//
// Request body (clear / inherit — any of the following work):
//
//	null
//	(empty body)
//
// If connectionId is non-empty it must refer to an existing connection.
//
// Response 200 — full updated ProjectStageConfig (same shape as GetProjectOverrides).
func (h *StageConfigHandler) SetProjectStageOverride(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	stage := model.StageName(chi.URLParam(r, "stage"))

	project, err := h.registry.Get(projectID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	// Decode into a raw JSON value first so we can distinguish between:
	//   - a JSON object  → set override
	//   - a JSON null    → clear override (inherit from global)
	//   - an empty body  → also clear override (Content-Length: 0 is a natural
	//                      way for API clients to express "clear", and the AC
	//                      states that "PUT with null body clears override")
	var raw json.RawMessage
	decErr := json.NewDecoder(r.Body).Decode(&raw)
	if decErr != nil && !errors.Is(decErr, io.EOF) {
		// Real parse error — the body was present but malformed JSON.
		jsonError(w, http.StatusBadRequest, "invalid request body: "+decErr.Error())
		return
	}

	// A JSON null literal or an empty body both mean "inherit from global".
	var assignment *provider.StageAssignment
	if decErr == nil && string(raw) != "null" {
		var a provider.StageAssignment
		if unmarshalErr := json.Unmarshal(raw, &a); unmarshalErr != nil {
			jsonError(w, http.StatusBadRequest, "invalid stage assignment: "+unmarshalErr.Error())
			return
		}
		// Validate that the referenced connection exists (if specified).
		if a.ConnectionID != "" {
			if _, connErr := h.connStore.Get(a.ConnectionID); connErr != nil {
				jsonError(w, http.StatusBadRequest, "unknown connectionId: "+a.ConnectionID)
				return
			}
		}
		assignment = &a
	}
	// assignment == nil when body was empty or raw was "null" — treated as inherit.

	if setErr := h.stageConfig.SetProjectStageOverride(project.HostDir, stage, assignment); setErr != nil {
		// Returns an error only for invalid stage names.
		jsonError(w, http.StatusBadRequest, setErr.Error())
		return
	}

	overrides := h.stageConfig.GetProjectOverrides(project.HostDir)
	writeJSON(w, overrides)
}

// ResetProjectOverrides removes all per-project stage overrides, reverting
// every stage for this project back to the global defaults.
//
// POST /api/projects/:id/config/stages/reset
//
// Response 200:
//
//	{ "status": "ok" }
func (h *StageConfigHandler) ResetProjectOverrides(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.registry.Get(projectID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	if resetErr := h.stageConfig.ResetProjectOverrides(project.HostDir); resetErr != nil {
		jsonError(w, http.StatusInternalServerError, "failed to reset project overrides: "+resetErr.Error())
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}
