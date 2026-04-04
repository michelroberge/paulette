package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockRegistryRepo implements repository.RegistryRepo for testing.
// Only Get and List are used by StageConfigHandler.
type mockRegistryRepo struct {
	projects map[string]*model.Project
}

func (r *mockRegistryRepo) List() ([]model.Project, error) {
	out := make([]model.Project, 0, len(r.projects))
	for _, p := range r.projects {
		out = append(out, *p)
	}
	return out, nil
}

func (r *mockRegistryRepo) Get(id string) (*model.Project, error) {
	p, ok := r.projects[id]
	if !ok {
		return nil, fmt.Errorf("project %q not found", id)
	}
	return p, nil
}

func (r *mockRegistryRepo) Create(p *model.Project) error { return nil }
func (r *mockRegistryRepo) Update(p *model.Project) error { return nil }
func (r *mockRegistryRepo) UpdateFunc(id string, fn func(*model.Project) error) error {
	p, ok := r.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	return fn(p)
}
func (r *mockRegistryRepo) Delete(id string) error { return nil }

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestHandlerSetup creates a StageConfigHandler backed by real stores using
// temp directories, along with a chi router ready for testing.
// Returns the handler and a cleanup function.
func newTestHandlerSetup(t *testing.T) (*StageConfigHandler, *chi.Mux, string) {
	t.Helper()

	// Temp dir acts as ~/.paulette (global registry path).
	globalDir := t.TempDir()
	// Temp dir acts as the project hostDir.
	projectDir := t.TempDir()

	connStore := provider.NewConnectionStore(globalDir)
	stageStore := provider.NewStageConfigStore(globalDir)

	registry := &mockRegistryRepo{
		projects: map[string]*model.Project{
			"proj-1": {
				ID:      "proj-1",
				Name:    "Test Project",
				HostDir: projectDir,
			},
		},
	}

	h := NewStageConfigHandler(stageStore, connStore, registry)

	r := chi.NewRouter()
	r.Get("/api/config/stages", h.GetGlobalDefaults)
	r.Put("/api/config/stages/{stage}", h.SetGlobalStageDefault)
	r.Route("/api/projects/{id}", func(r chi.Router) {
		r.Get("/config/stages", h.GetProjectOverrides)
		r.Put("/config/stages/{stage}", h.SetProjectStageOverride)
		r.Post("/config/stages/reset", h.ResetProjectOverrides)
	})

	return h, r, projectDir
}

