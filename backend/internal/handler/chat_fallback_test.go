// Package handler — ChatHandler Claude CLI fallback regression tests.
//
// These tests verify that NewChatHandler accepts nil providerRegistry,
// stageConfig, and connStore without panicking, and that the internal
// resolveProvider method returns a *provider.ClaudeCLIProvider with the
// correct v0.1.0 hardcoded models when no registry is configured.
package handler

import (
	"testing"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// ---------------------------------------------------------------------------
// Test doubles — minimal no-op implementations of repository interfaces
// ---------------------------------------------------------------------------

// nopChatRepo satisfies repository.ChatRepo with no-op implementations.
type nopChatRepo struct{}

func (r *nopChatRepo) GetHistory(hostDir string, stage model.StageName) ([]model.Message, error) {
	return nil, nil
}
func (r *nopChatRepo) AppendMessage(hostDir string, stage model.StageName, msg model.Message) error {
	return nil
}

// nopArtifactRepo satisfies repository.ArtifactRepo with no-op implementations.
type nopArtifactRepo struct{}

func (r *nopArtifactRepo) Read(hostDir string, stage model.StageName) (string, error) {
	return "", nil
}
func (r *nopArtifactRepo) ReadWithFallback(dataDir, hostDir, version string, stage model.StageName) (string, error) {
	return "", nil
}
func (r *nopArtifactRepo) Write(hostDir string, stage model.StageName, content string) error {
	return nil
}
func (r *nopArtifactRepo) Exists(hostDir string, stage model.StageName) (bool, error) {
	return false, nil
}

// nopActivityRepo satisfies repository.ActivityRepo with no-op implementations.
type nopActivityRepo struct{}

func (r *nopActivityRepo) ReadActivity(hostDir string) (map[model.StageName]*model.StageActivity, error) {
	return nil, nil
}
func (r *nopActivityRepo) SetActivity(hostDir string, stage model.StageName, a *model.StageActivity) error {
	return nil
}
func (r *nopActivityRepo) ClearActivity(hostDir string, stage model.StageName) error {
	return nil
}
func (r *nopActivityRepo) AppendBtw(hostDir string, stage model.StageName, message string, sentAt time.Time) error {
	return nil
}
func (r *nopActivityRepo) ClearBtw(hostDir string, stage model.StageName) ([]model.BtwMessage, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestChatHandler_NilRegistry_ResolveProviderFallback verifies that when the
// ChatHandler is created with nil providerRegistry (as in v0.1.0 and as a
// graceful degradation path), resolveProvider returns a *ClaudeCLIProvider
// with the correct hardcoded model for each pipeline stage.
//
// This test directly exercises the private resolveProvider method to ensure
// the fallback path is intact without spinning up an HTTP server.
func TestChatHandler_NilRegistry_ResolveProviderFallback(t *testing.T) {
	runs := stream.NewManager()
	h := NewChatHandler(
		&mockRegistryRepo{projects: nil}, // registry — not used by resolveProvider
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		nil, // providerRegistry — nil triggers v0.1.0 fallback path
		nil, // stageConfig
		nil, // connStore
		nil, // ragClient
		"",  // logBase empty in tests
	)

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
			prov, modelID, _, err := h.resolveProvider("proj-1", "/tmp/testproject", tc.stage)

			if err != nil {
				t.Fatalf("resolveProvider(%s) with nil registry: unexpected error: %v", tc.stage, err)
			}
			if prov == nil {
				t.Fatal("resolveProvider returned nil provider — pipeline would crash")
			}
			if _, ok := prov.(*provider.ClaudeCLIProvider); !ok {
				t.Errorf("resolveProvider(%s) returned %T, want *provider.ClaudeCLIProvider", tc.stage, prov)
			}
			if modelID != tc.wantModel {
				t.Errorf("resolveProvider(%s) model = %q, want %q", tc.stage, modelID, tc.wantModel)
			}
		})
	}
}

// TestChatHandler_WithRegistryNoConfig_ResolveProviderFallback verifies that
// when the ChatHandler has a real Registry but no config files on disk (the
// common upgrade scenario for v0.1.0 users), resolveProvider still returns
// a *ClaudeCLIProvider with the correct model for each stage.
func TestChatHandler_WithRegistryNoConfig_ResolveProviderFallback(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := provider.NewConnectionStore(globalDir)
	provRegistry := provider.NewRegistry(connStore)
	stageConfig := provider.NewStageConfigStore(globalDir)

	runs := stream.NewManager()
	h := NewChatHandler(
		&mockRegistryRepo{projects: nil},
		&nopChatRepo{},
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		provRegistry,
		stageConfig,
		connStore,
		nil, // ragClient
		"",  // logBase empty in tests
	)

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
			prov, modelID, _, err := h.resolveProvider("proj-1", projectDir, tc.stage)

			if err != nil {
				t.Fatalf("resolveProvider(%s) with empty registry: unexpected error: %v", tc.stage, err)
			}
			if _, ok := prov.(*provider.ClaudeCLIProvider); !ok {
				t.Errorf("resolveProvider(%s) returned %T, want *provider.ClaudeCLIProvider", tc.stage, prov)
			}
			if modelID != tc.wantModel {
				t.Errorf("resolveProvider(%s) model = %q, want %q", tc.stage, modelID, tc.wantModel)
			}
		})
	}
}
