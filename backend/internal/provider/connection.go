package provider

import "time"

// ProviderType identifies which LLM backend a connection uses.
type ProviderType string

const (
	// P1 — Local inference, no credentials required.
	ProviderOllama   ProviderType = "ollama"
	ProviderLMStudio ProviderType = "lmstudio"

	// P2 — Cloud APIs, API key required.
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGemini    ProviderType = "gemini"

	// Existing Claude CLI subprocess — retained for backward compatibility.
	ProviderClaudeCLI ProviderType = "claude_cli"

	// P3 — GitHub OAuth device-code flow (not functional in v0.2.0).
	ProviderGitHubCopilot ProviderType = "github_copilot"
)

// Connection is a named, reusable record describing how to reach an inference backend.
// Persisted to ~/.paulette/connections.json (file mode 0600).
type Connection struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"providerType"`

	// BaseURL is required for Ollama and LM Studio; optional override for cloud providers.
	BaseURL string `json:"baseUrl,omitempty"`

	// APIKey is stored in plain text (file is chmod 600). Never exposed via the API.
	APIKey string `json:"apiKey,omitempty"`

	// OrgID and ProjectID are OpenAI-specific optional identifiers.
	OrgID     string `json:"orgId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`

	// DefaultModel is the model used when no per-stage override is set.
	DefaultModel string `json:"defaultModel"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ConnectionResponse is the API-facing representation of a Connection.
// Credentials are redacted: APIKey is never returned; HasCredentials signals
// whether a key is stored so the frontend can show an indicator.
type ConnectionResponse struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"providerType"`
	BaseURL      string       `json:"baseUrl,omitempty"`

	// HasCredentials is true when an API key is stored for this connection.
	HasCredentials bool `json:"hasCredentials"`

	OrgID        string `json:"orgId,omitempty"`
	ProjectID    string `json:"projectId,omitempty"`
	DefaultModel string `json:"defaultModel"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ToResponse converts a Connection to its API-safe representation, redacting credentials.
func (c *Connection) ToResponse() ConnectionResponse {
	return ConnectionResponse{
		ID:             c.ID,
		Name:           c.Name,
		ProviderType:   c.ProviderType,
		BaseURL:        c.BaseURL,
		HasCredentials: c.APIKey != "",
		OrgID:          c.OrgID,
		ProjectID:      c.ProjectID,
		DefaultModel:   c.DefaultModel,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}
