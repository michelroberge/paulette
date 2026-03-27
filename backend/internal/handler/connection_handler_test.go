// Package handler — ConnectionHandler integration tests (Milestone 6.5).
//
// These tests verify the full HTTP surface of ConnectionHandler:
//   - CRUD operations (List, Create, Get, Update, Delete)
//   - Credential redaction: APIKey is never returned; HasCredentials is set
//   - Validation errors map to HTTP 400
//   - Missing connections map to HTTP 404
//   - Connection deletion cascades stage-config cleanup
//   - Test and TestNew route to the correct provider
//   - ListModels and ListModelsNew fetch from the provider
//
// External provider endpoints are mocked with net/http/httptest so no real
// network connectivity is required.
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// ---------------------------------------------------------------------------
// Test setup helpers
// ---------------------------------------------------------------------------

// newConnectionHandlerSetup creates a fully wired ConnectionHandler backed by
// real in-memory-backed stores using temp directories.
// Returns the handler, a chi router, and a cleanup function.
func newConnectionHandlerSetup(t *testing.T) (*ConnectionHandler, *chi.Mux) {
	t.Helper()

	globalDir := t.TempDir()

	connStore := provider.NewConnectionStore(globalDir)
	stageStore := provider.NewStageConfigStore(globalDir)
	registry := provider.NewRegistry(connStore)

	projRegistry := &mockRegistryRepo{
		projects: map[string]*model.Project{
			"proj-1": {
				ID:      "proj-1",
				Name:    "Test Project",
				HostDir: t.TempDir(),
			},
		},
	}

	h := NewConnectionHandler(connStore, registry, stageStore, projRegistry)

	r := chi.NewRouter()
	r.Route("/api/connections", h.RegisterRoutes)

	return h, r
}

// seedConnection inserts a connection record into the store and returns the
// assigned ID. Fails the test on any error.
func seedConnection(t *testing.T, h *ConnectionHandler, conn provider.Connection) string {
	t.Helper()
	created, err := h.connStore.Create(conn)
	if err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	return created.ID
}

// ollamaInput returns a minimal connectionInput body for an Ollama connection.
func ollamaInput(name string) map[string]string {
	return map[string]string{
		"name":         name,
		"providerType": "ollama",
		"baseUrl":      "http://localhost:11434",
		"defaultModel": "llama3:8b",
	}
}

