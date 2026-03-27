// Package provider — connection validation unit tests.
//
// These tests exercise validateConnectionInput to ensure all edge cases for
// connection creation/update are properly rejected before touching the
// filesystem. The validation gate is the first line of defence against
// malformed connections reaching the store.
package provider

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Name validation
// ---------------------------------------------------------------------------

func TestValidateConnectionInput_EmptyName_ReturnsError(t *testing.T) {
	conn := ollamaConn()
	conn.Name = ""
	if err := validateConnectionInput(&conn); err == nil {
		t.Error("want error for empty name")
	}
}

func TestValidateConnectionInput_WhitespaceName_ReturnsError(t *testing.T) {
	conn := ollamaConn()
	conn.Name = "   \t\n"
	err := validateConnectionInput(&conn)
	if err == nil {
		t.Error("want error for whitespace-only name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention 'name', got: %v", err)
	}
}

func TestValidateConnectionInput_ValidName_NoError(t *testing.T) {
	conn := ollamaConn()
	conn.Name = "My Local Ollama"
	if err := validateConnectionInput(&conn); err != nil {
		t.Errorf("want nil for valid name, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ProviderType validation
// ---------------------------------------------------------------------------

func TestValidateConnectionInput_UnknownProviderType_ReturnsError(t *testing.T) {
	conn := ollamaConn()
	conn.ProviderType = ProviderType("totally-made-up")
	err := validateConnectionInput(&conn)
	if err == nil {
		t.Error("want error for unknown provider type")
	}
	if !strings.Contains(err.Error(), "unknown provider type") {
		t.Errorf("error should mention 'unknown provider type', got: %v", err)
	}
}

func TestValidateConnectionInput_AllValidProviderTypes_NoError(t *testing.T) {
	// All seven declared provider types must pass validation.
	types := []struct {
		pt      ProviderType
		needURL bool
		needKey bool
	}{
		{ProviderOllama, true, false},
		{ProviderLMStudio, true, false},
		{ProviderAnthropic, false, true},
		{ProviderOpenAI, false, true},
		{ProviderGemini, false, true},
		{ProviderClaudeCLI, false, false},
		{ProviderGitHubCopilot, false, false},
	}

	for _, tc := range types {
		t.Run(string(tc.pt), func(t *testing.T) {
			conn := Connection{
				Name:         "test-" + string(tc.pt),
				ProviderType: tc.pt,
				DefaultModel: "some-model",
			}
			if tc.needURL {
				conn.BaseURL = "http://localhost:1234"
			}
			if tc.needKey {
				conn.APIKey = "test-key"
			}
			if err := validateConnectionInput(&conn); err != nil {
				t.Errorf("want nil for valid %s connection, got: %v", tc.pt, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BaseURL validation (Ollama / LM Studio require it)
// ---------------------------------------------------------------------------

func TestValidateConnectionInput_OllamaMissingBaseURL_ReturnsError(t *testing.T) {
	conn := ollamaConn()
	conn.BaseURL = ""
	err := validateConnectionInput(&conn)
	if err == nil {
		t.Error("want error for Ollama without BaseURL")
	}
	if !strings.Contains(err.Error(), "base URL") {
		t.Errorf("error should mention 'base URL', got: %v", err)
	}
}

func TestValidateConnectionInput_LMStudioMissingBaseURL_ReturnsError(t *testing.T) {
	conn := Connection{
		Name:         "lm-studio-no-url",
		ProviderType: ProviderLMStudio,
		DefaultModel: "mistral",
		// BaseURL intentionally empty
	}
	if err := validateConnectionInput(&conn); err == nil {
		t.Error("want error for LM Studio without BaseURL")
	}
}

func TestValidateConnectionInput_OpenAI_NoBaseURLRequired(t *testing.T) {
	// OpenAI uses a default base URL if none is provided.
	conn := Connection{
		Name:         "openai-no-url",
		ProviderType: ProviderOpenAI,
		APIKey:       "sk-test",
		DefaultModel: "gpt-4o",
		// BaseURL intentionally empty — should use default
	}
	if err := validateConnectionInput(&conn); err != nil {
		t.Errorf("want nil for OpenAI without explicit BaseURL, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DefaultModel validation
// ---------------------------------------------------------------------------

func TestValidateConnectionInput_EmptyDefaultModel_NonCLI_ReturnsError(t *testing.T) {
	// All providers except claude_cli require a default model.
	providers := []ProviderType{
		ProviderOllama, ProviderLMStudio, ProviderAnthropic,
		ProviderOpenAI, ProviderGemini,
	}
	for _, pt := range providers {
		t.Run(string(pt), func(t *testing.T) {
			conn := Connection{
				Name:         "test",
				ProviderType: pt,
				DefaultModel: "",
			}
			if pt == ProviderOllama || pt == ProviderLMStudio {
				conn.BaseURL = "http://localhost:1234"
			} else {
				conn.APIKey = "test-key"
			}
			if err := validateConnectionInput(&conn); err == nil {
				t.Errorf("want error for %s with empty DefaultModel", pt)
			}
		})
	}
}

func TestValidateConnectionInput_EmptyDefaultModel_ClaudeCLI_OK(t *testing.T) {
	// claude_cli is exempt from the DefaultModel requirement because it uses
	// the hardcoded stage-model map.
	conn := Connection{
		Name:         "cli-no-model",
		ProviderType: ProviderClaudeCLI,
		DefaultModel: "",
	}
	if err := validateConnectionInput(&conn); err != nil {
		t.Errorf("want nil for ClaudeCLI with empty DefaultModel, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Full valid connections pass without error
// ---------------------------------------------------------------------------

func TestValidateConnectionInput_FullOllamaConnection_NoError(t *testing.T) {
	conn := ollamaConn()
	if err := validateConnectionInput(&conn); err != nil {
		t.Errorf("want nil for complete Ollama connection, got: %v", err)
	}
}

func TestValidateConnectionInput_FullOpenAIConnection_NoError(t *testing.T) {
	conn := apiKeyConn()
	if err := validateConnectionInput(&conn); err != nil {
		t.Errorf("want nil for complete OpenAI connection, got: %v", err)
	}
}

func TestValidateConnectionInput_FullAnthropicConnection_NoError(t *testing.T) {
	conn := Connection{
		Name:         "anthropic-test",
		ProviderType: ProviderAnthropic,
		APIKey:       "anthropic-key",
		DefaultModel: "claude-opus-4-6",
	}
	if err := validateConnectionInput(&conn); err != nil {
		t.Errorf("want nil for complete Anthropic connection, got: %v", err)
	}
}

func TestValidateConnectionInput_FullGeminiConnection_NoError(t *testing.T) {
	conn := Connection{
		Name:         "gemini-test",
		ProviderType: ProviderGemini,
		APIKey:       "gemini-key",
		DefaultModel: "gemini-1.5-pro",
	}
	if err := validateConnectionInput(&conn); err != nil {
		t.Errorf("want nil for complete Gemini connection, got: %v", err)
	}
}
