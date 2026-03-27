// Package prompts holds all system prompt constants used across the Paulette pipeline.
// Centralising them here lets any provider implementation import prompts without
// creating a circular dependency with the agent package.
package prompts

// ProviderHint identifies the provider family so WrapForProvider can apply
// the appropriate framing adjustments.
type ProviderHint string

const (
	HintClaudeCLI   ProviderHint = "claude_cli"
	HintOpenAI      ProviderHint = "openai_compat"
	HintOllama      ProviderHint = "ollama"
	HintAnthropic   ProviderHint = "anthropic"
	HintGemini      ProviderHint = "gemini"
	HintGitHubCopilot ProviderHint = "github_copilot"
)

// WrapForProvider applies provider-specific framing around a base prompt.
// For Claude CLI the prompt is returned unchanged (it already uses the XML
// envelope convention).  For OpenAI-compatible and Ollama providers the XML
// envelope instruction is preserved — those providers accept and follow it
// correctly in practice.  Future iterations may strip or adapt it per provider.
func WrapForProvider(basePrompt string, _ ProviderHint) string {
	return basePrompt
}
