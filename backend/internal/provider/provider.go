// Package provider implements the pluggable LLM provider abstraction layer for
// Paulette v0.2.0. It defines the Provider interface that all LLM backends must
// implement, along with a Registry that resolves the correct provider and model
// for each pipeline stage using a three-level fallback hierarchy.
package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
)

// ErrModelListUnsupported is returned by ListModels when the provider
// does not expose a model enumeration endpoint.
var ErrModelListUnsupported = errors.New("model listing not supported by this provider")

// StreamEvent is re-exported from the agent package so callers can use
// provider.StreamEvent without importing agent directly.
// Event types: "chunk", "artifact", "done", "error", "tokens".
type StreamEvent = agent.StreamEvent

// ModelInfo describes a single model available on a provider.
type ModelInfo struct {
	ID   string `json:"id"`   // e.g. "llama3:8b", "gpt-4o"
	Name string `json:"name"` // human-readable label (may equal ID)
}

// Message aliases model.Message so callers of the provider package do not
// need to import the model package directly.
type Message = model.Message

// ChatRequest contains all information a Provider needs to execute a chat turn
// and stream the response.
type ChatRequest struct {
	// Model is the provider-specific model identifier (e.g. "llama3:8b", "gpt-4o",
	// "claude-sonnet-4-6"). Required.
	Model string

	// SystemPrompt is the stage-specific instruction prompt prepended to every
	// conversation. The XML envelope format (<response><artifact>…</artifact></response>)
	// must be preserved across all providers so the backend parser continues to work.
	SystemPrompt string

	// History contains prior conversation turns in chronological order.
	History []Message

	// UserMessage is the new user turn to send.
	UserMessage string

	// ProjectDir is the working directory context. Required by the Claude CLI
	// provider; ignored by HTTP-based providers.
	ProjectDir string
}

// Provider is the interface every LLM backend must implement.
// Implementations convert their native streaming format (NDJSON, SSE, etc.)
// into a channel of StreamEvent so the rest of the pipeline remains unchanged.
type Provider interface {
	// Chat sends a conversation to the LLM and streams events back on the
	// returned channel. The channel is closed after a "done" or "error" event.
	// The caller must drain the channel; failing to do so will leak the goroutine.
	Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)

	// TestConnection sends a minimal probe request and returns nil on success.
	// Implementations should use a 30-second timeout.
	TestConnection(ctx context.Context) error

	// ListModels returns the models available on this connection.
	// Returns ErrModelListUnsupported if the provider has no model-list endpoint.
	// On success, providers should also honour the opportunistic-discovery contract:
	// callers treat the result as advisory and never block on it.
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// stageModels is the hardcoded fallback model assignment used when no
// stage configuration exists. This preserves v0.1.0 behaviour exactly:
// lighter Sonnet for early stages, more capable Opus for Architecture/Build.
var stageModels = map[model.StageName]string{
	model.StageVision:       "claude-sonnet-4-6",
	model.StageUX:           "claude-sonnet-4-6",
	model.StageArchitecture: "claude-opus-4-6",
	model.StageBuild:        "claude-opus-4-6",
	model.StageComplete:     "claude-sonnet-4-6",
}

// Registry maps provider types to factory functions and resolves the
// correct Provider + model for a given project stage.
type Registry struct {
	connStore *ConnectionStore
}

// NewRegistry creates a Registry backed by the given ConnectionStore.
func NewRegistry(cs *ConnectionStore) *Registry {
	return &Registry{connStore: cs}
}

// ForConnection returns a configured Provider instance for the given connection ID.
// It reads the connection record from the ConnectionStore and delegates to
// providerForConnection for type-specific instantiation.
func (r *Registry) ForConnection(connectionID string) (Provider, error) {
	conn, err := r.connStore.Get(connectionID)
	if err != nil {
		return nil, fmt.Errorf("connection %q: %w", connectionID, err)
	}
	return r.providerForConnection(conn)
}

// providerForConnection instantiates the correct Provider implementation for
// the given Connection. Each case is wired as provider implementations are
// added in Milestone 2 (tasks 2.1–2.7).
func (r *Registry) providerForConnection(conn *Connection) (Provider, error) {
	switch conn.ProviderType {
	case ProviderClaudeCLI:
		return NewClaudeCLIProvider(), nil

	case ProviderOllama:
		if conn.BaseURL == "" {
			return nil, fmt.Errorf("ollama provider requires a base URL")
		}
		return NewOllamaProvider(conn.BaseURL), nil

	case ProviderLMStudio:
		if conn.BaseURL == "" {
			return nil, fmt.Errorf("lmstudio provider requires a base URL")
		}
		return NewLMStudioProvider(conn.BaseURL), nil

	case ProviderOpenAI:
		return NewOpenAIProvider(conn.BaseURL, conn.APIKey, conn.OrgID, conn.ProjectID)

	case ProviderAnthropic:
		return NewAnthropicProvider(conn.BaseURL, conn.APIKey), nil

	case ProviderGemini:
		return NewGeminiProvider(conn.BaseURL, conn.APIKey), nil

	case ProviderGitHubCopilot:
		return nil, fmt.Errorf("GitHub Copilot provider is planned for v0.3.0 and is not yet available")

	default:
		return nil, fmt.Errorf("unknown provider type: %q", conn.ProviderType)
	}
}

// ResolveForStage returns the Provider and model string to use for the given
// pipeline stage, applying a three-level resolution order:
//
//  1. Per-project stage overrides stored in <hostDir>/.paulette/stage_config.json
//  2. Global defaults stored in ~/.paulette/config.json
//  3. Hardcoded Claude CLI fallback (preserves v0.1.0 behaviour exactly)
//
// If stageConfig is nil, levels 1 and 2 are skipped and the function always
// returns the Claude CLI fallback, making the upgrade path non-breaking.
func (r *Registry) ResolveForStage(
	projectID string,
	stage model.StageName,
	stageConfig *StageConfigStore,
	hostDir string,
) (Provider, string, error) {
	if stageConfig != nil {
		// Levels 1 & 2: single read-lock acquisition via ResolveStage.
		// This avoids the double lock-release cycle that would occur if
		// GetProjectOverrides and GetGlobalDefaults were called sequentially.
		if assignment := stageConfig.ResolveStage(hostDir, stage); assignment != nil {
			p, err := r.ForConnection(assignment.ConnectionID)
			if err != nil {
				return nil, "", fmt.Errorf("stage %q: configured connection error: %w", stage, err)
			}
			return p, assignment.Model, nil
		}
	}

	// Level 3: hardcoded Claude CLI fallback — identical to v0.1.0 behaviour.
	claudeModel, ok := stageModels[stage]
	if !ok {
		claudeModel = "claude-sonnet-4-6"
	}
	return NewClaudeCLIProvider(), claudeModel, nil
}
