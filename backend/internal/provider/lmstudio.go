package provider

// lmstudio.go implements the Provider interface for LM Studio.
//
// LM Studio exposes an OpenAI-compatible REST API on a configurable local
// port (default http://localhost:1234). Because the API surface is identical
// to OpenAI's /v1/ endpoints, this provider is a thin adapter that delegates
// every method to the shared openai_compat helpers with an empty auth header
// map — LM Studio requires no authentication.
//
// Supported operations:
//   - Chat:           POST <baseURL>/v1/chat/completions  (streaming SSE)
//   - TestConnection: GET  <baseURL>/v1/models            (used as health check)
//   - ListModels:     GET  <baseURL>/v1/models            (returns loaded models)
//
// The base URL is required; no default is assumed because users run LM Studio
// on various ports and the correct value depends on their local setup.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// LMStudioProvider implements Provider using the LM Studio HTTP API.
// LM Studio exposes an OpenAI-compatible API, so all three interface methods
// delegate to the shared openAICompat* helpers with no authentication headers.
type LMStudioProvider struct {
	baseURL    string
	httpClient *http.Client
}

// NewLMStudioProvider returns an LMStudioProvider configured to talk to the
// LM Studio instance at baseURL (e.g. "http://localhost:1234"). Trailing
// slashes are stripped for consistency.
//
// The caller is responsible for ensuring baseURL is non-empty; the Registry
// guards against empty base URLs before calling this constructor.
//
// A single http.Client is allocated at construction time with a 10-minute
// timeout suited to long-running chat streams. TestConnection and ListModels
// apply their own short deadlines via context timeouts inside the shared
// openAICompat* helpers.
func NewLMStudioProvider(baseURL string) *LMStudioProvider {
	baseURL = strings.TrimRight(baseURL, "/")
	return &LMStudioProvider{
		baseURL: baseURL,
		httpClient: &http.Client{
			// Streaming chat can run for many minutes; TestConnection and
			// ListModels apply their own short deadlines via context.
			Timeout: 10 * time.Minute,
		},
	}
}

// Chat sends the conversation to LM Studio's POST /v1/chat/completions
// endpoint with stream:true and returns a channel of StreamEvent values.
//
// The implementation delegates entirely to openAICompatChat with a nil auth
// header map (LM Studio requires no authentication). The caller-provided ctx
// controls cancellation; p.httpClient applies a 10-minute transport-level
// timeout for long-running streams.
func (p *LMStudioProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("lmstudio: model must not be empty")
	}

	ch, err := openAICompatChat(ctx, p.httpClient, p.baseURL, nil, req)
	if err != nil {
		return nil, fmt.Errorf("lmstudio: %w", err)
	}
	return ch, nil
}

// TestConnection verifies that the LM Studio instance is reachable by calling
// GET /v1/models. Returns nil on success. A 30-second deadline is enforced
// internally by the openAICompatTestConnection helper.
func (p *LMStudioProvider) TestConnection(ctx context.Context) error {
	if err := openAICompatTestConnection(ctx, p.baseURL, nil); err != nil {
		return fmt.Errorf("lmstudio: %w", err)
	}
	return nil
}

// ListModels returns the models currently loaded in LM Studio by calling
// GET /v1/models. A 30-second deadline is enforced internally by the
// openAICompatListModels helper. Returns an empty slice (not an error) if LM
// Studio reports no loaded models.
func (p *LMStudioProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := openAICompatListModels(ctx, p.baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("lmstudio: %w", err)
	}
	return models, nil
}

// ExecuteAgent runs an agentic tool-use loop via the LM Studio Chat Completions API.
func (p *LMStudioProvider) ExecuteAgent(ctx context.Context, req AgentRequest) (<-chan StreamEvent, error) {
	ch, err := openAICompatExecuteAgent(ctx, p.httpClient, p.baseURL, nil, req)
	if err != nil {
		return nil, fmt.Errorf("lmstudio: %w", err)
	}
	return ch, nil
}
