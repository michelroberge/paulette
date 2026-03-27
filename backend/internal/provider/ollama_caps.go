package provider

import "strings"

// OllamaModelCaps describes runtime capabilities for a specific Ollama model.
// Capabilities are inferred from the model family name and are used to select
// between native tool calling and the pseudo-tool text fallback.
type OllamaModelCaps struct {
	// NativeTools indicates the model supports the "tools" field in /api/chat
	// and will emit structured tool_calls in its response.
	NativeTools bool

	// XMLFriendly indicates the model reliably follows the XML envelope
	// instruction (wrap output in <response><discussion>...</discussion>
	// <artifact>...</artifact></response>). Models that are not XML-friendly
	// receive a simpler "## Discussion / ## Artifact" section-header format.
	XMLFriendly bool
}

// capEntry maps a model family prefix to its known capabilities.
// Entries are ordered from most-specific to least-specific so that the first
// strings.HasPrefix match wins (e.g. "mistral-nemo" before "mistral").
type capEntry struct {
	prefix string
	caps   OllamaModelCaps
}

// ollamaCapTable is the ordered lookup table for model capabilities.
// Last entry is the catch-all for unknown models (safe defaults: no tools, no XML).
var ollamaCapTable = []capEntry{
	// ── Llama 3.x ────────────────────────────────────────────────────────────
	{prefix: "llama3.1", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "llama3.2", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "llama3.3", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},

	// ── Mistral family ───────────────────────────────────────────────────────
	// More-specific prefixes first.
	{prefix: "mistral-nemo", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "mistral-small", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "mistral-large", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "mistral", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},

	// ── Qwen family ──────────────────────────────────────────────────────────
	{prefix: "qwen2.5-coder", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "qwen2.5", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "qwen3", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},

	// ── Gemma family ─────────────────────────────────────────────────────────
	// gemma3 supports tools; gemma2 and plain "gemma" do not.
	{prefix: "gemma3", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "gemma2", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "gemma", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},

	// ── Phi family ───────────────────────────────────────────────────────────
	// phi4 and phi3.5 support tools; plain phi3 does not.
	{prefix: "phi4", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "phi3.5", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "phi3", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},

	// ── DeepSeek family ──────────────────────────────────────────────────────
	// deepseek-coder-v2 supports tools; deepseek-r1 and plain "deepseek" do not.
	{prefix: "deepseek-coder-v2", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true}},
	{prefix: "deepseek-r1", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "deepseek", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},

	// ── Code-focused models without tool support ──────────────────────────────
	{prefix: "codellama", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "codegemma", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},

	// ── Older/community models without tool support ───────────────────────────
	{prefix: "vicuna", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "orca", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "solar", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "falcon", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "wizard", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "openchat", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "starling", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
	{prefix: "zephyr", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false}},
}

// safeDefault is returned for any model name not matched in the table.
// Choosing false/false is the safe option: the pseudo-tool loop works for all
// models, while sending a "tools" array to a model that ignores it can produce
// empty or malformed responses.
var safeDefault = OllamaModelCaps{NativeTools: false, XMLFriendly: false}

// ollamaModelCaps returns the capability profile for the given Ollama model name.
// The name may include a size/variant tag (e.g. "llama3.2:3b", "qwen2.5-coder:7b-instruct").
// Matching is case-insensitive and strips the tag suffix before lookup.
func ollamaModelCaps(modelName string) OllamaModelCaps {
	// Normalise: lowercase and strip tag (everything from the first ':' onward).
	name := strings.ToLower(modelName)
	if idx := strings.IndexByte(name, ':'); idx >= 0 {
		name = name[:idx]
	}

	for _, e := range ollamaCapTable {
		if strings.HasPrefix(name, e.prefix) {
			return e.caps
		}
	}
	return safeDefault
}
