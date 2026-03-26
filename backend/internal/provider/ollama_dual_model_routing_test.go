// Package provider — Ollama dual-model routing tests (Milestone 6.3).
//
// These tests verify the concrete mixed-provider scenario described in the
// v0.2.0 acceptance criteria for JRN-v0.2.0-006 and JRN-v0.2.0-007:
//
//   - A single Ollama instance serves all pipeline stages.
//   - Vision and UX use gemma3:4b  (fast, small model for early creative stages).
//   - Architecture and Build use codellama:7b  (code-specialised model).
//   - Complete falls back to the hardcoded Claude CLI default.
//   - Per-project overrides can swap individual stages without affecting others.
//   - Configuration survives a simulated server restart (JSON round-trip).
//
// Unit tests (no build tag): use an httptest mock Ollama server — no live
// instance required.
//
// Integration tests (//go:build integration): require a live Ollama instance at
// http://10.0.0.57:11434 with gemma3:4b and codellama:7b pulled and available.
// Run with:
//
//	go test -tags integration ./backend/internal/provider/... \
//	  -run TestOllamaDualModel -v
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/michelroberge/paulette/backend/internal/model"
)


// ---------------------------------------------------------------------------
// Test constants
// ---------------------------------------------------------------------------

const (
	// ollamaRemoteURL is the live Ollama host used in integration tests.
	ollamaRemoteURL = "http://10.0.0.57:11434"

	// ollamaVisionUXModel is the lightweight model assigned to Vision and UX.
	ollamaVisionUXModel = "gemma3:4b"

	// ollamaCodeModel is the code-specialised model assigned to Architecture and Build.
	ollamaCodeModel = "codellama:7b"
)

// ---------------------------------------------------------------------------
// Mock Ollama server for dual-model tests
// ---------------------------------------------------------------------------

// ollamaDualModelMockServer creates a test HTTP server that exposes both models
// (gemma3:4b and codellama:7b) via GET /api/tags and streams simple text
// chunks from POST /api/chat. The server records which model each /api/chat
// request uses so tests can assert correct routing.
func ollamaDualModelMockServer(t *testing.T) (srv *httptest.Server, lastChatModel func() string) {
	t.Helper()

	var recorded string

	mux := http.NewServeMux()

	// GET /api/tags — advertise both models.
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		type entry struct {
			Name string `json:"name"`
		}
		type tagsResp struct {
			Models []entry `json:"models"`
		}
		resp := tagsResp{
			Models: []entry{
				{Name: ollamaVisionUXModel},
				{Name: ollamaCodeModel},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// POST /api/chat — record the requested model and stream a short response.
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the request to capture the model field.
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		recorded = req.Model

		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		type msgContent struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		type chunk struct {
			Message msgContent `json:"message"`
			Done    bool       `json:"done"`
		}

		// Stream a minimal XML-envelope response so the parser stays happy.
		content := fmt.Sprintf(
			"<response><discussion>using %s</discussion><artifact>artifact</artifact></response>",
			req.Model,
		)
		line, _ := json.Marshal(chunk{Message: msgContent{Role: "assistant", Content: content}})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()

		doneLine, _ := json.Marshal(chunk{Done: true})
		fmt.Fprintf(w, "%s\n", doneLine)
		flusher.Flush()
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() string { return recorded }
}

// ---------------------------------------------------------------------------
// Setup helper
// ---------------------------------------------------------------------------

// setupOllamaDualModelConfig builds the full test environment:
//   - One Ollama connection pointing at the given baseURL.
//   - Global stage defaults: Vision/UX → gemma3:4b, Architecture/Build → codellama:7b.
//   - Complete is intentionally left unconfigured (falls back to Claude CLI).
//
// Returns the registry, stageConfig, the connection ID, and a temp project dir.
func setupOllamaDualModelConfig(t *testing.T, ollamaBaseURL string) (
	*Registry,
	*StageConfigStore,
	string, // ollamaConnectionID
	string, // projectDir
) {
	t.Helper()

	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)

	// Single Ollama connection — same endpoint serves both models.
	conn, err := connStore.Create(Connection{
		Name:         "Ollama Dual-Model",
		ProviderType: ProviderOllama,
		BaseURL:      ollamaBaseURL,
		DefaultModel: ollamaVisionUXModel, // connection default; stage config overrides per stage
	})
	if err != nil {
		t.Fatalf("create ollama connection: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)

	// Vision and UX → gemma3:4b (lightweight creative model).
	for _, stage := range []model.StageName{model.StageVision, model.StageUX} {
		if err := stageConfig.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: conn.ID,
			Model:        ollamaVisionUXModel,
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s): %v", stage, err)
		}
	}

	// Architecture and Build → codellama:7b (code-specialised model).
	for _, stage := range []model.StageName{model.StageArchitecture, model.StageBuild} {
		if err := stageConfig.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: conn.ID,
			Model:        ollamaCodeModel,
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s): %v", stage, err)
		}
	}
	// Complete deliberately left unconfigured → falls back to Claude CLI.

	return NewRegistry(connStore), stageConfig, conn.ID, projectDir
}

