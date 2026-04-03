// Package handler — connection error event payload tests (Milestone 6.4).
//
// These tests verify that buildProviderErrorEvent produces the structured JSON
// payload that the frontend ConnectionErrorBanner component requires to render
// an actionable error message with a "Go to Configure →" link.
//
// Acceptance criteria (JRN-v0.2.0-008, SCR-012):
//   - isConnectionError: true is always set so the frontend knows to render the
//     banner instead of a generic error message.
//   - connectionId is populated when a stage assignment can be resolved.
//   - connectionName is populated with the human-readable connection label.
//   - reason contains the underlying error message.
//   - The payload is valid JSON (not a Go error string).
//   - When stageConfig or connStore are nil, safe defaults are used (no panic).
package handler

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// ---------------------------------------------------------------------------
// connectionErrorPayload structure verification
// ---------------------------------------------------------------------------

// TestBuildProviderErrorEvent_IsConnectionError verifies that the event always
// has isConnectionError=true so the frontend can distinguish provider failures
// from ordinary AI content errors.
func TestBuildProviderErrorEvent_IsConnectionError(t *testing.T) {
	runs := stream.NewManager()
	h := NewChatHandler(
		&mockRegistryRepo{},
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		nil, nil, nil,
		"", // logBase empty in tests
	)

	event := h.buildProviderErrorEvent("/tmp/testdir", model.StageVision, errors.New("connection refused"))

	if event.Type != "error" {
		t.Errorf("event type = %q, want 'error'", event.Type)
	}

	var payload struct {
		IsConnectionError bool   `json:"isConnectionError"`
		Reason            string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(event.Content), &payload); err != nil {
		t.Fatalf("Content is not valid JSON: %v — content: %s", err, event.Content)
	}
	if !payload.IsConnectionError {
		t.Error("want isConnectionError=true, got false")
	}
	if payload.Reason == "" {
		t.Error("want non-empty reason in error payload")
	}
	if payload.Reason != "connection refused" {
		t.Errorf("reason = %q, want 'connection refused'", payload.Reason)
	}
}

// TestBuildProviderErrorEvent_ValidJSON verifies that the event Content field is
// always parseable JSON, even when stageConfig and connStore are nil (the path
// taken when the v0.1.0 upgrade has no configuration).
func TestBuildProviderErrorEvent_ValidJSON(t *testing.T) {
	runs := stream.NewManager()
	h := NewChatHandler(
		&mockRegistryRepo{},
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		nil, nil, nil, // nil registry — fallback path
		"", // logBase empty in tests
	)

	event := h.buildProviderErrorEvent("/tmp/testdir", model.StageArchitecture,
		errors.New("timeout after 30s"))

	var payload map[string]any
	if err := json.Unmarshal([]byte(event.Content), &payload); err != nil {
		t.Fatalf("buildProviderErrorEvent Content is not valid JSON: %v\ncontent: %s",
			err, event.Content)
	}

	// Verify required keys are present.
	for _, key := range []string{"isConnectionError", "reason"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("missing required key %q in error payload", key)
		}
	}
}

// TestBuildProviderErrorEvent_WithStageConfig verifies that when stageConfig and
// connStore are populated, the event payload includes the connection ID and a
// human-readable connection name, enabling the frontend "Go to Configure →" link.
func TestBuildProviderErrorEvent_WithStageConfig(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := provider.NewConnectionStore(globalDir)
	stageConfig := provider.NewStageConfigStore(globalDir)

	// Create a connection and assign it to the vision stage.
	conn, err := connStore.Create(provider.Connection{
		Name:         "My Ollama",
		ProviderType: provider.ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}
	if err := stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{
		ConnectionID: conn.ID,
		Model:        "llama3:8b",
	}); err != nil {
		t.Fatalf("set stage default: %v", err)
	}

	runs := stream.NewManager()
	provRegistry := provider.NewRegistry(connStore)
	h := NewChatHandler(
		&mockRegistryRepo{},
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		provRegistry,
		stageConfig,
		connStore,
		"", // logBase empty in tests
	)

	event := h.buildProviderErrorEvent(projectDir, model.StageVision,
		errors.New("connection refused at http://localhost:11434"))

	var payload struct {
		IsConnectionError bool   `json:"isConnectionError"`
		ConnectionID      string `json:"connectionId"`
		ConnectionName    string `json:"connectionName"`
		Reason            string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(event.Content), &payload); err != nil {
		t.Fatalf("parse event payload: %v — content: %s", err, event.Content)
	}

	if !payload.IsConnectionError {
		t.Error("want isConnectionError=true")
	}
	if payload.ConnectionID != conn.ID {
		t.Errorf("connectionId = %q, want %q", payload.ConnectionID, conn.ID)
	}
	if payload.ConnectionName != "My Ollama" {
		t.Errorf("connectionName = %q, want 'My Ollama'", payload.ConnectionName)
	}
	if payload.Reason == "" {
		t.Error("reason must not be empty")
	}
}

// TestBuildProviderErrorEvent_NilStageConfig_NoNilPanic verifies that a nil
// stageConfig does not cause a panic — the function must degrade gracefully
// to safe default values.
func TestBuildProviderErrorEvent_NilStageConfig_NoNilPanic(t *testing.T) {
	runs := stream.NewManager()
	h := NewChatHandler(
		&mockRegistryRepo{},
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		nil, // nil providerRegistry
		nil, // nil stageConfig — must not panic
		nil, // nil connStore
		"",  // logBase empty in tests
	)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildProviderErrorEvent panicked with nil stageConfig: %v", r)
		}
	}()

	event := h.buildProviderErrorEvent("/tmp/nonstop", model.StageBuild,
		errors.New("some error"))

	if event.Type != "error" {
		t.Errorf("want type 'error', got %q", event.Type)
	}
	if event.Content == "" {
		t.Error("Content must not be empty even with nil stageConfig")
	}
}

// TestBuildProviderErrorEvent_AllStages verifies that the error event can be
// built for every pipeline stage without panicking or returning empty Content.
func TestBuildProviderErrorEvent_AllStages(t *testing.T) {
	stages := []model.StageName{
		model.StageVision,
		model.StageUX,
		model.StageArchitecture,
		model.StageBuild,
		model.StageComplete,
	}

	runs := stream.NewManager()
	h := NewChatHandler(
		&mockRegistryRepo{},
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		nil, nil, nil,
		"", // logBase empty in tests
	)

	for _, stage := range stages {
		t.Run(string(stage), func(t *testing.T) {
			event := h.buildProviderErrorEvent("/tmp/proj", stage,
				errors.New("provider unreachable"))
			if event.Content == "" {
				t.Errorf("stage %s: Content must not be empty", stage)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(event.Content), &payload); err != nil {
				t.Errorf("stage %s: Content is not valid JSON: %v", stage, err)
			}
		})
	}
}
