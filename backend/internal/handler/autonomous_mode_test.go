// Package handler — autonomous mode provider regression tests (Milestone 6.7).
//
// These tests verify that the provider resolution path used during autonomous
// (hands-free) mode correctly falls back to Claude CLI when no provider is
// configured, and correctly routes to configured providers when they exist.
//
// The autonomous orchestrator advances the pipeline without human approval
// by calling the same internal pipeline mechanics as manual mode. These tests
// validate the handler-level resolveProvider method (the entry point for both
// manual Send and autonomous advance) under conditions relevant to autonomous
// operation:
//
//   - No configuration: every stage falls back to Claude CLI (v0.1.0 parity)
//   - With Ollama configured globally: all stages resolve to Ollama
//   - With mixed config: each stage resolves to its assigned provider
//   - Stage-by-stage resolution is deterministic (no shared mutable state)
package handler

import (
	"testing"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// buildAutonomousHandler creates a ChatHandler wired with real stores backed
// by temp directories, simulating the handler configuration the server creates
// at startup. Returns the handler, connStore and stageConfig for seeding.
func buildAutonomousHandler(t *testing.T) (
	*ChatHandler,
	*provider.ConnectionStore,
	*provider.StageConfigStore,
	string, // projectDir
) {
	t.Helper()

	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := provider.NewConnectionStore(globalDir)
	stageConfig := provider.NewStageConfigStore(globalDir)
	provRegistry := provider.NewRegistry(connStore)

	runs := stream.NewManager()
	h := NewChatHandler(
		&mockRegistryRepo{},
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		provRegistry,
		stageConfig,
		connStore,
	)

	return h, connStore, stageConfig, projectDir
}

// ---------------------------------------------------------------------------
// Autonomous mode — no configuration (M6.1 regression)
// ---------------------------------------------------------------------------

// TestAutonomousMode_NoConfig_AllStagesFallbackToCLI verifies that the
// autonomous pipeline works identically to v0.1.0 when no configuration exists.
// This is the "zero regression" requirement from the definition of done.
func TestAutonomousMode_NoConfig_AllStagesFallbackToCLI(t *testing.T) {
	h, _, _, projectDir := buildAutonomousHandler(t)

	stages := []struct {
		stage     model.StageName
		wantModel string
	}{
		{model.StageVision, "claude-sonnet-4-6"},
		{model.StageUX, "claude-sonnet-4-6"},
		{model.StageArchitecture, "claude-opus-4-6"},
		{model.StageBuild, "claude-opus-4-6"},
		{model.StageComplete, "claude-sonnet-4-6"},
	}

	for _, tc := range stages {
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, modelID, err := h.resolveProvider("proj-auto", projectDir, tc.stage)

			if err != nil {
				t.Fatalf("resolveProvider(%s): unexpected error: %v", tc.stage, err)
			}
			if _, ok := prov.(*provider.ClaudeCLIProvider); !ok {
				t.Errorf("%s: want *ClaudeCLIProvider, got %T", tc.stage, prov)
			}
			if modelID != tc.wantModel {
				t.Errorf("%s: model = %q, want %q", tc.stage, modelID, tc.wantModel)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Autonomous mode — Ollama only (M6.2 parity)
// ---------------------------------------------------------------------------

// TestAutonomousMode_OllamaOnly_AllStagesResolveToOllama verifies that
// assigning Ollama to all five stages via global defaults causes every
// autonomous stage advance to route through the OllamaProvider. This is the
// "no Claude CLI installed" scenario from the success metrics.
func TestAutonomousMode_OllamaOnly_AllStagesResolveToOllama(t *testing.T) {
	h, connStore, stageConfig, projectDir := buildAutonomousHandler(t)

	conn, err := connStore.Create(provider.Connection{
		Name:         "Autonomous Ollama",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create ollama connection: %v", err)
	}

	for _, stage := range []model.StageName{
		model.StageVision, model.StageUX, model.StageArchitecture,
		model.StageBuild, model.StageComplete,
	} {
		if err := stageConfig.SetGlobalStageDefault(stage, provider.StageAssignment{
			ConnectionID: conn.ID, Model: "llama3:8b",
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s): %v", stage, err)
		}
	}

	for _, stage := range []model.StageName{
		model.StageVision, model.StageUX, model.StageArchitecture,
		model.StageBuild, model.StageComplete,
	} {
		t.Run(string(stage), func(t *testing.T) {
			prov, modelID, err := h.resolveProvider("proj-ollama-auto", projectDir, stage)
			if err != nil {
				t.Fatalf("resolveProvider(%s): %v", stage, err)
			}
			if _, ok := prov.(*provider.OllamaProvider); !ok {
				t.Errorf("%s: want *OllamaProvider, got %T", stage, prov)
			}
			if modelID != "llama3:8b" {
				t.Errorf("%s: model = %q, want 'llama3:8b'", stage, modelID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Autonomous mode — mixed providers (M6.3 parity)
// ---------------------------------------------------------------------------

// TestAutonomousMode_MixedProviders_EachStageResolvesCorrectly mirrors the
// mixed provider routing tests at the handler level, verifying that the
// resolveProvider method correctly dispatches each stage.
func TestAutonomousMode_MixedProviders_EachStageResolvesCorrectly(t *testing.T) {
	h, connStore, stageConfig, projectDir := buildAutonomousHandler(t)

	ollamaConn, _ := connStore.Create(provider.Connection{
		Name:         "Ollama Fast",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	openaiConn, _ := connStore.Create(provider.Connection{
		Name:         "OpenAI Powerful",
		ProviderType: provider.ProviderOpenAI,
		APIKey:       "sk-autonomous-test",
		DefaultModel: "gpt-4o",
	})

	// Cheap model for early stages.
	for _, stage := range []model.StageName{model.StageVision, model.StageUX} {
		stageConfig.SetGlobalStageDefault(stage, provider.StageAssignment{
			ConnectionID: ollamaConn.ID, Model: "llama3:8b",
		})
	}
	// Powerful model for late stages.
	for _, stage := range []model.StageName{model.StageArchitecture, model.StageBuild} {
		stageConfig.SetGlobalStageDefault(stage, provider.StageAssignment{
			ConnectionID: openaiConn.ID, Model: "gpt-4o",
		})
	}
	// Complete: unassigned → Claude CLI fallback.

	cases := []struct {
		stage    model.StageName
		wantType string
	}{
		{model.StageVision, "ollama"},
		{model.StageUX, "ollama"},
		{model.StageArchitecture, "openai"},
		{model.StageBuild, "openai"},
		{model.StageComplete, "claude_cli"},
	}

	for _, tc := range cases {
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, _, err := h.resolveProvider("proj-mixed-auto", projectDir, tc.stage)
			if err != nil {
				t.Fatalf("resolveProvider(%s): %v", tc.stage, err)
			}
			switch tc.wantType {
			case "ollama":
				if _, ok := prov.(*provider.OllamaProvider); !ok {
					t.Errorf("%s: want *OllamaProvider, got %T", tc.stage, prov)
				}
			case "openai":
				if _, ok := prov.(*provider.OpenAIProvider); !ok {
					t.Errorf("%s: want *OpenAIProvider, got %T", tc.stage, prov)
				}
			case "claude_cli":
				if _, ok := prov.(*provider.ClaudeCLIProvider); !ok {
					t.Errorf("%s: want *ClaudeCLIProvider (fallback), got %T", tc.stage, prov)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Autonomous mode — determinism (no shared state mutation between calls)
// ---------------------------------------------------------------------------

// TestAutonomousMode_Determinism_RepeatCallsReturnSameProvider verifies that
// calling resolveProvider twice for the same stage returns the same provider
// type and model each time. This is critical for autonomous mode where the
// orchestrator may call resolveProvider multiple times (e.g. on retry).
func TestAutonomousMode_Determinism_RepeatCallsReturnSameProvider(t *testing.T) {
	h, connStore, stageConfig, projectDir := buildAutonomousHandler(t)

	conn, _ := connStore.Create(provider.Connection{
		Name:         "Stable Ollama",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	stageConfig.SetGlobalStageDefault(model.StageBuild, provider.StageAssignment{
		ConnectionID: conn.ID, Model: "llama3:8b",
	})

	const iterations = 5
	for i := 0; i < iterations; i++ {
		prov, modelID, err := h.resolveProvider("proj-det", projectDir, model.StageBuild)
		if err != nil {
			t.Fatalf("iteration %d: resolveProvider: %v", i, err)
		}
		if _, ok := prov.(*provider.OllamaProvider); !ok {
			t.Errorf("iteration %d: want *OllamaProvider, got %T", i, prov)
		}
		if modelID != "llama3:8b" {
			t.Errorf("iteration %d: model = %q, want 'llama3:8b'", i, modelID)
		}
	}
}