// ---------------------------------------------------------------------------
// Unit tests — mock Ollama server, no live instance required
// ---------------------------------------------------------------------------

// TestOllamaDualModel_VisionAndUX_ResolveToGemma3 verifies that Vision and UX
// stages resolve to an OllamaProvider configured with gemma3:4b.
func TestOllamaDualModel_VisionAndUX_ResolveToGemma3(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	registry, stageConfig, _, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	for _, stage := range []model.StageName{model.StageVision, model.StageUX} {
		t.Run(string(stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-dual", stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", stage, err)
			}

			if _, ok := prov.(*OllamaProvider); !ok {
				t.Errorf("%s: want *OllamaProvider, got %T", stage, prov)
			}
			if modelID != ollamaVisionUXModel {
				t.Errorf("%s: model = %q, want %q", stage, modelID, ollamaVisionUXModel)
			}
		})
	}
}

// TestOllamaDualModel_ArchitectureAndBuild_ResolveToCodellama verifies that
// Architecture and Build stages resolve to an OllamaProvider with codellama:7b.
func TestOllamaDualModel_ArchitectureAndBuild_ResolveToCodellama(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	registry, stageConfig, _, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	for _, stage := range []model.StageName{model.StageArchitecture, model.StageBuild} {
		t.Run(string(stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-dual", stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", stage, err)
			}

			if _, ok := prov.(*OllamaProvider); !ok {
				t.Errorf("%s: want *OllamaProvider, got %T", stage, prov)
			}
			if modelID != ollamaCodeModel {
				t.Errorf("%s: model = %q, want %q", stage, modelID, ollamaCodeModel)
			}
		})
	}
}

// TestOllamaDualModel_Complete_FallsBackToCLI verifies that the Complete stage
// falls back to ClaudeCLIProvider because it has no configured global default.
func TestOllamaDualModel_Complete_FallsBackToCLI(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	registry, stageConfig, _, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	prov, modelID, err := registry.ResolveForStage("proj-dual", model.StageComplete, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(complete): %v", err)
	}
	if _, ok := prov.(*ClaudeCLIProvider); !ok {
		t.Errorf("complete: want *ClaudeCLIProvider (fallback), got %T", prov)
	}
	// The fallback model for Complete is claude-sonnet-4-6 per stageModels map.
	if modelID != "claude-sonnet-4-6" {
		t.Errorf("complete: model = %q, want 'claude-sonnet-4-6'", modelID)
	}
}

