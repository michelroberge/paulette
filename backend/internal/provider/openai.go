package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// OpenAIProvider implements Provider using the OpenAI Chat Completions API.
// It delegates to the shared openai_compat helpers (openai_compat.go), adding
// the OpenAI-specific authentication and header requirements:
//
//   - Authorization: Bearer {apiKey}
//   - OpenAI-Organization: {orgID}   (optional)
//   - OpenAI-Project: {projectID}    (optional)
//
// The default base URL is https://api.openai.com but can be overridden for
// Azure OpenAI or other OpenAI-compatible deployments.
//
// Streaming format: SSE with "data: {...}" lines — handled by openAICompatChat.
type OpenAIProvider struct {
	baseURL    string
	apiKey     string
	orgID      string // optional — sent as OpenAI-Organization header when non-empty
	projectID  string // optional — sent as OpenAI-Project header when non-empty
	httpClient *http.Client // long-lived client; reused across Chat calls for connection pooling
}

// defaultOpenAIBaseURL is the canonical OpenAI API base URL used when the
// caller does not supply an override.
const defaultOpenAIBaseURL = "https://api.openai.com"

// NewOpenAIProvider returns an OpenAIProvider configured with the given
// parameters. If baseURL is empty, https://api.openai.com is used. Trailing
// slashes are stripped for consistency. apiKey must be non-empty.
//
// orgID and projectID are optional; pass empty strings to omit those headers.
func NewOpenAIProvider(baseURL, apiKey, orgID, projectID string) (*OpenAIProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai: apiKey is required")
	}
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &OpenAIProvider{
		baseURL:    baseURL,
		apiKey:     apiKey,
		orgID:      orgID,
		projectID:  projectID,
		httpClient: newOpenAICompatChatClient(),
	}, nil
}

// authHeaders builds the set of HTTP headers required by the OpenAI API.
// The Authorization header is always set. OpenAI-Organization and
// OpenAI-Project are included only when the respective fields are non-empty.
func (p *OpenAIProvider) authHeaders() map[string]string {
	h := map[string]string{
		"Authorization": "Bearer " + p.apiKey,
	}
	if p.orgID != "" {
		h["OpenAI-Organization"] = p.orgID
	}
	if p.projectID != "" {
		h["OpenAI-Project"] = p.projectID
	}
	return h
}

// Chat sends the conversation to POST /v1/chat/completions (streaming SSE) and
// returns a channel of StreamEvent values. It delegates to the shared
// openAICompatChat function with OpenAI-specific auth headers.
//
// p.httpClient is reused across calls to benefit from TCP keep-alive and
// connection pooling — no new client is allocated per invocation.
//
// The channel is closed after a "done" or final "error" event. The caller must
// drain the channel to avoid leaking the background goroutine.
func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("openai: model must not be empty")
	}

	ch, err := openAICompatChat(
		ctx,
		p.httpClient,
		p.baseURL,
		p.authHeaders(),
		req,
	)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	return ch, nil
}

// TestConnection verifies that the OpenAI API is reachable and that the API
// key is valid by calling GET /v1/models. Returns nil on success.
//
// The 30-second deadline is enforced internally by openAICompatTestConnection;
// no additional timeout is applied here to avoid masking the effective deadline.
func (p *OpenAIProvider) TestConnection(ctx context.Context) error {
	if err := openAICompatTestConnection(ctx, p.baseURL, p.authHeaders()); err != nil {
		return fmt.Errorf("openai: %w", err)
	}
	return nil
}

// ListModels returns the models available on this OpenAI account by calling
// GET /v1/models. The returned ModelInfo slice uses the model ID as both ID
// and Name since the OpenAI /v1/models endpoint does not provide display names.
//
// The 30-second deadline is enforced internally by openAICompatListModels;
// no additional timeout is applied here to avoid masking the effective deadline.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := openAICompatListModels(ctx, p.baseURL, p.authHeaders())
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	return models, nil
}
