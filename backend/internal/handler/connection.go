package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/repository"
)

// modelsResult is an internal type used to pass ListModels results between
// goroutines inside runTest.
type modelsResult struct {
	models []provider.ModelInfo
	err    error
}

// ConnectionHandler handles HTTP requests for LLM provider connection CRUD,
// connectivity testing, and model discovery.
//
// All responses redact stored credentials: the APIKey is never returned;
// HasCredentials indicates whether a key is stored so the frontend can render
// the appropriate indicator.
type ConnectionHandler struct {
	connStore        *provider.ConnectionStore
	providerRegistry *provider.Registry
	stageConfig      *provider.StageConfigStore
	registry         repository.RegistryRepo
	registryPath     string // for cleaning up connection prompt dirs on delete
}

// NewConnectionHandler creates a ConnectionHandler with all required dependencies.
// registry is used to collect project host directories for stage-config cascade
// cleanup when a connection is deleted.
func NewConnectionHandler(
	connStore *provider.ConnectionStore,
	providerRegistry *provider.Registry,
	stageConfig *provider.StageConfigStore,
	registry repository.RegistryRepo,
	registryPath string,
) *ConnectionHandler {
	return &ConnectionHandler{
		connStore:        connStore,
		providerRegistry: providerRegistry,
		stageConfig:      stageConfig,
		registry:         registry,
		registryPath:     registryPath,
	}
}

// RegisterRoutes mounts all connection endpoints on the provided router.
// Call this with a chi sub-router already scoped to /api/connections.
//
// Route table:
//
//	GET    /               — List all connections (credentials redacted)
//	POST   /               — Create a new connection
//	POST   /test           — Test an unsaved connection (body = full config)
//	POST   /models         — List models for an unsaved connection
//	GET    /{id}           — Get a single connection (credentials redacted)
//	PUT    /{id}           — Update a connection
//	DELETE /{id}           — Delete a connection; returns affected stage names
//	POST   /{id}/test      — Test a saved connection
//	GET    /{id}/models    — List models for a saved connection
func (h *ConnectionHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	// Specific sub-paths must be registered before the {id} wildcard.
	r.Post("/test", h.TestNew)
	r.Post("/models", h.ListModelsNew)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/test", h.Test)
	r.Get("/{id}/models", h.ListModels)
}

// ---------------------------------------------------------------------------
// Request / response shapes
// ---------------------------------------------------------------------------

// connectionInput is the accepted body for Create and Update requests.
// APIKey is write-only and is never returned in any response.
type connectionInput struct {
	Name         string               `json:"name"`
	ProviderType provider.ProviderType `json:"providerType"`
	BaseURL      string               `json:"baseUrl"`
	// APIKey is optional on update — an empty value means "keep existing key".
	APIKey    string `json:"apiKey"`
	OrgID     string `json:"orgId"`
	ProjectID string `json:"projectId"`
	// DefaultModel is the model used when no per-stage override is configured.
	DefaultModel string `json:"defaultModel"`
}

// testResult is the response body for Test and TestNew requests.
type testResult struct {
	Success bool                 `json:"success"`
	Error   string               `json:"error,omitempty"`
	Models  []provider.ModelInfo `json:"models,omitempty"`
}

// deleteResult is the response body for the Delete request.
type deleteResult struct {
	AffectedStages []string `json:"affectedStages"`
}

// ---------------------------------------------------------------------------
// CRUD handlers
// ---------------------------------------------------------------------------

// List handles GET /api/connections.
// Returns all stored connections with credentials redacted (HasCredentials bool).
func (h *ConnectionHandler) List(w http.ResponseWriter, r *http.Request) {
	conns := h.connStore.List()
	responses := make([]provider.ConnectionResponse, len(conns))
	for i, c := range conns {
		responses[i] = c.ToResponse()
	}
	writeJSON(w, responses)
}

// Create handles POST /api/connections.
// Assigns a UUID and UTC timestamps, persists to connections.json (chmod 600),
// and returns 201 with the created connection (credentials redacted).
func (h *ConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input connectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	conn := provider.Connection{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		BaseURL:      input.BaseURL,
		APIKey:       input.APIKey,
		OrgID:        input.OrgID,
		ProjectID:    input.ProjectID,
		DefaultModel: input.DefaultModel,
	}

	created, err := h.connStore.Create(conn)
	if err != nil {
		if isConnValidationError(err) {
			jsonError(w, http.StatusBadRequest, err.Error())
		} else {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("create connection: %s", err.Error()))
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created.ToResponse())
}