// TestOllamaDualModel_AllFiveStages verifies the complete routing table in one
// pass — provider type and model must both be correct for every stage.
func TestOllamaDualModel_AllFiveStages(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	registry, stageConfig, _, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	cases := []struct {
		stage     model.StageName
		wantOllama bool   // true → *OllamaProvider; false → *ClaudeCLIProvider
		wantModel  string
	}{
		{model.StageVision, true, ollamaVisionUXModel},
		{model.StageUX, true, ollamaVisionUXModel},
		{model.StageArchitecture, true, ollamaCodeModel},
		{model.StageBuild, true, ollamaCodeModel},
		{model.StageComplete, false, "claude-sonnet-4-6"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-dual", tc.stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", tc.stage, err)
			}
			if modelID != tc.wantModel {
				t.Errorf("model = %q, want %q", modelID, tc.wantModel)
			}
			if tc.wantOllama {
				if _, ok := prov.(*OllamaProvider); !ok {
					t.Errorf("want *OllamaProvider, got %T", prov)
				}
			} else {
				if _, ok := prov.(*ClaudeCLIProvider); !ok {
					t.Errorf("want *ClaudeCLIProvider, got %T", prov)
				}
			}
		})
	}
}

// TestOllamaDualModel_Chat_ReceivesCorrectModel verifies that when Chat is
// called with the resolved provider and model, the Ollama server receives the
// correct model name in the POST /api/chat request body.
func TestOllamaDualModel_Chat_ReceivesCorrectModel(t *testing.T) {
	srv, lastChatModel := ollamaDualModelMockServer(t)
	registry, stageConfig, _, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	stages := []struct {
		stage      model.StageName
		wantModel  string
	}{
		{model.StageVision, ollamaVisionUXModel},
		{model.StageArchitecture, ollamaCodeModel},
	}

	for _, tc := range stages {
		tc := tc
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-chat", tc.stage, stageConfig, projectDir)
			if err != nil {
				t.Fatalf("ResolveForStage(%s): %v", tc.stage, err)
			}

			// Invoke Chat with the resolved model and drain the stream.
			ch, err := prov.Chat(context.Background(), ChatRequest{
				Model:       modelID,
				UserMessage: "hello",
			})
			if err != nil {
				t.Fatalf("Chat(%s): %v", tc.stage, err)
			}
			var got strings.Builder
			for ev := range ch {
				if ev.Type == "chunk" {
					got.WriteString(ev.Content)
				}
			}

			// The mock server records the model sent in the request.
			if gotModel := lastChatModel(); gotModel != tc.wantModel {
				t.Errorf("Ollama received model=%q, want %q", gotModel, tc.wantModel)
			}
		})
	}
}

// TestOllamaDualModel_ModelDiscovery verifies that ListModels returns both
// gemma3:4b and codellama:7b from a configured Ollama connection.
func TestOllamaDualModel_ModelDiscovery(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	p := NewOllamaProvider(srv.URL)

	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	wantModels := map[string]bool{
		ollamaVisionUXModel: false,
		ollamaCodeModel:     false,
	}
	for _, m := range models {
		if _, ok := wantModels[m.ID]; ok {
			wantModels[m.ID] = true
		}
	}
	for name, found := range wantModels {
		if !found {
			t.Errorf("model %q not found in ListModels response", name)
		}
	}
}

// TestOllamaDualModel_ProjectOverride_SwitchesArchitectureModel verifies that a
// per-project override for Architecture (e.g. switching to gemma3:4b for a small
// exploratory project) does not bleed into the Build stage which still uses
// codellama:7b from the global default.
func TestOllamaDualModel_ProjectOverride_SwitchesArchitectureModel(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	registry, stageConfig, connID, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	// Override Architecture → gemma3:4b at project level (e.g. cost optimisation).
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageArchitecture,
		&StageAssignment{ConnectionID: connID, Model: ollamaVisionUXModel},
	); err != nil {
		t.Fatalf("SetProjectStageOverride(architecture): %v", err)
	}

	// Architecture at project level → gemma3:4b.
	_, archModel, err := registry.ResolveForStage("proj-override", model.StageArchitecture, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(architecture): %v", err)
	}
	if archModel != ollamaVisionUXModel {
		t.Errorf("architecture (project override): model = %q, want %q", archModel, ollamaVisionUXModel)
	}

	// Build still uses global default → codellama:7b (override must not bleed).
	_, buildModel, err := registry.ResolveForStage("proj-override", model.StageBuild, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(build): %v", err)
	}
	if buildModel != ollamaCodeModel {
		t.Errorf("build (global default): model = %q, want %q", buildModel, ollamaCodeModel)
	}
}

