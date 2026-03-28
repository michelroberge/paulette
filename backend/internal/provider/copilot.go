package provider

import (
	"context"
	"fmt"
	"net/http"
)

const copilotBaseURL = "https://api.githubcopilot.com"

// GitHubCopilotProvider implements Provider using the GitHub Copilot API.
// The Copilot API is OpenAI-compatible, so all operations delegate to the
// shared openAICompat* helpers with Copilot-specific auth headers.
type GitHubCopilotProvider struct {
	token      string
	httpClient *http.Client
}

// NewGitHubCopilotProvider returns a GitHubCopilotProvider using the given
// OAuth token (obtained via the device-code flow or a personal access token).
func NewGitHubCopilotProvider(token string) *GitHubCopilotProvider {
	return &GitHubCopilotProvider{
		token:      token,
		httpClient: newOpenAICompatChatClient(),
	}
}

// copilotHeaders builds the set of HTTP headers required by the Copilot API.
func (p *GitHubCopilotProvider) copilotHeaders() map[string]string {
	return map[string]string{
		"Authorization":          "Bearer " + p.token,
		"Editor-Version":         "Neovim/0.9.0",
		"Copilot-Integration-Id": "neovim.copilot",
		"X-GitHub-Api-Version":   "2023-01-01",
	}
}

// Chat sends the conversation to the GitHub Copilot chat completions endpoint
// and streams events back. Delegates to openAICompatChat.
func (p *GitHubCopilotProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	ch, err := openAICompatChat(ctx, p.httpClient, copilotBaseURL, p.copilotHeaders(), req)
	if err != nil {
		return nil, fmt.Errorf("copilot: %w", err)
	}
	return ch, nil
}

// ExecuteAgent runs an agentic tool-use loop via the Copilot API.
func (p *GitHubCopilotProvider) ExecuteAgent(ctx context.Context, req AgentRequest) (<-chan StreamEvent, error) {
	ch, err := openAICompatExecuteAgent(ctx, p.httpClient, copilotBaseURL, p.copilotHeaders(), req)
	if err != nil {
		return nil, fmt.Errorf("copilot: %w", err)
	}
	return ch, nil
}

// TestConnection verifies the Copilot token is valid by calling GET /v1/models.
func (p *GitHubCopilotProvider) TestConnection(ctx context.Context) error {
	if err := openAICompatTestConnection(ctx, copilotBaseURL, p.copilotHeaders()); err != nil {
		return fmt.Errorf("copilot: %w", err)
	}
	return nil
}

// ListModels returns models available via the Copilot API.
func (p *GitHubCopilotProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := openAICompatListModels(ctx, copilotBaseURL, p.copilotHeaders())
	if err != nil {
		return nil, fmt.Errorf("copilot: %w", err)
	}
	return models, nil
}