// openAIInput returns a connectionInput body for an OpenAI connection.
func openAIInput(name, apiKey string) map[string]any {
	return map[string]any{
		"name":         name,
		"providerType": "openai",
		"apiKey":       apiKey,
		"defaultModel": "gpt-4o",
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestConnectionHandler_List_EmptyStore(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	rr := doRequest(t, router, http.MethodGet, "/api/connections/", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var result []any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("want empty list, got %d items", len(result))
	}
}

func TestConnectionHandler_List_ReturnsConnections(t *testing.T) {
	h, router := newConnectionHandlerSetup(t)

	seedConnection(t, h, provider.Connection{
		Name:         "ollama-1",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	seedConnection(t, h, provider.Connection{
		Name:         "openai-1",
		ProviderType: provider.ProviderOpenAI,
		APIKey:       "sk-secret",
		DefaultModel: "gpt-4o",
	})

	rr := doRequest(t, router, http.MethodGet, "/api/connections/", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var result []provider.ConnectionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("want 2 connections, got %d", len(result))
	}
}

// TestConnectionHandler_List_CredentialsRedacted verifies that the APIKey is
// NEVER returned in the List response — only the HasCredentials boolean.
func TestConnectionHandler_List_CredentialsRedacted(t *testing.T) {
	h, router := newConnectionHandlerSetup(t)

	seedConnection(t, h, provider.Connection{
		Name:         "openai-secret",
		ProviderType: provider.ProviderOpenAI,
		APIKey:       "sk-top-secret-key",
		DefaultModel: "gpt-4o",
	})

	rr := doRequest(t, router, http.MethodGet, "/api/connections/", nil)

	// The raw body must not contain the key value.
	if strings.Contains(rr.Body.String(), "sk-top-secret-key") {
		t.Error("API key leaked in List response body")
	}

	var result []provider.ConnectionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("want 1 connection, got %d", len(result))
	}
	if !result[0].HasCredentials {
		t.Error("want HasCredentials=true for connection with API key, got false")
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestConnectionHandler_Create_Ollama_Returns201(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	rr := doRequest(t, router, http.MethodPost, "/api/connections/", ollamaInput("my-ollama"))

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d — body: %s", rr.Code, rr.Body)
	}

	var resp provider.ConnectionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID == "" {
		t.Error("want non-empty ID in created connection")
	}
	if resp.Name != "my-ollama" {
		t.Errorf("want name 'my-ollama', got %q", resp.Name)
	}
	if string(resp.ProviderType) != "ollama" {
		t.Errorf("want providerType 'ollama', got %q", resp.ProviderType)
	}
	// No credentials for Ollama.
	if resp.HasCredentials {
		t.Error("Ollama connection should not have HasCredentials=true")
	}
}

func TestConnectionHandler_Create_OpenAI_SetsHasCredentials(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	rr := doRequest(t, router, http.MethodPost, "/api/connections/", openAIInput("my-openai", "sk-abc"))

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d — body: %s", rr.Code, rr.Body)
	}

	var resp provider.ConnectionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.HasCredentials {
		t.Error("want HasCredentials=true for OpenAI connection with API key")
	}
	// Verify no key leakage in response body.
	if strings.Contains(rr.Body.String(), "sk-abc") {
		t.Error("API key leaked in Create response body")
	}
}

func TestConnectionHandler_Create_EmptyName_Returns400(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	body := map[string]string{
		"name":         "",
		"providerType": "ollama",
		"baseUrl":      "http://localhost:11434",
		"defaultModel": "llama3:8b",
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/", body)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty name, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestConnectionHandler_Create_UnknownProviderType_Returns400(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	body := map[string]string{
		"name":         "test",
		"providerType": "totally-made-up",
		"defaultModel": "some-model",
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/", body)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unknown provider type, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestConnectionHandler_Create_OllamaMissingBaseURL_Returns400(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	body := map[string]string{
		"name":         "no-url",
		"providerType": "ollama",
		"defaultModel": "llama3:8b",
		// baseUrl intentionally omitted
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/", body)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for Ollama without baseUrl, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestConnectionHandler_Create_BadJSON_Returns400(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	req := httptest.NewRequest(http.MethodPost, "/api/connections/",
		strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad JSON, got %d — body: %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestConnectionHandler_Get_Returns200(t *testing.T) {
	h, router := newConnectionHandlerSetup(t)
	id := seedConnection(t, h, provider.Connection{
		Name:         "get-test",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})

	rr := doRequest(t, router, http.MethodGet, "/api/connections/"+id, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var resp provider.ConnectionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != id {
		t.Errorf("want ID %q, got %q", id, resp.ID)
	}
}

func TestConnectionHandler_Get_NotFound_Returns404(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	rr := doRequest(t, router, http.MethodGet, "/api/connections/does-not-exist", nil)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d — body: %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestConnectionHandler_Update_ChangesModel(t *testing.T) {
	h, router := newConnectionHandlerSetup(t)
	id := seedConnection(t, h, provider.Connection{
		Name:         "update-test",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})

	body := map[string]string{
		"name":         "update-test",
		"providerType": "ollama",
		"baseUrl":      "http://localhost:11434",
		"defaultModel": "llama3:70b", // changed
	}
	rr := doRequest(t, router, http.MethodPut, "/api/connections/"+id, body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var resp provider.ConnectionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DefaultModel != "llama3:70b" {
		t.Errorf("want defaultModel 'llama3:70b', got %q", resp.DefaultModel)
	}
}

func TestConnectionHandler_Update_EmptyAPIKey_KeepsExistingKey(t *testing.T) {
	// When the client PUTs without an API key, the existing stored key must be
	// preserved — this is the standard "edit display fields without re-entering
	// the secret" pattern required by the UX spec.
	h, router := newConnectionHandlerSetup(t)
	id := seedConnection(t, h, provider.Connection{
		Name:         "openai-update",
		ProviderType: provider.ProviderOpenAI,
		APIKey:       "sk-original-key",
		DefaultModel: "gpt-4o",
	})

	// Update only the model — no API key in body.
	body := map[string]string{
		"name":         "openai-update",
		"providerType": "openai",
		"defaultModel": "gpt-4o-mini",
		// apiKey intentionally omitted
	}
	rr := doRequest(t, router, http.MethodPut, "/api/connections/"+id, body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	// Verify via the store that the key was preserved.
	updated, err := h.connStore.Get(id)
	if err != nil {
		t.Fatalf("Get updated connection: %v", err)
	}
	if updated.APIKey != "sk-original-key" {
		t.Errorf("want original API key preserved, got %q", updated.APIKey)
	}
}

func TestConnectionHandler_Update_NotFound_Returns404(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	body := ollamaInput("ghost")
	rr := doRequest(t, router, http.MethodPut, "/api/connections/no-such-id", body)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d — body: %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestConnectionHandler_Delete_Returns200WithEmptyAffectedStages(t *testing.T) {
	h, router := newConnectionHandlerSetup(t)
	id := seedConnection(t, h, provider.Connection{
		Name:         "to-delete",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})

	rr := doRequest(t, router, http.MethodDelete, "/api/connections/"+id, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stages, ok := resp["affectedStages"]
	if !ok {
		t.Fatal("want 'affectedStages' key in delete response")
	}
	// No stage assignments → empty array.
	arr, ok := stages.([]any)
	if !ok {
		t.Fatalf("want affectedStages to be array, got %T", stages)
	}
	if len(arr) != 0 {
		t.Errorf("want empty affectedStages, got %v", arr)
	}

	// Confirm removal from the store.
	if _, err := h.connStore.Get(id); err == nil {
		t.Error("connection should be removed after Delete, but Get succeeded")
	}
}

// TestConnectionHandler_Delete_CascadesStageAssignments is the HTTP-level
// verification of Milestone 6.5: deleting a connection must clear global stage
// assignments and return the affected stage names in the response.
func TestConnectionHandler_Delete_CascadesStageAssignments(t *testing.T) {
	h, router := newConnectionHandlerSetup(t)
	_ = router // router not used here — we construct a fresh one with stageConfig wired

	// Create a connection and assign it to two global stages.
	id := seedConnection(t, h, provider.Connection{
		Name:         "cascade-conn",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})

	// Use the stageConfig that is already part of the handler (same directory).
	if err := h.stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{
		ConnectionID: id, Model: "llama3:8b",
	}); err != nil {
		t.Fatalf("seed vision default: %v", err)
	}
	if err := h.stageConfig.SetGlobalStageDefault(model.StageUX, provider.StageAssignment{
		ConnectionID: id, Model: "llama3:8b",
	}); err != nil {
		t.Fatalf("seed ux default: %v", err)
	}

	// Build a fresh router backed by the same handler so stageConfig is wired.
	r2 := chi.NewRouter()
	r2.Route("/api/connections", h.RegisterRoutes)

	rr := doRequest(t, r2, http.MethodDelete, "/api/connections/"+id, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var resp struct {
		AffectedStages []string `json:"affectedStages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.AffectedStages) != 2 {
		t.Errorf("want 2 affected stages (vision+ux), got %v", resp.AffectedStages)
	}

	// Verify the stage defaults were actually cleared on disk.
	cfg := h.stageConfig.GetGlobalDefaults()
	if _, ok := cfg.StageDefaults[model.StageVision]; ok {
		t.Error("vision stage default should have been cleared after connection delete")
	}
	if _, ok := cfg.StageDefaults[model.StageUX]; ok {
		t.Error("ux stage default should have been cleared after connection delete")
	}
}

func TestConnectionHandler_Delete_NotFound_Returns404(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	rr := doRequest(t, router, http.MethodDelete, "/api/connections/ghost-id", nil)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d — body: %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------------------
// Test and TestNew — against a real mock HTTP server
// ---------------------------------------------------------------------------

// TestConnectionHandler_TestNew_OllamaSuccess creates a mock Ollama server,
// sends a TestNew request with its URL, and verifies the response includes
// success=true and the model list.
func TestConnectionHandler_TestNew_OllamaSuccess(t *testing.T) {
	// Create a mock Ollama server.
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "llama3:8b"},
					{"name": "mistral:7b"},
				},
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockOllama.Close()

	_, router := newConnectionHandlerSetup(t)

	body := map[string]string{
		"name":         "test-ollama",
		"providerType": "ollama",
		"baseUrl":      mockOllama.URL,
		"defaultModel": "llama3:8b",
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/test", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var result struct {
		Success bool                 `json:"success"`
		Error   string               `json:"error,omitempty"`
		Models  []provider.ModelInfo `json:"models,omitempty"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !result.Success {
		t.Errorf("want success=true, got false (error: %s)", result.Error)
	}
	if len(result.Models) != 2 {
		t.Errorf("want 2 models in TestNew response, got %d", len(result.Models))
	}
}

// TestConnectionHandler_TestNew_OllamaFailure tests the path where the Ollama
// server is unreachable — the response must be success=false with an error
// message (not an HTTP 5xx).
func TestConnectionHandler_TestNew_OllamaFailure(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	// Port 1 is always refused on Linux/macOS.
	body := map[string]string{
		"name":         "offline-ollama",
		"providerType": "ollama",
		"baseUrl":      "http://127.0.0.1:1",
		"defaultModel": "llama3:8b",
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/test", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 (error embedded in body), got %d — body: %s", rr.Code, rr.Body)
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Success {
		t.Error("want success=false for unreachable server")
	}
	if result.Error == "" {
		t.Error("want non-empty error message for unreachable server")
	}
}

// TestConnectionHandler_TestNew_UnsupportedProvider verifies that requesting a
// test for the GitHub Copilot provider (P3 / coming soon) returns a JSON body
// with success=false rather than HTTP 4xx/5xx — the frontend renders the result
// inline.
func TestConnectionHandler_TestNew_UnsupportedProvider(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	body := map[string]string{
		"name":         "copilot",
		"providerType": "github_copilot",
		"defaultModel": "copilot-model",
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/test", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 (error embedded in body), got %d — body: %s", rr.Code, rr.Body)
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Success {
		t.Error("want success=false for unsupported provider")
	}
	if result.Error == "" {
		t.Error("want error message explaining unsupported provider")
	}
}

// TestConnectionHandler_Test_SavedConnection calls Test on a saved connection.
func TestConnectionHandler_Test_SavedConnection(t *testing.T) {
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	}))
	defer mockOllama.Close()

	h, router := newConnectionHandlerSetup(t)
	id := seedConnection(t, h, provider.Connection{
		Name:         "saved-ollama",
		ProviderType: provider.ProviderOllama,
		BaseURL:      mockOllama.URL,
		DefaultModel: "llama3:8b",
	})

	rr := doRequest(t, router, http.MethodPost, "/api/connections/"+id+"/test", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !result.Success {
		t.Error("want success=true for reachable mock server")
	}
}

func TestConnectionHandler_Test_NotFound_Returns404(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	rr := doRequest(t, router, http.MethodPost, "/api/connections/no-such-id/test", nil)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d — body: %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------------------
// ListModels and ListModelsNew
// ---------------------------------------------------------------------------

func TestConnectionHandler_ListModels_SavedOllama(t *testing.T) {
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "phi3:3.8b"},
				{"name": "gemma2:9b"},
			},
		})
	}))
	defer mockOllama.Close()

	h, router := newConnectionHandlerSetup(t)
	id := seedConnection(t, h, provider.Connection{
		Name:         "models-test",
		ProviderType: provider.ProviderOllama,
		BaseURL:      mockOllama.URL,
		DefaultModel: "phi3:3.8b",
	})

	rr := doRequest(t, router, http.MethodGet, "/api/connections/"+id+"/models", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var models []provider.ModelInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("want 2 models, got %d", len(models))
	}
}

func TestConnectionHandler_ListModels_ClaudeCLI_Returns422(t *testing.T) {
	h, router := newConnectionHandlerSetup(t)
	id := seedConnection(t, h, provider.Connection{
		Name:         "cli-provider",
		ProviderType: provider.ProviderClaudeCLI,
	})

	rr := doRequest(t, router, http.MethodGet, "/api/connections/"+id+"/models", nil)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for Claude CLI (no model list), got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestConnectionHandler_ListModelsNew_OllamaSuccess(t *testing.T) {
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{{"name": "llama3:8b"}},
		})
	}))
	defer mockOllama.Close()

	_, router := newConnectionHandlerSetup(t)

	body := map[string]string{
		"name":         "unsaved-ollama",
		"providerType": "ollama",
		"baseUrl":      mockOllama.URL,
		"defaultModel": "llama3:8b",
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/models", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var models []provider.ModelInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(models) != 1 || models[0].ID != "llama3:8b" {
		t.Errorf("unexpected models: %v", models)
	}
}

// ---------------------------------------------------------------------------
// Claude CLI connection (no URL, no credentials)
// ---------------------------------------------------------------------------

func TestConnectionHandler_Create_ClaudeCLI_NoRequiredFields(t *testing.T) {
	_, router := newConnectionHandlerSetup(t)

	body := map[string]string{
		"name":         "claude-cli-conn",
		"providerType": "claude_cli",
		// No baseUrl, no apiKey, no defaultModel required for claude_cli.
	}
	rr := doRequest(t, router, http.MethodPost, "/api/connections/", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201 for Claude CLI connection, got %d — body: %s", rr.Code, rr.Body)
	}
}