// Get handles GET /api/connections/:id.
// Returns the connection with credentials redacted, or 404 if not found.
func (h *ConnectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conn, err := h.connStore.Get(id)
	if err != nil {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("connection %q not found", id))
		return
	}
	writeJSON(w, conn.ToResponse())
}

// Update handles PUT /api/connections/:id.
// Replaces stored fields; CreatedAt is preserved by the store; UpdatedAt is
// refreshed. If APIKey is absent or empty in the request body the existing
// stored key is preserved so callers can update non-secret fields without
// having to re-supply the credential.
func (h *ConnectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Fetch existing record first so we can preserve the API key when the
	// client omits it (common when editing display-name or model fields).
	existing, err := h.connStore.Get(id)
	if err != nil {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("connection %q not found", id))
		return
	}

	var input connectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Keep the existing key unless the caller explicitly provided a new one.
	apiKey := input.APIKey
	if strings.TrimSpace(apiKey) == "" {
		apiKey = existing.APIKey
	}

	conn := provider.Connection{
		ID:           id,
		Name:         input.Name,
		ProviderType: input.ProviderType,
		BaseURL:      input.BaseURL,
		APIKey:       apiKey,
		OrgID:        input.OrgID,
		ProjectID:    input.ProjectID,
		DefaultModel: input.DefaultModel,
	}

	updated, err := h.connStore.Update(conn)
	if err != nil {
		if isConnValidationError(err) {
			jsonError(w, http.StatusBadRequest, err.Error())
		} else if isConnNotFoundError(err) {
			jsonError(w, http.StatusNotFound, fmt.Sprintf("connection %q not found", id))
		} else {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("update connection: %s", err.Error()))
		}
		return
	}

	writeJSON(w, updated.ToResponse())
}

// Delete handles DELETE /api/connections/:id.
// Removes the connection, clears any global or per-project stage assignments
// that referenced it, and returns the list of affected stage names.
func (h *ConnectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Collect project host directories so per-project stage_config.json files
	// can be updated. Errors are non-fatal: cleanup will be partial but the
	// global config is always cleaned up.
	hostDirs := h.collectProjectHostDirs()

	affected, err := h.connStore.Delete(id, h.stageConfig, hostDirs)
	if err != nil {
		if isConnNotFoundError(err) {
			jsonError(w, http.StatusNotFound, fmt.Sprintf("connection %q not found", id))
		} else {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("delete connection: %s", err.Error()))
		}
		return
	}

	// Clean up connection prompt templates directory if it exists.
	promptDir := filepath.Join(h.registryPath, "connection-prompts", id)
	if err := os.RemoveAll(promptDir); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: failed to remove connection prompts dir %s: %v", promptDir, err)
	}

	// Normalise nil to an empty slice so the JSON response is always an array.
	if affected == nil {
		affected = []string{}
	}

	writeJSON(w, deleteResult{AffectedStages: affected})
}

// ---------------------------------------------------------------------------
// Test and model-discovery handlers
// ---------------------------------------------------------------------------

// Test handles POST /api/connections/:id/test.
// Probes a saved connection and opportunistically fetches the model list.
// The result is always a JSON testResult body — provider errors are surfaced
// inline rather than as HTTP 5xx so the frontend can display them alongside
// the form.
func (h *ConnectionHandler) Test(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	prov, err := h.providerRegistry.ForConnection(id)
	if err != nil {
		// Connection not found or unsupported provider type.
		jsonError(w, http.StatusNotFound, fmt.Sprintf("connection %q: %s", id, err.Error()))
		return
	}
	h.runTest(w, r, prov)
}

// TestNew handles POST /api/connections/test.
// Probes an unsaved connection described in the request body. Useful for
// testing credentials before the user saves a new connection form.
func (h *ConnectionHandler) TestNew(w http.ResponseWriter, r *http.Request) {
	var input connectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	conn := &provider.Connection{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		BaseURL:      input.BaseURL,
		APIKey:       input.APIKey,
		OrgID:        input.OrgID,
		ProjectID:    input.ProjectID,
		DefaultModel: input.DefaultModel,
	}

	prov, err := h.providerRegistry.ForUnsavedConnection(conn)
	if err != nil {
		// Return as a result body so the frontend renders it inline.
		writeJSON(w, testResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported provider: %s", err.Error()),
		})
		return
	}
	h.runTest(w, r, prov)
}

// ListModels handles GET /api/connections/:id/models.
// Returns available models for a saved connection, or 422 if the provider
// does not expose a model-list endpoint.
func (h *ConnectionHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	prov, err := h.providerRegistry.ForConnection(id)
	if err != nil {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("connection %q: %s", id, err.Error()))
		return
	}
	h.fetchModels(w, r, prov)
}

