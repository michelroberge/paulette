// Package provider — Claude CLI fallback regression tests (Milestone 6.1).
//
// These tests confirm that with no connections.json or config.json on disk,
// the pipeline resolves identically to v0.1.0:
//   - Vision / UX / Complete stages → claude-sonnet-4-6 via Claude CLI
//   - Architecture / Build stages   → claude-opus-4-6   via Claude CLI
//
// No configuration is required; all stores start gracefully from empty files.
package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// ---------------------------------------------------------------------------
// FallbackModel — hardcoded stage-model mapping (level 3 fallback)
// ---------------------------------------------------------------------------

// TestFallbackModel_AllStages verifies that FallbackModel returns the correct
// model identifiers that match v0.1.0 behaviour exactly.
func TestFallbackModel_AllStages(t *testing.T) {
	cases := []struct {
		stage     model.StageName
		wantModel string
	}{
		{model.StageVision, "claude-sonnet-4-6"},
		{model.StageUX, "claude-sonnet-4-6"},
		{model.StageArchitecture, "claude-opus-4-6"},
		{model.StageBuild, "claude-opus-4-6"},
		{model.StageComplete, "claude-sonnet-4-6"},
	}

	for _, tc := range cases {
		t.Run(string(tc.stage), func(t *testing.T) {
			got := FallbackModel(tc.stage)
			if got != tc.wantModel {
				t.Errorf("FallbackModel(%s) = %q, want %q", tc.stage, got, tc.wantModel)
			}
		})
	}
}

// TestFallbackModel_UnknownStage verifies that an unrecognised stage name
// falls back to the default Sonnet model rather than returning an empty string
// that would crash the agent invocation.
func TestFallbackModel_UnknownStage(t *testing.T) {
	got := FallbackModel("unknown-stage")
	if got != "claude-sonnet-4-6" {
		t.Errorf("FallbackModel(unknown-stage) = %q, want claude-sonnet-4-6", got)
	}
}

// ---------------------------------------------------------------------------
// ClaudeCLIProvider — interface compliance and behaviour
// ---------------------------------------------------------------------------

// TestClaudeCLIProvider_ImplementsProviderInterface is a compile-time assertion
// that ClaudeCLIProvider satisfies the Provider interface. The test body is
// intentionally empty — if the interface is not satisfied the build fails here.
func TestClaudeCLIProvider_ImplementsProviderInterface(t *testing.T) {
	var _ Provider = (*ClaudeCLIProvider)(nil)
}

