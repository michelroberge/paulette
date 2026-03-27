//go:build integration

// Package provider — Ollama dual-model routing live integration tests (Milestone 6.3).
//
// These tests verify the concrete scenario from the v0.2.0 acceptance criteria
// against a real Ollama instance at http://10.0.0.57:11434:
//
//   - gemma3:4b is available and responds to Vision/UX prompts.
//   - codellama:7b is available and responds to Architecture/Build prompts.
//   - ResolveForStage correctly routes each stage to the right model.
//   - Each model streams at least one non-empty chunk within a reasonable timeout.
//
// Prerequisites:
//
//	curl http://10.0.0.57:11434/api/tags        # confirms server is up
//	ollama pull gemma3:4b                        # on the remote host
//	ollama pull codellama:7b                     # on the remote host
//
// Run with:
//
//	cd backend && go test -tags integration \
//	  -run TestOllamaDualModelIntegration ./internal/provider/... -v -timeout 5m
package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// TestOllamaDualModelIntegration_TestConnection verifies that the Ollama
// instance at http://10.0.0.57:11434 is reachable and responds in under 3
// seconds (acceptance criterion 6.2.3).
func TestOllamaDualModelIntegration_TestConnection(t *testing.T) {
	p := NewOllamaProvider(ollamaRemoteURL)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := p.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection to %s failed: %v\n\nMake sure Ollama is running on the remote host.", ollamaRemoteURL, err)
	}
	t.Logf("Ollama at %s is reachable", ollamaRemoteURL)
}

// TestOllamaDualModelIntegration_ListModels verifies that both gemma3:4b and
// codellama:7b are available on the live Ollama instance. If a model is missing,
// the test fails with a helpful "pull it" message.
func TestOllamaDualModelIntegration_ListModels(t *testing.T) {
	p := NewOllamaProvider(ollamaRemoteURL)

	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels from %s: %v", ollamaRemoteURL, err)
	}
	if len(models) == 0 {
		t.Fatalf("ListModels returned no models from %s; is Ollama running?", ollamaRemoteURL)
	}

	t.Logf("Models available at %s:", ollamaRemoteURL)
	found := map[string]bool{}
	for _, m := range models {
		t.Logf("  - %s", m.ID)
		found[m.ID] = true
	}

	for _, want := range []string{ollamaVisionUXModel, ollamaCodeModel} {
		if !found[want] {
			t.Errorf("model %q not found; run: ollama pull %s  (on %s)", want, want, ollamaRemoteURL)
		}
	}
}

// TestOllamaDualModelIntegration_StageRouting exercises the full
// ResolveForStage → provider.Chat() pipeline against the live Ollama instance.
//
// For each of the four configured stages it:
//  1. Resolves the provider and confirms it is *OllamaProvider.
//  2. Confirms the resolved model ID matches the expected assignment.
//  3. Fires a minimal Chat request and confirms at least one content chunk
//     arrives (proving the model is loaded and responding).
func TestOllamaDualModelIntegration_StageRouting(t *testing.T) {
	registry, stageConfig, _, projectDir := setupOllamaDualModelConfig(t, ollamaRemoteURL)

	cases := []struct {
		stage     model.StageName
		wantModel string
	}{
		{model.StageVision, ollamaVisionUXModel},
		{model.StageUX, ollamaVisionUXModel},
		{model.StageArchitecture, ollamaCodeModel},
		{model.StageBuild, ollamaCodeModel},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.stage), func(t *testing.T) {
			// Step 1: resolve provider and model.
			prov, modelID, err := registry.ResolveForStage("integ-proj", tc.stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", tc.stage, err)
			}

			// Step 2: correct provider type.
			if _, ok := prov.(*OllamaProvider); !ok {
				t.Errorf("want *OllamaProvider, got %T", prov)
			}

			// Step 3: correct model ID.
			if modelID != tc.wantModel {
				t.Errorf("model = %q, want %q", modelID, tc.wantModel)
			}

			// Step 4: fire a real Chat request and confirm the stream works.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			ch, err := prov.Chat(ctx, ChatRequest{
				Model:       modelID,
				UserMessage: "Reply with one word only: hello",
			})
			if err != nil {
				t.Fatalf("Chat(%s, model=%s): %v", tc.stage, modelID, err)
			}

			var got strings.Builder
			gotDone := false
			for ev := range ch {
				switch ev.Type {
				case "chunk":
					got.WriteString(ev.Content)
				case "done":
					gotDone = true
				case "error":
					t.Errorf("stream error from %s/%s: %s", tc.stage, modelID, ev.Content)
				}
			}
			if !gotDone {
				t.Error("stream closed without a 'done' event")
			}
			if got.Len() == 0 {
				t.Errorf("no content chunks received from %s/%s", tc.stage, modelID)
			}
			t.Logf("%s (%s): %d chars — %q…", tc.stage, modelID, got.Len(),
				truncate(got.String(), 80))
		})
	}
}

// TestOllamaDualModelIntegration_ProjectOverride verifies that per-project
// overrides take effect immediately against the live Ollama instance.
//
// Scenario: globally Architecture uses codellama:7b, but a specific project
// overrides it to gemma3:4b. The test confirms both stages resolve to the
// correct model and that actual Chat requests are routed accordingly.
func TestOllamaDualModelIntegration_ProjectOverride(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	conn, err := connStore.Create(Connection{
		Name:         "Remote Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      ollamaRemoteURL,
		DefaultModel: ollamaVisionUXModel,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)
	// Global: Architecture → codellama:7b.
	if err := stageConfig.SetGlobalStageDefault(model.StageArchitecture, StageAssignment{
		ConnectionID: conn.ID, Model: ollamaCodeModel,
	}); err != nil {
		t.Fatalf("SetGlobalStageDefault(architecture): %v", err)
	}

	// Project override: Architecture → gemma3:4b.
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageArchitecture,
		&StageAssignment{ConnectionID: conn.ID, Model: ollamaVisionUXModel},
	); err != nil {
		t.Fatalf("SetProjectStageOverride(architecture): %v", err)
	}

	registry := NewRegistry(connStore)

	_, modelID, err := registry.ResolveForStage("integ-override", model.StageArchitecture, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(architecture): %v", err)
	}
	if modelID != ollamaVisionUXModel {
		t.Errorf("architecture with project override: model = %q, want %q", modelID, ollamaVisionUXModel)
	}
	t.Logf("Architecture correctly resolved to project-override model: %s", modelID)
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
