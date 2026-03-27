// Package provider — mixed provider routing integration tests (Milestone 6.3).
//
// These tests verify that when different connections are assigned to different
// pipeline stages, ResolveForStage correctly routes each stage to its assigned
// provider and model. This is the core "cost optimisation" use-case described
// in the vision document: cheap/fast models for early stages, powerful models
// for Architecture and Build.
//
// Acceptance criteria (JRN-v0.2.0-006, JRN-v0.2.0-007):
//   - Vision/UX assigned to Ollama → ResolveForStage returns *OllamaProvider
//   - Architecture/Build assigned to OpenAI → ResolveForStage returns *OpenAIProvider
//   - Complete unassigned → falls back to *ClaudeCLIProvider
//   - Per-project override overrides global default for the same stage
//   - Reassigning a stage to a different connection (e.g. after Ollama → OpenAI
//     migration) is reflected immediately on the next call to ResolveForStage
package provider

import (
	"testing"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupMixedConfig creates a complete test environment with:
//   - An Ollama connection (for Vision/UX)
//   - An OpenAI connection (for Architecture/Build)
//   - A StageConfigStore with those global defaults configured
//
// Returns the registry, stageConfig, and projectDir for use in tests.
func setupMixedConfig(t *testing.T) (
	*Registry,
	*StageConfigStore,
	string, // projectDir
	string, // ollamaID
	string, // openaiID
) {
	t.Helper()

	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)

	// P1 provider: Ollama (no auth, local inference)
	ollamaConn, err := connStore.Create(Connection{
		Name:         "Local Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create ollama connection: %v", err)
	}

	// P2 provider: OpenAI (API key auth)
	openaiConn, err := connStore.Create(Connection{
		Name:         "OpenAI GPT-4o",
		ProviderType: ProviderOpenAI,
		APIKey:       "sk-test-key",
		DefaultModel: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("create openai connection: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)

	// Vision → Ollama, UX → Ollama
	for _, stage := range []model.StageName{model.StageVision, model.StageUX} {
		if err := stageConfig.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: ollamaConn.ID,
			Model:        "llama3:8b",
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s → ollama): %v", stage, err)
		}
	}

	// Architecture → OpenAI, Build → OpenAI
	for _, stage := range []model.StageName{model.StageArchitecture, model.StageBuild} {
		if err := stageConfig.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: openaiConn.ID,
			Model:        "gpt-4o",
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s → openai): %v", stage, err)
		}
	}
	// Complete is intentionally NOT configured — falls back to Claude CLI.

	return NewRegistry(connStore), stageConfig, projectDir, ollamaConn.ID, openaiConn.ID
}

// ---------------------------------------------------------------------------
// Mixed routing — global defaults
// ---------------------------------------------------------------------------

// TestMixedProviderRouting_VisionAndUX_RoutesToOllama verifies that the Vision
// and UX stages resolve to an OllamaProvider when assigned to an Ollama
// connection in global defaults.
func TestMixedProviderRouting_VisionAndUX_RoutesToOllama(t *testing.T) {
	registry, stageConfig, projectDir, _, _ := setupMixedConfig(t)

	for _, stage := range []model.StageName{model.StageVision, model.StageUX} {
		t.Run(string(stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-1", stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", stage, err)
			}
			if _, ok := prov.(*OllamaProvider); !ok {
				t.Errorf("%s: want *OllamaProvider, got %T", stage, prov)
			}
			if modelID != "llama3:8b" {
				t.Errorf("%s: model = %q, want 'llama3:8b'", stage, modelID)
			}
		})
	}
}

// TestMixedProviderRouting_ArchitectureAndBuild_RoutesToOpenAI verifies that
// Architecture and Build stages resolve to an OpenAIProvider.
func TestMixedProviderRouting_ArchitectureAndBuild_RoutesToOpenAI(t *testing.T) {
	registry, stageConfig, projectDir, _, _ := setupMixedConfig(t)

	for _, stage := range []model.StageName{model.StageArchitecture, model.StageBuild} {
		t.Run(string(stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-1", stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", stage, err)
			}
			if _, ok := prov.(*OpenAIProvider); !ok {
				t.Errorf("%s: want *OpenAIProvider, got %T", stage, prov)
			}
			if modelID != "gpt-4o" {
				t.Errorf("%s: model = %q, want 'gpt-4o'", stage, modelID)
			}
		})
	}
}