// TestOllamaDualModel_InheritRestoredAfterClear verifies that clearing a
// per-project override (by setting it to nil) causes the stage to revert to
// the global default.
func TestOllamaDualModel_InheritRestoredAfterClear(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	registry, stageConfig, connID, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	// Override Vision → codellama:7b at project level.
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageVision,
		&StageAssignment{ConnectionID: connID, Model: ollamaCodeModel},
	); err != nil {
		t.Fatalf("SetProjectStageOverride(vision): %v", err)
	}

	// Confirm override is active.
	_, m1, _ := registry.ResolveForStage("proj-inherit", model.StageVision, stageConfig, projectDir)
	if m1 != ollamaCodeModel {
		t.Fatalf("before clear: vision model = %q, want %q", m1, ollamaCodeModel)
	}

	// Clear the override (nil = inherit).
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageVision, nil); err != nil {
		t.Fatalf("clear vision override: %v", err)
	}

	// Vision should now fall back to the global default (gemma3:4b).
	_, m2, err := registry.ResolveForStage("proj-inherit", model.StageVision, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage after clear: %v", err)
	}
	if m2 != ollamaVisionUXModel {
		t.Errorf("after clear: vision model = %q, want %q", m2, ollamaVisionUXModel)
	}
}

// TestOllamaDualModel_Persistence verifies that the stage assignments survive a
// JSON round-trip (i.e. simulate a server restart by constructing new store
// instances from the same directory).
func TestOllamaDualModel_Persistence(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// --- First "server run" ---
	connStore1 := NewConnectionStore(globalDir)
	conn, err := connStore1.Create(Connection{
		Name:         "Ollama Restart Test",
		ProviderType: ProviderOllama,
		BaseURL:      srv.URL,
		DefaultModel: ollamaVisionUXModel,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	stageConfig1 := NewStageConfigStore(globalDir)
	for _, stage := range []model.StageName{model.StageVision, model.StageUX} {
		_ = stageConfig1.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: conn.ID, Model: ollamaVisionUXModel,
		})
	}
	for _, stage := range []model.StageName{model.StageArchitecture, model.StageBuild} {
		_ = stageConfig1.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: conn.ID, Model: ollamaCodeModel,
		})
	}
	// Add a project-level override: Architecture → gemma3:4b.
	_ = stageConfig1.SetProjectStageOverride(projectDir, model.StageArchitecture,
		&StageAssignment{ConnectionID: conn.ID, Model: ollamaVisionUXModel},
	)

	// --- Simulated restart: new instances from same directory ---
	connStore2 := NewConnectionStore(globalDir)
	stageConfig2 := NewStageConfigStore(globalDir)
	registry2 := NewRegistry(connStore2)

	// Global defaults survived restart.
	for _, tc := range []struct {
		stage     model.StageName
		wantModel string
	}{
		{model.StageVision, ollamaVisionUXModel},
		{model.StageUX, ollamaVisionUXModel},
		{model.StageBuild, ollamaCodeModel},
	} {
		_, m, err := registry2.ResolveForStage("proj-restart", tc.stage, stageConfig2, projectDir)
		// For stages with project overrides (Architecture), test separately below.
		if tc.stage == model.StageArchitecture {
			continue
		}
		if err != nil {
			t.Errorf("ResolveForStage(%s) after restart: %v", tc.stage, err)
			continue
		}
		if m != tc.wantModel {
			t.Errorf("%s after restart: model = %q, want %q", tc.stage, m, tc.wantModel)
		}
	}

	// Project-level override survived restart: Architecture → gemma3:4b.
	_, archModel, err := registry2.ResolveForStage("proj-restart", model.StageArchitecture, stageConfig2, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(architecture) after restart: %v", err)
	}
	if archModel != ollamaVisionUXModel {
		t.Errorf("architecture override after restart: model = %q, want %q", archModel, ollamaVisionUXModel)
	}
}