// ListModelsNew handles POST /api/connections/models.
// Returns available models for an unsaved connection described in the request body.
func (h *ConnectionHandler) ListModelsNew(w http.ResponseWriter, r *http.Request) {
	var input connectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	conn := &provider.Connection{
		Name:         input.Name,
		ProviderType: input.ProviderType,
		BaseURL:      input.BaseURL,
		APIKey:       input.APIKey,
		OrgID:        input.OrgID,
		ProjectID:    input.ProjectID,
		DefaultModel: input.DefaultModel,
	}

	prov, err := h.providerRegistry.ForUnsavedConnection(conn)
	if err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("unsupported provider: %s", err.Error()))
		return
	}
	h.fetchModels(w, r, prov)
}

// ---------------------------------------------------------------------------
// Shared helper methods
// ---------------------------------------------------------------------------

// runTest probes a provider's connectivity (TestConnection) and opportunistically
// fetches the model list (ListModels) in parallel within a shared 30-second
// deadline derived from the HTTP request context.  Running both calls
// concurrently avoids doubling the round-trip latency for providers that expose
// separate health-check and model-list endpoints (Anthropic, OpenAI, Gemini).
//
// The response is always a testResult; provider-level errors are embedded in
// the JSON body rather than surfaced as HTTP error codes so the frontend can
// render them inline next to the connection form.
//
// Buffered channels (size 1) are used so the goroutines can always send their
// result without blocking, even if the function has already returned due to
// context cancellation — preventing goroutine leaks.
func (h *ConnectionHandler) runTest(w http.ResponseWriter, r *http.Request, prov provider.Provider) {
	// Derive the timeout from the request context so that a client disconnect
	// cancels the in-flight provider calls immediately.
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Buffered channels let both goroutines send exactly once without blocking.
	testErrCh := make(chan error, 1)
	modelsCh := make(chan modelsResult, 1)

	go func() {
		testErrCh <- prov.TestConnection(ctx)
	}()

	go func() {
		models, err := prov.ListModels(ctx)
		modelsCh <- modelsResult{models: models, err: err}
	}()

	// Collect both results (order doesn't matter; channels are buffered).
	testErr := <-testErrCh
	mr := <-modelsCh

	if testErr != nil {
		writeJSON(w, testResult{
			Success: false,
			Error:   testErr.Error(),
		})
		return
	}

	// Connectivity succeeded. Include models when the provider supports listing;
	// a ListModels failure is non-fatal and does not invalidate the test result.
	if mr.err != nil {
		writeJSON(w, testResult{Success: true})
		return
	}

	writeJSON(w, testResult{
		Success: true,
		Models:  mr.models,
	})
}

// fetchModels fetches the model list for a provider and writes it as a JSON array.
// Returns 422 Unprocessable Entity when the provider does not support model listing.
// The 30-second timeout is derived from r.Context() so that client disconnects
// cancel the in-flight provider call promptly.
func (h *ConnectionHandler) fetchModels(w http.ResponseWriter, r *http.Request, prov provider.Provider) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	models, err := prov.ListModels(ctx)
	if err != nil {
		jsonError(w, http.StatusUnprocessableEntity, fmt.Sprintf("model listing not available: %s", err.Error()))
		return
	}

	writeJSON(w, models)
}

// collectProjectHostDirs returns the HostDir path of every registered project.
// Used during connection deletion to locate per-project stage_config.json files
// for cascade cleanup. On registry errors an empty slice is returned — cleanup
// will still proceed for the global config file.
func (h *ConnectionHandler) collectProjectHostDirs() []string {
	if h.registry == nil {
		return nil
	}
	projects, err := h.registry.List()
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(projects))
	for _, p := range projects {
		if p.HostDir != "" {
			dirs = append(dirs, p.HostDir)
		}
	}
	return dirs
}

// ---------------------------------------------------------------------------
// Error classification helpers
// ---------------------------------------------------------------------------

// isConnValidationError reports whether err originates from a user-input
// validation check inside the ConnectionStore (empty name, unknown provider
// type, missing required fields). These map to HTTP 400.
//
// The store returns plain errors without sentinel types, so we use message
// substring matching — kept in sync with validateConnectionInput in
// provider/connection_store.go.
func isConnValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "must not be empty") ||
		strings.Contains(msg, "unknown provider type") ||
		strings.Contains(msg, "requires a base URL")
}

// isConnNotFoundError reports whether err indicates a missing connection record.
// Maps to HTTP 404.
func isConnNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}