// createTestConnection inserts a real connection into the store and returns
// its assigned ID.
func createTestConnection(t *testing.T, cs *provider.ConnectionStore) string {
	t.Helper()
	conn, err := cs.Create(provider.Connection{
		Name:         "test-conn",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create test connection: %v", err)
	}
	return conn.ID
}

// doRequest is a convenience wrapper around httptest.
func doRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// GetGlobalDefaults
// ---------------------------------------------------------------------------

func TestGetGlobalDefaults_Empty(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	rr := doRequest(t, router, http.MethodGet, "/api/config/stages", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var cfg provider.GlobalConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Empty config — no defaults yet.
	if len(cfg.StageDefaults) != 0 {
		t.Errorf("want empty stageDefaults, got %v", cfg.StageDefaults)
	}
}

func TestGetGlobalDefaults_ReturnsStoredDefaults(t *testing.T) {
	h, router, _ := newTestHandlerSetup(t)

	// Seed a default directly via the store.
	connID := createTestConnection(t, h.connStore)
	if err := h.stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{
		ConnectionID: connID,
		Model:        "llama3:8b",
	}); err != nil {
		t.Fatalf("seed stage default: %v", err)
	}

	rr := doRequest(t, router, http.MethodGet, "/api/config/stages", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var cfg provider.GlobalConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got, ok := cfg.StageDefaults[model.StageVision]
	if !ok {
		t.Fatalf("want vision default in stageDefaults, got %v", cfg.StageDefaults)
	}
	if got.ConnectionID != connID || got.Model != "llama3:8b" {
		t.Errorf("want {%s, llama3:8b}, got %+v", connID, got)
	}
}

// ---------------------------------------------------------------------------
// SetGlobalStageDefault
// ---------------------------------------------------------------------------

func TestSetGlobalStageDefault_ValidAssignment(t *testing.T) {
	h, router, _ := newTestHandlerSetup(t)
	connID := createTestConnection(t, h.connStore)

	body := map[string]string{"connectionId": connID, "model": "llama3:8b"}
	rr := doRequest(t, router, http.MethodPut, "/api/config/stages/vision", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var cfg provider.GlobalConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got := cfg.StageDefaults[model.StageVision]
	if got.ConnectionID != connID {
		t.Errorf("want connectionId %s, got %s", connID, got.ConnectionID)
	}
}

func TestSetGlobalStageDefault_EmptyConnectionID_AllowedAsCLIFallback(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	// Empty connectionId is valid — it means "use Claude CLI fallback".
	body := map[string]string{"connectionId": "", "model": "claude-3-sonnet"}
	rr := doRequest(t, router, http.MethodPut, "/api/config/stages/ux", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestSetGlobalStageDefault_UnknownConnectionID_Returns400(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	body := map[string]string{"connectionId": "does-not-exist", "model": "gpt-4o"}
	rr := doRequest(t, router, http.MethodPut, "/api/config/stages/build", body)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d — body: %s", rr.Code, rr.Body)
	}

	var errResp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("want non-empty error message in response")
	}
}

func TestSetGlobalStageDefault_InvalidStageName_Returns400(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	body := map[string]string{"connectionId": "", "model": "llama3"}
	rr := doRequest(t, router, http.MethodPut, "/api/config/stages/notastage", body)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestSetGlobalStageDefault_InvalidBody_Returns400(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	req := httptest.NewRequest(http.MethodPut, "/api/config/stages/vision",
		bytes.NewBufferString("not-json{"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Route directly to avoid chi needing to parse parameters.
	// Instead, create a minimal chi context.
	router.ServeHTTP(rr, req)

	// The path /api/config/stages/vision with bad JSON body should be 400.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d — body: %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------------------
// GetProjectOverrides
// ---------------------------------------------------------------------------

func TestGetProjectOverrides_UnknownProject_Returns404(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	rr := doRequest(t, router, http.MethodGet, "/api/projects/no-such-project/config/stages", nil)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestGetProjectOverrides_EmptyOverrides(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	rr := doRequest(t, router, http.MethodGet, "/api/projects/proj-1/config/stages", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var cfg provider.ProjectStageConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(cfg.Overrides) != 0 {
		t.Errorf("want empty overrides, got %v", cfg.Overrides)
	}
}

// ---------------------------------------------------------------------------
// SetProjectStageOverride
// ---------------------------------------------------------------------------

func TestSetProjectStageOverride_SetOverride(t *testing.T) {
	h, router, _ := newTestHandlerSetup(t)
	connID := createTestConnection(t, h.connStore)

	body := map[string]string{"connectionId": connID, "model": "gpt-4o"}
	rr := doRequest(t, router, http.MethodPut, "/api/projects/proj-1/config/stages/architecture", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var cfg provider.ProjectStageConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	got := cfg.Overrides[model.StageArchitecture]
	if got == nil {
		t.Fatal("want non-nil architecture override")
	}
	if got.ConnectionID != connID || got.Model != "gpt-4o" {
		t.Errorf("want {%s, gpt-4o}, got %+v", connID, got)
	}
}

func TestSetProjectStageOverride_ClearWithJSONNull(t *testing.T) {
	h, router, projectDir := newTestHandlerSetup(t)
	connID := createTestConnection(t, h.connStore)

	// First set an override.
	if err := h.stageConfig.SetProjectStageOverride(
		projectDir, model.StageBuild,
		&provider.StageAssignment{ConnectionID: connID, Model: "gpt-4o"},
	); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	// Now clear it by sending JSON null.
	req := httptest.NewRequest(http.MethodPut, "/api/projects/proj-1/config/stages/build",
		bytes.NewBufferString("null"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var cfg provider.ProjectStageConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Key present but value nil means "inherit from global".
	if assignment, ok := cfg.Overrides[model.StageBuild]; ok && assignment != nil {
		t.Errorf("want nil (inherit) for build, got %+v", assignment)
	}
}

func TestSetProjectStageOverride_ClearWithEmptyBody(t *testing.T) {
	h, router, projectDir := newTestHandlerSetup(t)
	connID := createTestConnection(t, h.connStore)

	// First set an override.
	if err := h.stageConfig.SetProjectStageOverride(
		projectDir, model.StageVision,
		&provider.StageAssignment{ConnectionID: connID, Model: "llama3"},
	); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	// Now clear it by sending an empty body (Content-Length: 0).
	req := httptest.NewRequest(http.MethodPut, "/api/projects/proj-1/config/stages/vision",
		bytes.NewBufferString(""))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var cfg provider.ProjectStageConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Empty body should clear the override (nil assignment = inherit).
	if assignment, ok := cfg.Overrides[model.StageVision]; ok && assignment != nil {
		t.Errorf("want nil (inherit) for vision after empty-body clear, got %+v", assignment)
	}
}

func TestSetProjectStageOverride_UnknownConnectionID_Returns400(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	body := map[string]string{"connectionId": "ghost-id", "model": "gpt-4o"}
	rr := doRequest(t, router, http.MethodPut, "/api/projects/proj-1/config/stages/build", body)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestSetProjectStageOverride_UnknownProject_Returns404(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	body := map[string]string{"connectionId": "", "model": "llama3"}
	rr := doRequest(t, router, http.MethodPut, "/api/projects/no-proj/config/stages/ux", body)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestSetProjectStageOverride_InvalidStageName_Returns400(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	body := map[string]string{"connectionId": "", "model": "llama3"}
	rr := doRequest(t, router, http.MethodPut, "/api/projects/proj-1/config/stages/badstage", body)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestSetProjectStageOverride_MalformedJSON_Returns400(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	req := httptest.NewRequest(http.MethodPut, "/api/projects/proj-1/config/stages/ux",
		bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d — body: %s", rr.Code, rr.Body)
	}
}

// ---------------------------------------------------------------------------
// ResetProjectOverrides
// ---------------------------------------------------------------------------

func TestResetProjectOverrides_ClearsAll(t *testing.T) {
	h, router, projectDir := newTestHandlerSetup(t)
	connID := createTestConnection(t, h.connStore)

	// Seed two overrides.
	stages := []model.StageName{model.StageVision, model.StageUX}
	for _, s := range stages {
		if err := h.stageConfig.SetProjectStageOverride(projectDir, s,
			&provider.StageAssignment{ConnectionID: connID, Model: "test-model"}); err != nil {
			t.Fatalf("seed override for %s: %v", s, err)
		}
	}

	rr := doRequest(t, router, http.MethodPost, "/api/projects/proj-1/config/stages/reset", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("want status=ok, got %v", resp)
	}

	// Verify the stage_config.json was actually removed.
	path := filepath.Join(projectDir, ".paulette", "stage_config.json")
	if _, err := os.Stat(path); err == nil {
		t.Error("want stage_config.json to be removed after reset, but it still exists")
	}
}

func TestResetProjectOverrides_UnknownProject_Returns404(t *testing.T) {
	_, router, _ := newTestHandlerSetup(t)

	rr := doRequest(t, router, http.MethodPost, "/api/projects/no-proj/config/stages/reset", nil)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestResetProjectOverrides_NoOverridesFile_Returns200(t *testing.T) {
	// Reset on a project that has no stage_config.json yet should succeed silently.
	_, router, _ := newTestHandlerSetup(t)

	rr := doRequest(t, router, http.MethodPost, "/api/projects/proj-1/config/stages/reset", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rr.Code, rr.Body)
	}
}