// TestClaudeCLIProvider_ListModels_ReturnsUnsupported verifies that ListModels
// returns ErrModelListUnsupported (not a nil error or a different error type).
// The frontend uses this signal to fall back to free-text model entry.
func TestClaudeCLIProvider_ListModels_ReturnsUnsupported(t *testing.T) {
	p := NewClaudeCLIProvider()
	models, err := p.ListModels(context.Background())

	if models != nil {
		t.Errorf("ListModels: expected nil slice, got %v", models)
	}
	if err != ErrModelListUnsupported {
		t.Errorf("ListModels: expected ErrModelListUnsupported, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ConnectionStore — graceful handling of missing connections.json
// ---------------------------------------------------------------------------

// TestConnectionStore_MissingFile_StartsEmpty verifies that a ConnectionStore
// whose backing file does not yet exist starts with zero connections rather than
// failing at construction. This is the "first run" guarantee: users who have
// never opened the Configure page must still have a working pipeline.
func TestConnectionStore_MissingFile_StartsEmpty(t *testing.T) {
	dir := t.TempDir()
	// Do NOT create connections.json — verify that the store handles a missing file.
	store := NewConnectionStore(dir)

	list := store.List()
	if len(list) != 0 {
		t.Errorf("expected empty list with no connections.json, got %d connections", len(list))
	}

	// Verify the file was not created as a side-effect of construction.
	path := filepath.Join(dir, "connections.json")
	if _, err := os.Stat(path); err == nil {
		t.Error("connections.json should not be created by NewConnectionStore alone")
	}
}

// TestConnectionStore_Get_NotFound verifies that Get on an empty store returns
// an error rather than panicking or returning a nil pointer.
func TestConnectionStore_Get_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewConnectionStore(dir)

	_, err := store.Get("does-not-exist")
	if err == nil {
		t.Error("expected error when getting connection from empty store, got nil")
	}
}

// ---------------------------------------------------------------------------
// StageConfigStore — graceful handling of missing config files
// ---------------------------------------------------------------------------

// TestStageConfigStore_MissingGlobalConfig_EmptyDefaults verifies that
// GetGlobalDefaults returns an empty config (not an error) when config.json
// does not exist. This is the critical "no configuration required" guarantee:
// v0.1.0 users who have not created any global defaults must still get the
// Claude CLI fallback.
func TestStageConfigStore_MissingGlobalConfig_EmptyDefaults(t *testing.T) {
	dir := t.TempDir()
	store := NewStageConfigStore(dir)

	cfg := store.GetGlobalDefaults()
	if len(cfg.StageDefaults) != 0 {
		t.Errorf("expected empty stageDefaults with no config.json, got %v", cfg.StageDefaults)
	}
}

// TestStageConfigStore_MissingProjectConfig_EmptyOverrides verifies that
// GetProjectOverrides returns empty overrides when stage_config.json does not
// exist in the project directory.
func TestStageConfigStore_MissingProjectConfig_EmptyOverrides(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	store := NewStageConfigStore(globalDir)

	cfg := store.GetProjectOverrides(projectDir)
	if len(cfg.Overrides) != 0 {
		t.Errorf("expected empty overrides with no stage_config.json, got %v", cfg.Overrides)
	}
}

// TestStageConfigStore_ResolveStage_MissingFiles verifies that ResolveStage
// returns nil for every pipeline stage when no config files exist. A nil return
// signals the Registry to apply the hardcoded Claude CLI fallback.
func TestStageConfigStore_ResolveStage_MissingFiles(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	store := NewStageConfigStore(globalDir)

	stages := []model.StageName{
		model.StageVision,
		model.StageUX,
		model.StageArchitecture,
		model.StageBuild,
		model.StageComplete,
	}

	for _, stage := range stages {
		t.Run(string(stage), func(t *testing.T) {
			result := store.ResolveStage(projectDir, stage)
			if result != nil {
				t.Errorf("ResolveStage(%s) = %+v, want nil (no config files)", stage, result)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Registry.ResolveForStage — full three-level fallback chain
// ---------------------------------------------------------------------------

// TestRegistry_ResolveForStage_NoConfigFiles is the core regression test.
// It verifies that with no connections.json or config.json on disk,
// ResolveForStage returns:
//   - A *ClaudeCLIProvider (not any HTTP-based provider)
//   - The correct hardcoded model for each stage (matching v0.1.0 behaviour)
//   - No error
func TestRegistry_ResolveForStage_NoConfigFiles(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	stageConfig := NewStageConfigStore(globalDir)
	registry := NewRegistry(connStore)

	// Verify no config files were created during setup.
	for _, f := range []string{"connections.json", "config.json"} {
		if _, err := os.Stat(filepath.Join(globalDir, f)); err == nil {
			t.Fatalf("setup created %s unexpectedly — test pre-condition violated", f)
		}
	}

	cases := []struct {
		stage     model.StageName
		wantModel string
	}{
		{model.StageVision, "claude-sonnet-4-6"},       // JRN-v0.1.0-002
		{model.StageUX, "claude-sonnet-4-6"},           // JRN-v0.1.0-005
		{model.StageArchitecture, "claude-opus-4-6"},   // JRN-v0.1.0-003
		{model.StageBuild, "claude-opus-4-6"},          // JRN-v0.1.0-006
		{model.StageComplete, "claude-sonnet-4-6"},
	}

	for _, tc := range cases {
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-1", tc.stage, stageConfig, projectDir)

			if err != nil {
				t.Fatalf("ResolveForStage(%s): unexpected error: %v", tc.stage, err)
			}
			if prov == nil {
				t.Fatal("ResolveForStage returned nil provider")
			}
			if _, ok := prov.(*ClaudeCLIProvider); !ok {
				t.Errorf("ResolveForStage(%s) returned %T, want *ClaudeCLIProvider", tc.stage, prov)
			}
			if modelID != tc.wantModel {
				t.Errorf("ResolveForStage(%s) model = %q, want %q", tc.stage, modelID, tc.wantModel)
			}
		})
	}
}

// TestRegistry_ResolveForStage_NilStageConfig verifies that passing a nil
// stageConfig to ResolveForStage skips both level-1 and level-2 resolution
// and always returns the Claude CLI fallback. This path is used when the
// server struct is constructed without a StageConfigStore (e.g. in tests).
func TestRegistry_ResolveForStage_NilStageConfig(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	registry := NewRegistry(connStore)

	cases := []struct {
		stage     model.StageName
		wantModel string
	}{
		{model.StageVision, "claude-sonnet-4-6"},
		{model.StageArchitecture, "claude-opus-4-6"},
		{model.StageBuild, "claude-opus-4-6"},
	}

	for _, tc := range cases {
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-1", tc.stage, nil, projectDir)

			if err != nil {
				t.Fatalf("ResolveForStage(%s) with nil stageConfig: unexpected error: %v", tc.stage, err)
			}
			if _, ok := prov.(*ClaudeCLIProvider); !ok {
				t.Errorf("expected *ClaudeCLIProvider, got %T", prov)
			}
			if modelID != tc.wantModel {
				t.Errorf("model = %q, want %q", modelID, tc.wantModel)
			}
		})
	}
}

// TestRegistry_ResolveForStage_EmptyAssignment_FallsBackToCLI verifies that a
// StageAssignment with an empty ConnectionID (which passes the validation gate
// in the config handler) does not override the Claude CLI fallback. The
// ResolveStage helper explicitly checks for non-empty ConnectionID.
func TestRegistry_ResolveForStage_EmptyAssignment_FallsBackToCLI(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	stageConfig := NewStageConfigStore(globalDir)

	// Manually write a config.json where the vision stage has an assignment
	// with an empty connectionId — should be treated as "inherit / fallback".
	writeJSONAtomic(filepath.Join(globalDir, "config.json"), GlobalConfig{
		StageDefaults: map[model.StageName]StageAssignment{
			model.StageVision: {ConnectionID: "", Model: "some-model"},
		},
	}, 0644) //nolint:errcheck // test setup

	connStore := NewConnectionStore(globalDir)
	registry := NewRegistry(connStore)

	prov, modelID, err := registry.ResolveForStage("proj-1", model.StageVision, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := prov.(*ClaudeCLIProvider); !ok {
		t.Errorf("expected *ClaudeCLIProvider for empty connectionId, got %T", prov)
	}
	if modelID != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6 fallback model, got %q", modelID)
	}
}

// ---------------------------------------------------------------------------
// Three-level priority verification
// ---------------------------------------------------------------------------

// TestFallback_ProjectOverrideTakesPrecedenceOverGlobalAndCLI verifies that a
// per-project override (level 1) takes precedence over both the global default
// (level 2) and the hardcoded Claude CLI fallback (level 3).
func TestFallback_ProjectOverrideTakesPrecedenceOverGlobalAndCLI(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	ollamaConn, err := connStore.Create(Connection{
		Name:         "project-ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)

	// Set a global default that would be used if project override were absent.
	anotherConn, _ := connStore.Create(Connection{
		Name:         "global-ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "mistral:7b",
	})
	if err := stageConfig.SetGlobalStageDefault(model.StageVision, StageAssignment{
		ConnectionID: anotherConn.ID,
		Model:        "mistral:7b",
	}); err != nil {
		t.Fatalf("SetGlobalStageDefault: %v", err)
	}

	// Set a project-level override for vision.
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageVision, &StageAssignment{
		ConnectionID: ollamaConn.ID,
		Model:        "llama3:8b",
	}); err != nil {
		t.Fatalf("SetProjectStageOverride: %v", err)
	}

	registry := NewRegistry(connStore)
	prov, modelID, err := registry.ResolveForStage("proj-1", model.StageVision, stageConfig, projectDir)

	if err != nil {
		t.Fatalf("ResolveForStage: unexpected error: %v", err)
	}
	if _, ok := prov.(*OllamaProvider); !ok {
		t.Errorf("expected *OllamaProvider (project override), got %T", prov)
	}
	if modelID != "llama3:8b" {
		t.Errorf("expected llama3:8b (project model), got %q", modelID)
	}
}

// TestFallback_GlobalDefaultTakesPrecedenceOverCLI verifies that a global
// default (level 2) takes precedence over the hardcoded Claude CLI fallback
// (level 3) when no project-level override exists.
func TestFallback_GlobalDefaultTakesPrecedenceOverCLI(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	ollamaConn, err := connStore.Create(Connection{
		Name:         "global-ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:70b",
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)
	if err := stageConfig.SetGlobalStageDefault(model.StageArchitecture, StageAssignment{
		ConnectionID: ollamaConn.ID,
		Model:        "llama3:70b",
	}); err != nil {
		t.Fatalf("SetGlobalStageDefault: %v", err)
	}

	registry := NewRegistry(connStore)
	prov, modelID, err := registry.ResolveForStage("proj-1", model.StageArchitecture, stageConfig, projectDir)

	if err != nil {
		t.Fatalf("ResolveForStage: unexpected error: %v", err)
	}
	if _, ok := prov.(*OllamaProvider); !ok {
		t.Errorf("expected *OllamaProvider (global default), got %T", prov)
	}
	if modelID != "llama3:70b" {
		t.Errorf("expected llama3:70b (global model), got %q", modelID)
	}
}

// TestFallback_DeletedConnection_FallsBackToCLI verifies that after a
// connection is deleted and its stage references are cleared, the stage reverts
// to the Claude CLI fallback. This covers the cascade deletion flow.
func TestFallback_DeletedConnection_FallsBackToCLI(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	conn, err := connStore.Create(Connection{
		Name:         "temp-ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)
	if err := stageConfig.SetGlobalStageDefault(model.StageBuild, StageAssignment{
		ConnectionID: conn.ID,
		Model:        "llama3:8b",
	}); err != nil {
		t.Fatalf("SetGlobalStageDefault: %v", err)
	}

	// Delete the connection, cascading stage reference cleanup.
	affected, err := connStore.Delete(conn.ID, stageConfig, []string{projectDir})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	t.Logf("affected stages after delete: %v", affected)

	// After deletion, build stage should fall back to Claude CLI.
	registry := NewRegistry(connStore)
	prov, modelID, err := registry.ResolveForStage("proj-1", model.StageBuild, stageConfig, projectDir)

	if err != nil {
		t.Fatalf("ResolveForStage after delete: unexpected error: %v", err)
	}
	if _, ok := prov.(*ClaudeCLIProvider); !ok {
		t.Errorf("expected *ClaudeCLIProvider after connection delete, got %T", prov)
	}
	if modelID != "claude-opus-4-6" {
		t.Errorf("expected claude-opus-4-6 fallback, got %q", modelID)
	}
}