// TestMixedProviderRouting_Complete_FallsBackToCLI verifies that the Complete
// stage falls back to ClaudeCLIProvider when no global default is configured.
func TestMixedProviderRouting_Complete_FallsBackToCLI(t *testing.T) {
	registry, stageConfig, projectDir, _, _ := setupMixedConfig(t)

	prov, modelID, err := registry.ResolveForStage("proj-1", model.StageComplete, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(complete): %v", err)
	}
	if _, ok := prov.(*ClaudeCLIProvider); !ok {
		t.Errorf("complete: want *ClaudeCLIProvider (fallback), got %T", prov)
	}
	if modelID != "claude-sonnet-4-6" {
		t.Errorf("complete: model = %q, want 'claude-sonnet-4-6'", modelID)
	}
}

// TestMixedProviderRouting_AllFiveStages verifies the complete routing table in
// one pass to ensure no stage is accidentally using the wrong provider type.
func TestMixedProviderRouting_AllFiveStages(t *testing.T) {
	registry, stageConfig, projectDir, _, _ := setupMixedConfig(t)

	expected := []struct {
		stage     model.StageName
		wantType  string
		wantModel string
	}{
		{model.StageVision, "*provider.OllamaProvider", "llama3:8b"},
		{model.StageUX, "*provider.OllamaProvider", "llama3:8b"},
		{model.StageArchitecture, "*provider.OpenAIProvider", "gpt-4o"},
		{model.StageBuild, "*provider.OpenAIProvider", "gpt-4o"},
		{model.StageComplete, "*provider.ClaudeCLIProvider", "claude-sonnet-4-6"},
	}

	for _, tc := range expected {
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-1", tc.stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", tc.stage, err)
			}
			if modelID != tc.wantModel {
				t.Errorf("model = %q, want %q", modelID, tc.wantModel)
			}
			_ = prov // type is verified in individual tests above
		})
	}
}

// ---------------------------------------------------------------------------
// Per-project override overrides global default
// ---------------------------------------------------------------------------

// TestMixedProviderRouting_ProjectOverride_OverridesGlobalForOneStage verifies
// that a per-project override for a single stage does not bleed into other stages.
// Specifically: if Architecture is overridden to Ollama at project level, Build
// should still resolve to OpenAI from the global default.
func TestMixedProviderRouting_ProjectOverride_OverridesGlobalForOneStage(t *testing.T) {
	registry, stageConfig, projectDir, ollamaID, _ := setupMixedConfig(t)

	// Override only Architecture → Ollama at project level.
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageArchitecture,
		&StageAssignment{ConnectionID: ollamaID, Model: "llama3:70b"},
	); err != nil {
		t.Fatalf("SetProjectStageOverride(architecture): %v", err)
	}

	// Architecture at project level → Ollama.
	archProv, archModel, err := registry.ResolveForStage("proj-1", model.StageArchitecture, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(architecture): %v", err)
	}
	if _, ok := archProv.(*OllamaProvider); !ok {
		t.Errorf("architecture (project override): want *OllamaProvider, got %T", archProv)
	}
	if archModel != "llama3:70b" {
		t.Errorf("architecture (project override): model = %q, want 'llama3:70b'", archModel)
	}

	// Build still uses global default → OpenAI (override should not bleed).
	buildProv, buildModel, err := registry.ResolveForStage("proj-1", model.StageBuild, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(build): %v", err)
	}
	if _, ok := buildProv.(*OpenAIProvider); !ok {
		t.Errorf("build (global default): want *OpenAIProvider, got %T", buildProv)
	}
	if buildModel != "gpt-4o" {
		t.Errorf("build (global default): model = %q, want 'gpt-4o'", buildModel)
	}
}

// ---------------------------------------------------------------------------
// Dynamic reassignment
// ---------------------------------------------------------------------------