// TestOllamaDualModel_DynamicReassignment verifies that changing a global stage
// default (e.g. swapping Vision from gemma3:4b to codellama:7b) is reflected
// immediately on the next ResolveForStage call without a restart.
func TestOllamaDualModel_DynamicReassignment(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)
	registry, stageConfig, connID, projectDir := setupOllamaDualModelConfig(t, srv.URL)

	// Before: Vision → gemma3:4b.
	_, m1, _ := registry.ResolveForStage("proj-dyn", model.StageVision, stageConfig, projectDir)
	if m1 != ollamaVisionUXModel {
		t.Fatalf("initial vision: model = %q, want %q", m1, ollamaVisionUXModel)
	}

	// Reassign Vision → codellama:7b (e.g. user decides to use stronger model).
	if err := stageConfig.SetGlobalStageDefault(model.StageVision, StageAssignment{
		ConnectionID: connID, Model: ollamaCodeModel,
	}); err != nil {
		t.Fatalf("reassign vision: %v", err)
	}

	// After: Vision → codellama:7b without restart.
	_, m2, err := registry.ResolveForStage("proj-dyn", model.StageVision, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage after reassign: %v", err)
	}
	if m2 != ollamaCodeModel {
		t.Errorf("after reassign: vision model = %q, want %q", m2, ollamaCodeModel)
	}

	// UX is unchanged — still uses gemma3:4b.
	_, m3, err := registry.ResolveForStage("proj-dyn", model.StageUX, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(ux) after reassign: %v", err)
	}
	if m3 != ollamaVisionUXModel {
		t.Errorf("ux after vision reassign: model = %q, want %q", m3, ollamaVisionUXModel)
	}
}

// TestOllamaDualModel_IndependentProjects verifies that two projects can have
// different per-project model assignments using the same shared Ollama connection.
func TestOllamaDualModel_IndependentProjects(t *testing.T) {
	srv, _ := ollamaDualModelMockServer(t)

	globalDir := t.TempDir()
	projectADir := t.TempDir()
	projectBDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	conn, _ := connStore.Create(Connection{
		Name:         "Shared Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      srv.URL,
		DefaultModel: ollamaVisionUXModel,
	})

	stageConfig := NewStageConfigStore(globalDir)
	// Global default: Architecture → codellama:7b.
	_ = stageConfig.SetGlobalStageDefault(model.StageArchitecture, StageAssignment{
		ConnectionID: conn.ID, Model: ollamaCodeModel,
	})

	// Project B overrides Architecture → gemma3:4b (cost-conscious team).
	_ = stageConfig.SetProjectStageOverride(projectBDir, model.StageArchitecture,
		&StageAssignment{ConnectionID: conn.ID, Model: ollamaVisionUXModel},
	)

	registry := NewRegistry(connStore)

	// Project A: uses global default → codellama:7b.
	_, mA, err := registry.ResolveForStage("proj-a", model.StageArchitecture, stageConfig, projectADir)
	if err != nil {
		t.Fatalf("project A ResolveForStage: %v", err)
	}
	if mA != ollamaCodeModel {
		t.Errorf("project A: architecture model = %q, want %q", mA, ollamaCodeModel)
	}

	// Project B: uses per-project override → gemma3:4b.
	_, mB, err := registry.ResolveForStage("proj-b", model.StageArchitecture, stageConfig, projectBDir)
	if err != nil {
		t.Fatalf("project B ResolveForStage: %v", err)
	}
	if mB != ollamaVisionUXModel {
		t.Errorf("project B: architecture model = %q, want %q", mB, ollamaVisionUXModel)
	}
}

