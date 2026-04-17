// Package provider implements the pluggable LLM provider abstraction layer for
// Paulette v0.2.0. It defines the Provider interface that all LLM backends must
// implement, along with a Registry that resolves the correct provider and model
// for each pipeline stage using a three-level fallback hierarchy.
package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// ErrModelListUnsupported is returned by ListModels when the provider
// does not expose a model enumeration endpoint.
var ErrModelListUnsupported = errors.New("model listing not supported by this provider")

// StreamEvent is re-exported from the model package so callers can use
// provider.StreamEvent without importing model directly.
// Event types: "chunk", "artifact", "done", "error", "tokens", "plan_limit", "log".
type StreamEvent = model.StreamEvent

// AgentTool describes a tool the LLM may call during agentic execution.
type AgentTool struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema object
}

// AgentRequest is the payload for tool-use (agentic) LLM calls.
// Unlike ChatRequest, it has no History — agentic bead execution is single-turn.
type AgentRequest struct {
	Model        string
	SystemPrompt string
	UserMessage  string
	ProjectDir   string
	Tools        []AgentTool
}

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
	// conversation. The XML envelope format (<!-- RESPONSE:START --><artifact>…</artifact><!-- RESPONSE:END -->)
	// must be preserved across all providers so the backend parser continues to work.
	SystemPrompt string

	// History contains prior conversation turns in chronological order.
	History []Message

	// UserMessage is the new user turn to send.
	UserMessage string

	// ProjectDir is the working directory context. Required by the Claude CLI
	// provider; ignored by HTTP-based providers.
	ProjectDir string

	// Stage is an optional label used for debug logging (e.g. "architecture", "ux").
	Stage string

	// Temperature overrides the provider's default/heuristic temperature.
	// When nil, the provider uses its own logic (e.g. Ollama's stage-based default).
	Temperature *float64

	// NumCtx overrides the context window size sent to the provider.
	// When nil, the provider uses its own default (e.g. Ollama's model capability table).
	NumCtx *int

	// Stream controls whether the response is streamed.
	// When nil, the provider defaults to streaming where supported.
	Stream *bool
}

// TempLow returns a pointer to 0.3, suitable for structured output prompts.
func TempLow() *float64 { v := 0.3; return &v }

// Provider is the interface every LLM backend must implement.
// Implementations convert their native streaming format (NDJSON, SSE, etc.)
// into a channel of StreamEvent so the rest of the pipeline remains unchanged.
type Provider interface {
	// Chat sends a conversation to the LLM and streams events back on the
	// returned channel. The channel is closed after a "done" or "error" event.
	// The caller must drain the channel; failing to do so will leak the goroutine.
	Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)

	// ExecuteAgent runs an agentic tool-use loop and streams events back on the
	// returned channel. The channel is closed after a "done" or "error" event.
	ExecuteAgent(ctx context.Context, req AgentRequest) (<-chan StreamEvent, error)

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

// FallbackModel returns the hardcoded default Claude model for a stage.
// It is exported so callers (e.g. handler/chat.go) can use it when no
// Registry is available.
func FallbackModel(stage model.StageName) string {
	if m, ok := stageModels[stage]; ok {
		return m
	}
	return "claude-sonnet-4-6"
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

// ForUnsavedConnection instantiates a Provider for the given Connection record
// without requiring it to be persisted in the ConnectionStore first. This is
// used by the Test and ListModels endpoints to probe an unsaved connection
// configuration before the user commits it.
func (r *Registry) ForUnsavedConnection(conn *Connection) (Provider, error) {
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
		if conn.APIKey == "" {
			return nil, fmt.Errorf("github copilot: no OAuth token configured; complete device auth first")
		}
		return NewGitHubCopilotProvider(conn.APIKey), nil

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
	prov, model, _, err := r.ResolveForStageWithSettings(projectID, stage, stageConfig, hostDir)
	return prov, model, err
}

// ResolveForStageWithSettings is identical to ResolveForStage but additionally
// returns the full *StageAssignment (which includes optional inference settings
// such as Temperature, NumCtx, and Stream). The assignment is nil when no
// configuration exists (level-3 Claude CLI fallback).
func (r *Registry) ResolveForStageWithSettings(
	projectID string,
	stage model.StageName,
	stageConfig *StageConfigStore,
	hostDir string,
) (Provider, string, *StageAssignment, error) {
	if stageConfig != nil {
		// Levels 1 & 2: single read-lock acquisition via ResolveStage.
		// This avoids the double lock-release cycle that would occur if
		// GetProjectOverrides and GetGlobalDefaults were called sequentially.
		if assignment := stageConfig.ResolveStage(hostDir, stage); assignment != nil {
			p, err := r.ForConnection(assignment.ConnectionID)
			if err != nil {
				return nil, "", nil, fmt.Errorf("stage %q: configured connection error: %w", stage, err)
			}
			return p, assignment.Model, assignment, nil
		}
	}

	// Level 3: hardcoded Claude CLI fallback — identical to v0.1.0 behaviour.
	claudeModel, ok := stageModels[stage]
	if !ok {
		claudeModel = "claude-sonnet-4-6"
	}
	return NewClaudeCLIProvider(), claudeModel, nil, nil
}

// ResolveForStageOperation resolves the provider and model for a specific
// sub-step operation (e.g. "ux.mock", "build.generate") using a five-level
// fallback hierarchy:
//
//  1. Per-project operation override
//  2. Global operation default
//  3. Per-project stage override
//  4. Global stage default
//  5. Hardcoded Claude CLI fallback
//
// Falls back to ResolveForStageWithSettings when stageConfig is nil or when
// operation is empty.
func (r *Registry) ResolveForStageOperation(
	projectID string,
	stage model.StageName,
	operation OperationKey,
	stageConfig *StageConfigStore,
	hostDir string,
) (Provider, string, *StageAssignment, error) {
	if stageConfig != nil && operation != "" {
		if assignment := stageConfig.ResolveOperation(hostDir, stage, operation); assignment != nil {
			p, err := r.ForConnection(assignment.ConnectionID)
			if err != nil {
				return nil, "", nil, fmt.Errorf("operation %q: configured connection error: %w", operation, err)
			}
			return p, assignment.Model, assignment, nil
		}
	}

	// No operation-level config — fall through to stage-level resolution.
	return r.ResolveForStageWithSettings(projectID, stage, stageConfig, hostDir)
}