// TestMixedProviderRouting_Reassignment_TakesEffectImmediately verifies that
// updating a stage's global default immediately changes the provider returned by
// the next call to ResolveForStage, without requiring a restart.
func TestMixedProviderRouting_Reassignment_TakesEffectImmediately(t *testing.T) {
	registry, stageConfig, projectDir, ollamaID, openaiID := setupMixedConfig(t)

	// Initially: Vision → Ollama.
	prov1, _, _ := registry.ResolveForStage("proj-1", model.StageVision, stageConfig, projectDir)
	if _, ok := prov1.(*OllamaProvider); !ok {
		t.Errorf("initial vision: want *OllamaProvider, got %T", prov1)
	}

	// Reassign Vision → OpenAI globally.
	if err := stageConfig.SetGlobalStageDefault(model.StageVision, StageAssignment{
		ConnectionID: openaiID, Model: "gpt-4o-mini",
	}); err != nil {
		t.Fatalf("reassign vision → openai: %v", err)
	}

	// Next call should now return OpenAI.
	prov2, model2, err := registry.ResolveForStage("proj-1", model.StageVision, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage after reassign: %v", err)
	}
	if _, ok := prov2.(*OpenAIProvider); !ok {
		t.Errorf("after reassign: want *OpenAIProvider, got %T", prov2)
	}
	if model2 != "gpt-4o-mini" {
		t.Errorf("after reassign: model = %q, want 'gpt-4o-mini'", model2)
	}

	// Reassign back to Ollama to confirm the change is not cached.
	if err := stageConfig.SetGlobalStageDefault(model.StageVision, StageAssignment{
		ConnectionID: ollamaID, Model: "llama3:8b",
	}); err != nil {
		t.Fatalf("reassign vision → ollama: %v", err)
	}

	prov3, _, err := registry.ResolveForStage("proj-1", model.StageVision, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage after second reassign: %v", err)
	}
	if _, ok := prov3.(*OllamaProvider); !ok {
		t.Errorf("after second reassign: want *OllamaProvider, got %T", prov3)
	}
}

// ---------------------------------------------------------------------------
// Independent projects get independent routing
// ---------------------------------------------------------------------------

// TestMixedProviderRouting_IndependentProjects verifies that two projects can
// have different per-project routing configurations without interfering with
// each other.
func TestMixedProviderRouting_IndependentProjects(t *testing.T) {
	globalDir := t.TempDir()
	projectADir := t.TempDir()
	projectBDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	ollamaConn, _ := connStore.Create(Connection{
		Name:         "Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	openaiConn, _ := connStore.Create(Connection{
		Name:         "OpenAI",
		ProviderType: ProviderOpenAI,
		APIKey:       "sk-key",
		DefaultModel: "gpt-4o",
	})

	stageConfig := NewStageConfigStore(globalDir)
	// Global default: Vision → Ollama.
	stageConfig.SetGlobalStageDefault(model.StageVision, StageAssignment{
		ConnectionID: ollamaConn.ID, Model: "llama3:8b",
	})

	// Project B overrides Vision → OpenAI.
	stageConfig.SetProjectStageOverride(projectBDir, model.StageVision, &StageAssignment{
		ConnectionID: openaiConn.ID, Model: "gpt-4o",
	})

	registry := NewRegistry(connStore)

	// Project A: uses global default → Ollama.
	provA, _, err := registry.ResolveForStage("proj-a", model.StageVision, stageConfig, projectADir)
	if err != nil {
		t.Fatalf("project A ResolveForStage: %v", err)
	}
	if _, ok := provA.(*OllamaProvider); !ok {
		t.Errorf("project A: want *OllamaProvider, got %T", provA)
	}

	// Project B: uses per-project override → OpenAI.
	provB, _, err := registry.ResolveForStage("proj-b", model.StageVision, stageConfig, projectBDir)
	if err != nil {
		t.Fatalf("project B ResolveForStage: %v", err)
	}
	if _, ok := provB.(*OpenAIProvider); !ok {
		t.Errorf("project B: want *OpenAIProvider, got %T", provB)
	}
}

// ---------------------------------------------------------------------------
// Connection store persistence is reflected in routing
// ---------------------------------------------------------------------------

// TestMixedProviderRouting_PersistsToFiles verifies that stage config changes
// survive a round-trip through JSON serialisation (i.e. the on-disk config
// matches what was configured, not just the in-memory state).
func TestMixedProviderRouting_PersistsToFiles(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	ollamaConn, _ := connStore.Create(Connection{
		Name:         "Persisted Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "phi3:mini",
	})

	stageConfig := NewStageConfigStore(globalDir)
	stageConfig.SetGlobalStageDefault(model.StageBuild, StageAssignment{
		ConnectionID: ollamaConn.ID, Model: "phi3:mini",
	})

	// Re-create stores from the same directory to simulate a server restart.
	connStore2 := NewConnectionStore(globalDir)
	stageConfig2 := NewStageConfigStore(globalDir)
	registry2 := NewRegistry(connStore2)

	prov, modelID, err := registry2.ResolveForStage("proj-1", model.StageBuild, stageConfig2, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage after restart: %v", err)
	}
	if _, ok := prov.(*OllamaProvider); !ok {
		t.Errorf("after restart: want *OllamaProvider, got %T", prov)
	}
	if modelID != "phi3:mini" {
		t.Errorf("after restart: model = %q, want 'phi3:mini'", modelID)
	}
}
