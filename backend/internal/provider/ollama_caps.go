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
	// instruction (wrap output in <!-- RESPONSE:START --><discussion>...</discussion>
	// <artifact>...</artifact><!-- RESPONSE:END -->). Models that are not XML-friendly
	// receive a simpler "## Discussion / ## Artifact" section-header format.
	XMLFriendly bool

	// NumCtx is the context window size (in tokens) to request from Ollama.
	// Ollama defaults to 2048 which is far too small for later pipeline stages
	// that inject prior artifacts. Zero means use the default (8192).
	NumCtx int
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
	{prefix: "llama3.1", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "llama3.2", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "llama3.3", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},

	// ── Mistral family ───────────────────────────────────────────────────────
	// More-specific prefixes first.
	{prefix: "mistral-nemo", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "mistral-small", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 32768}},
	{prefix: "mistral-large", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "mixtral", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 32768}},
	{prefix: "mistral", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 32768}},

	// ── Qwen family ──────────────────────────────────────────────────────────
	{prefix: "qwen2.5-coder", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 32768}},
	{prefix: "qwen2.5", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 32768}},
	{prefix: "qwen3", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 32768}},

	// ── Gemma family ─────────────────────────────────────────────────────────
	// gemma3 supports tools; gemma2 and plain "gemma" do not.
	{prefix: "gemma3", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "gemma2", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 8192}},
	{prefix: "gemma", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 8192}},

	// ── Phi family ───────────────────────────────────────────────────────────
	// phi4 and phi3.5 support tools; plain phi3 does not.
	{prefix: "phi4", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 16384}},
	{prefix: "phi3.5", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "phi3", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 4096}},

	// ── DeepSeek family ──────────────────────────────────────────────────────
	// deepseek-coder-v2 supports tools; deepseek-r1 and plain "deepseek" do not.
	{prefix: "deepseek-coder-v2", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "deepseek-r1", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 131072}},
	{prefix: "deepseek", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 32768}},

	// ── Command R family ─────────────────────────────────────────────────────
	{prefix: "command-r-plus", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},
	{prefix: "command-r", caps: OllamaModelCaps{NativeTools: true, XMLFriendly: true, NumCtx: 131072}},

	// ── Code-focused models without tool support ──────────────────────────────
	{prefix: "codellama", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 16384}},
	{prefix: "codegemma", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 8192}},

	// ── Community models ──────────────────────────────────────────────────────
	{prefix: "yi", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 16384}},
	{prefix: "internlm", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 32768}},
	{prefix: "nous-hermes", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 8192}},
	{prefix: "neural-chat", caps: OllamaModelCaps{NativeTools: false, XMLFriendly: false, NumCtx: 8192}},

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

// defaultNumCtx is the context window size used when a model has no specific
// entry in the caps table or its entry has NumCtx == 0. 8192 is a safe default
// that works on most hardware while being large enough for single-stage prompts.
const defaultNumCtx = 8192

// safeDefault is returned for any model name not matched in the table.
// Choosing false/false is the safe option: the pseudo-tool loop works for all
// models, while sending a "tools" array to a model that ignores it can produce
// empty or malformed responses.
var safeDefault = OllamaModelCaps{NativeTools: false, XMLFriendly: false}

// EffectiveNumCtx returns the context window size to request, falling back to
// defaultNumCtx when the model entry has no explicit value.
func (c OllamaModelCaps) EffectiveNumCtx() int {
	if c.NumCtx > 0 {
		return c.NumCtx
	}
	return defaultNumCtx
}

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
