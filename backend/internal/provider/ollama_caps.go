package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

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

// ModelNumCtx returns the context window size for the given model name.
// It consults the capability table and falls back to defaultNumCtx for unknown models.
// Callers should prefer a StageAssignment.NumCtx override when set.
func ModelNumCtx(modelName string) int {
	return ollamaModelCaps(modelName).EffectiveNumCtx()
}

// ollamaModelCaps returns the capability profile for the given Ollama model name.
// The name may include a size/variant tag (e.g. "llama3.2:3b", "qwen2.5-coder:7b-instruct").
// Matching is case-insensitive and strips the tag suffix before lookup.
// For unknown models, it checks the runtime probe cache before returning safeDefault.
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

	// Check runtime probe cache for unknown models.
	if caps, ok := probeCache.get(modelName); ok {
		return caps
	}

	return safeDefault
}

// ── Runtime capability probing ───────────────────────────────────────────────

// probeCacheMu protects the in-memory probe result cache.
var probeCache = newCapCache()

type capCache struct {
	mu    sync.RWMutex
	items map[string]OllamaModelCaps
}

func newCapCache() *capCache {
	return &capCache{items: make(map[string]OllamaModelCaps)}
}

func (c *capCache) get(model string) (OllamaModelCaps, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	caps, ok := c.items[model]
	return caps, ok
}

func (c *capCache) set(model string, caps OllamaModelCaps) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[model] = caps
}

// ProbeOllamaModelCaps sends a tiny structured-output probe to the Ollama API
// to determine if the model handles XML envelopes and produces valid JSON.
// The result is cached in memory for subsequent calls.
// This should be called once per unknown model (e.g. on first chat request).
func ProbeOllamaModelCaps(baseURL, model string) OllamaModelCaps {
	// Check cache first.
	if caps, ok := probeCache.get(model); ok {
		return caps
	}

	caps := runProbe(baseURL, model)
	probeCache.set(model, caps)
	return caps
}

// runProbe sends a minimal prompt asking for XML-wrapped JSON and classifies
// the response to determine XMLFriendly and NativeTools capabilities.
func runProbe(baseURL, model string) OllamaModelCaps {
	const probePrompt = `Respond with exactly this XML structure, no other text:
<!-- RESPONSE:START -->
<jsonplan>[{"probe": true}]</jsonplan>
<!-- RESPONSE:END -->`

	body, err := json.Marshal(map[string]any{
		"model":  model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": probePrompt},
		},
		"options": map[string]any{
			"temperature": 0.0,
			"num_predict": 200,
		},
	})
	if err != nil {
		return safeDefault
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return safeDefault
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return safeDefault
	}
	defer resp.Body.Close()

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return safeDefault
	}

	content := result.Message.Content
	xmlFriendly := strings.Contains(content, "<jsonplan>") || strings.Contains(content, "RESPONSE:START")

	// Check for valid JSON in the response.
	jsonStart := strings.Index(content, "[")
	jsonEnd := strings.LastIndex(content, "]")
	validJSON := false
	if jsonStart >= 0 && jsonEnd > jsonStart {
		var probe []map[string]any
		if json.Unmarshal([]byte(content[jsonStart:jsonEnd+1]), &probe) == nil {
			validJSON = true
		}
	}

	caps := OllamaModelCaps{
		NativeTools: false, // probe doesn't test tools — keep safe default
		XMLFriendly: xmlFriendly && validJSON,
		NumCtx:      defaultNumCtx,
	}

	fmt.Printf("[ollama-probe] model=%s xml=%v json=%v → XMLFriendly=%v\n", model, xmlFriendly, validJSON, caps.XMLFriendly)
	return caps
}
