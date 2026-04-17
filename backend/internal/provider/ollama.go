package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OllamaProvider implements Provider using the Ollama HTTP API.
// Chat streams via POST /api/chat (NDJSON format); model discovery uses
// GET /api/tags. No authentication headers are required because Ollama is
// designed as a local service.
//
// Streaming format: Ollama returns newline-delimited JSON (NDJSON) objects:
//
//	{"message":{"role":"assistant","content":"hello "},"done":false}
//	{"message":{"role":"assistant","content":"world"},"done":false}
//	{"done":true,"total_duration":...}
//
// Each non-done line with non-empty content is converted to a "chunk"
// StreamEvent. The line with "done":true triggers a "done" StreamEvent.
type OllamaProvider struct {
	baseURL    string
	httpClient *http.Client
}

// resolveModelCaps returns the capability profile for the model. If the model
// is unknown in the static table, it triggers a runtime probe and caches the result.
func (p *OllamaProvider) resolveModelCaps(model string) OllamaModelCaps {
	caps := ollamaModelCaps(model)
	if caps == safeDefault {
		// Unknown model — try runtime probe.
		return ProbeOllamaModelCaps(p.baseURL, model)
	}
	return caps
}

// NewOllamaProvider returns an OllamaProvider configured to talk to the
// Ollama instance at baseURL (e.g. "http://localhost:11434"). Trailing
// slashes are stripped for consistency.
func NewOllamaProvider(baseURL string) *OllamaProvider {
	baseURL = strings.TrimRight(baseURL, "/")
	return &OllamaProvider{
		baseURL: baseURL,
		httpClient: &http.Client{
			// Streaming chat can run for many minutes; TestConnection and
			// ListModels apply their own short deadlines via context.
			Timeout: 10 * time.Minute,
		},
	}
}

// ── Ollama request / response types ──────────────────────────────────────────

// ollamaMessage mirrors the Ollama /api/chat message format.
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatOptions holds runtime parameters sent in the "options" field.
type ollamaChatOptions struct {
	NumCtx      int     `json:"num_ctx,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// ollamaChatRequest is the JSON body sent to POST /api/chat.
type ollamaChatRequest struct {
	Model    string           `json:"model"`
	Messages []ollamaMessage  `json:"messages"`
	Stream   bool             `json:"stream"`
	Options  *ollamaChatOptions `json:"options,omitempty"`
}

// ollamaStreamChunk is one NDJSON line from the /api/chat stream.
// Only the fields we need are decoded; unknown fields are silently dropped.
type ollamaStreamChunk struct {
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	Error           string        `json:"error,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

// ollamaModel is one entry in the GET /api/tags response.
type ollamaModel struct {
	Name string `json:"name"` // e.g. "llama3:8b", "mistral:7b"
}

// ollamaTagsResponse is the JSON body returned by GET /api/tags.
type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

// ── Provider interface ────────────────────────────────────────────────────────

// Chat sends the conversation to POST /api/chat with stream:true and converts
// the NDJSON response into a channel of StreamEvent values.
//
// Each line from the stream is unmarshalled as an ollamaStreamChunk:
//   - Lines with done:false emit a "chunk" event when content is non-empty.
//   - The line with done:true closes the stream with a "done" event.
//   - Lines carrying a non-empty "error" field emit an "error" then "done".
func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("ollama: model must not be empty")
	}

	body, err := p.buildChatRequest(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}

	// --- DEBUG: write full prompt to file (only when PAULETTE_DEBUG_DIR is set) ---
	if debugDir := os.Getenv("PAULETTE_DEBUG_DIR"); debugDir != "" {
		label := req.Stage
		if label == "" {
			label = "unknown"
		}
		debugFile := fmt.Sprintf("%s/%s_prompt_%d.json", debugDir, label, time.Now().UnixNano())
		if f, err := os.Create(debugFile); err == nil {
			_, _ = f.Write(body)
			_ = f.Close()
			fmt.Printf("Ollama request body written to %s\n", debugFile)
		}
	}

	url := fmt.Sprintf("%s/api/chat", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.streamResponse(ctx, resp.Body, ch)
	}()
	return ch, nil
}

// TestConnection verifies that the Ollama instance is reachable by calling
// GET /api/tags. A 30-second deadline is applied via the context. No
// credentials are required; the only failure modes are network errors and
// unexpected HTTP status codes.
func (p *OllamaProvider) TestConnection(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := p.fetchModels(ctx)
	if err != nil {
		return fmt.Errorf("ollama: test connection: %w", err)
	}
	return nil
}

// ListModels returns all models available on the Ollama instance by querying
// GET /api/tags. A 30-second deadline is applied via the context.
func (p *OllamaProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models, err := p.fetchModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("ollama: list models: %w", err)
	}
	return models, nil
}

// ── System prompt adaptation ──────────────────────────────────────────────────

// xmlEnvelopeMarker is a substring present in any prompt that uses the
// <!-- RESPONSE:START --> envelope convention. Used to detect whether wrapping is needed.
const xmlEnvelopeMarker = "<!-- RESPONSE:START -->"

// ollamaWrapSystemPrompt adapts the base system prompt for models that are not
// XML-friendly. For XMLFriendly models the prompt is returned unchanged.
//
// Non-XML-friendly models receive XML envelope instructions replaced with
// simpler alternatives that those models follow more reliably:
//   - Stage prompts (with discussion/artifact): get "## Discussion / ## Artifact" sections
//   - HTML generation prompts (with CDATA): get code-fence instructions
//   - Other prompts with XML markers: get markers stripped
//
// If the prompt does not contain the XML envelope marker, it is returned
// unchanged regardless of capability.
func ollamaWrapSystemPrompt(basePrompt string, caps OllamaModelCaps) string {
	if caps.XMLFriendly {
		return basePrompt
	}
	if !strings.Contains(basePrompt, xmlEnvelopeMarker) {
		return basePrompt
	}

	// Detect prompt type by content and apply appropriate replacement.
	hasHTMLContent := strings.Contains(basePrompt, "<htmlcontent>")
	hasArtifact := strings.Contains(basePrompt, "<artifact>") || strings.Contains(basePrompt, "<discussion>")

	if hasHTMLContent {
		return ollamaAdaptHTMLPrompt(basePrompt)
	}
	if hasArtifact {
		return ollamaAdaptStagePrompt(basePrompt)
	}

	// Generic: remove the XML envelope markers and add a plain-output note.
	result := strings.ReplaceAll(basePrompt, xmlEnvelopeMarker, "")
	result = strings.ReplaceAll(result, "<!-- RESPONSE:END -->", "")
	return result + "\n\nOutput your response directly without XML wrapper tags."
}

// ollamaAdaptStagePrompt replaces XML envelope instructions with plain-section format
// for stage prompts that use <discussion>/<artifact> structure.
func ollamaAdaptStagePrompt(basePrompt string) string {
	plainFormat := `Structure your response using these plain sections:

## Discussion
Your explanation, reasoning, analysis or questions go here.

## Artifact
The complete document, code, or deliverable goes here.

Always include both sections. Do not use XML tags.`

	// Find the paragraph containing the XML envelope marker and replace it.
	paragraphs := strings.Split(basePrompt, "\n\n")
	replaced := false
	for i, p := range paragraphs {
		if strings.Contains(p, xmlEnvelopeMarker) {
			paragraphs[i] = plainFormat
			replaced = true
			break
		}
	}
	if replaced {
		return strings.Join(paragraphs, "\n\n")
	}
	return basePrompt + "\n\n" + plainFormat
}

// ollamaAdaptHTMLPrompt replaces XML/CDATA envelope instructions with code-fence
// format for HTML generation prompts (mock components, styler, views).
func ollamaAdaptHTMLPrompt(basePrompt string) string {
	plainFormat := "OUTPUT FORMAT — follow this exactly:\n" +
		"Return ONLY the HTML content inside a code fence:\n\n" +
		"```html\n...your HTML here...\n```\n\n" +
		"Do not write anything outside the code fence."

	// Find the paragraph containing the XML envelope marker and replace it.
	paragraphs := strings.Split(basePrompt, "\n\n")
	replaced := false
	for i, p := range paragraphs {
		if strings.Contains(p, xmlEnvelopeMarker) {
			paragraphs[i] = plainFormat
			replaced = true
			break
		}
	}

	result := basePrompt
	if replaced {
		result = strings.Join(paragraphs, "\n\n")
	} else {
		result = basePrompt + "\n\n" + plainFormat
	}

	// Also strip any remaining CDATA/XML tags from examples in the prompt.
	result = strings.ReplaceAll(result, "<htmlcontent><![CDATA[", "")
	result = strings.ReplaceAll(result, "]]></htmlcontent>", "")
	result = strings.ReplaceAll(result, xmlEnvelopeMarker, "")
	result = strings.ReplaceAll(result, "<!-- RESPONSE:END -->", "")
	return result
}

// ollamaStageDefault holds sensible Ollama defaults for a pipeline stage.
// numCtx is intentionally absent: context window size comes from model caps
// (ollamaCapTable / EffectiveNumCtx) so that the model's actual capacity is
// used rather than a small generic constant that causes truncation.
type ollamaStageDefault struct {
	temperature float64
}

// ollamaStageDefaults maps pipeline stage names to their recommended Ollama
// defaults. These are used when no explicit value is provided in ChatRequest.
// Users may override any field via StageAssignment in their config files.
var ollamaStageDefaults = map[string]ollamaStageDefault{
	"vision":       {temperature: 0.9}, // high creativity for brainstorming
	"ux":           {temperature: 0.6}, // structured reasoning for UX design
	"ui":           {temperature: 0.4}, // focused code gen for HTML mockups
	"architecture": {temperature: 0.5}, // balanced reasoning for arch diagrams
	"build":        {temperature: 0.4}, // deterministic build plans
	"complete":     {temperature: 0.5}, // summary generation
}

// ollamaTemperatureForPrompt selects a temperature based on prompt content.
// This is the fallback heuristic when no stage default or explicit value applies.
// Structured output prompts (JSON planning, code generation) use low temperature
// for predictable formatting; conversational prompts use moderate temperature.
func ollamaTemperatureForPrompt(systemPrompt string) float64 {
	lower := strings.ToLower(systemPrompt)
	// Low temperature for structured output: JSON plans, code generation, summaries
	if strings.Contains(lower, "<jsonplan>") ||
		strings.Contains(lower, "json array") ||
		strings.Contains(lower, "json object") ||
		strings.Contains(lower, "code fence") ||
		strings.Contains(lower, "iteration summary") {
		return 0.3
	}
	return 0.7
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// buildChatRequest converts a ChatRequest into the Ollama /api/chat JSON body.
// The system prompt is adapted for the model's XML capability before use.
// If a system prompt is provided it is prepended as a message with role "system".
// History messages are appended in order, followed by the new user message.
func (p *OllamaProvider) buildChatRequest(req ChatRequest) ([]byte, error) {
	caps := p.resolveModelCaps(req.Model)
	systemPrompt := ollamaWrapSystemPrompt(req.SystemPrompt, caps)

	msgs := make([]ollamaMessage, 0, len(req.History)+2)

	// Ollama supports an explicit "system" role message. Prepend it so the
	// model receives the output format instruction before any conversation turns.
	if systemPrompt != "" {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: systemPrompt})
	}

	for _, m := range req.History {
		role := string(m.Role)
		// The pipeline uses "assistant"; Ollama expects "assistant" as well, so
		// no translation is needed. Guard against unexpected role values by
		// passing them through unchanged — Ollama will reject unknown roles
		// with a 400, which surfaces as a clear error via the status check.
		msgs = append(msgs, ollamaMessage{Role: role, Content: m.Content})
	}

	// Append the new user turn.
	msgs = append(msgs, ollamaMessage{Role: "user", Content: req.UserMessage})

	// Resolve temperature: explicit override → stage default → prompt heuristic.
	var temp float64
	if req.Temperature != nil {
		temp = *req.Temperature
	} else if sd, ok := ollamaStageDefaults[req.Stage]; ok {
		temp = sd.temperature
	} else {
		temp = ollamaTemperatureForPrompt(systemPrompt)
	}

	// Resolve numCtx: explicit override → model capability.
	// Stage defaults are not used for numCtx; the model's actual context
	// window (from ollamaCapTable) must be respected to avoid truncation.
	var numCtx int
	if req.NumCtx != nil {
		numCtx = *req.NumCtx
	} else {
		numCtx = caps.EffectiveNumCtx()
	}

	// Resolve stream: explicit override → default true.
	stream := true
	if req.Stream != nil {
		stream = *req.Stream
	}

	return json.Marshal(ollamaChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   stream,
		Options: &ollamaChatOptions{
			NumCtx:      numCtx,
			Temperature: temp,
		},
	})
}

// streamResponse reads the NDJSON stream from the Ollama /api/chat endpoint
// and emits StreamEvent values on ch.
//
// Ollama streams one JSON object per line:
//
//	{"message":{"role":"assistant","content":"<token>"},"done":false}
//	{"done":true,"total_duration":...}
//
// Content lines with done:false emit "chunk" events.
// The final line with done:true emits the "done" sentinel.
// Lines containing a non-empty "error" field emit "error" then "done" and
// terminate the stream early.
//
// A "done" event is always the last event emitted, even on the error path,
// so callers that drain until "done" never block indefinitely.
func (p *OllamaProvider) streamResponse(ctx context.Context, body io.Reader, ch chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	// Increase buffer to accommodate large token chunks.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		// Honour context cancellation between lines.
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: "error", Content: ctx.Err().Error()}
			ch <- StreamEvent{Type: "done"}
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			// Non-fatal: skip malformed lines and continue.
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("ollama: malformed NDJSON line: %v", err)}
			continue
		}

		// Surface provider-level errors (e.g. model not found) immediately.
		if chunk.Error != "" {
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("ollama: %s", chunk.Error)}
			ch <- StreamEvent{Type: "done"}
			return
		}

		if chunk.Done {
			// Emit token count if available (Ollama includes this in the final response).
			totalTokens := chunk.PromptEvalCount + chunk.EvalCount
			if totalTokens > 0 {
				ch <- StreamEvent{Type: "tokens", Content: fmt.Sprintf("%d", totalTokens)}
			}
			// Generation complete; emit the sentinel and stop.
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Emit content as a chunk event; skip empty content lines.
		// Truncate at known instruction-format stop tokens that some Llama-family
		// models leak into their output (e.g. [/INST], <|eot_id|>). If found,
		// emit the clean prefix and stop — this prevents duplicated artifact content.
		if chunk.Message.Content != "" {
			content, stopped := truncateAtStopToken(chunk.Message.Content)
			if content != "" {
				ch <- StreamEvent{Type: "chunk", Content: content}
			}
			if stopped {
				ch <- StreamEvent{Type: "done"}
				return
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("ollama: stream read error: %v", err)}
	}

	// Fallback done sentinel in case the stream ended without a done:true line.
	ch <- StreamEvent{Type: "done"}
}

// ExecuteAgent runs an agentic tool-use loop via the Ollama /api/chat endpoint.
// Models with NativeTools support use the structured "tools" API field.
// Models without native tool support use the pseudo-tool text loop, where the
// model emits <tool_call> markers and the backend executes them.
func (p *OllamaProvider) ExecuteAgent(ctx context.Context, req AgentRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("ollama execute-agent: model must not be empty")
	}
	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		runOllamaAgentLoop(ctx, p, req, ch)
	}()
	return ch, nil
}

// ollamaAgentToolCall is a tool call entry in an Ollama agentic response.
type ollamaAgentToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

// ollamaAgentMsg is a message in the Ollama agentic conversation.
type ollamaAgentMsg struct {
	Role      string                `json:"role"`
	Content   string                `json:"content"`
	ToolCalls []ollamaAgentToolCall `json:"tool_calls,omitempty"`
}

// ollamaAgentResp is the non-streaming response from POST /api/chat.
type ollamaAgentResp struct {
	Message ollamaAgentMsg `json:"message"`
	Done    bool           `json:"done"`
	Error   string         `json:"error,omitempty"`
}

// runOllamaAgentLoop is the Ollama agentic loop.
// It branches on model capabilities: models with NativeTools use the structured
// tool_calls API; all others use the pseudo-tool text fallback.
func runOllamaAgentLoop(ctx context.Context, p *OllamaProvider, req AgentRequest, ch chan<- StreamEvent) {
	caps := p.resolveModelCaps(req.Model)
	executor := &ToolExecutor{ProjectDir: req.ProjectDir}
	msgs := ollamaInitMessages(req, caps)
	toolSchemas := ollamaBuildToolSchemas(req.Tools, caps)
	apiURL := fmt.Sprintf("%s/api/chat", p.baseURL)

	usePseudoTools := !caps.NativeTools && len(req.Tools) > 0
	var cont bool
	var err error

	for iter := 0; iter < maxAgentIterations; iter++ {
		if ctx.Err() != nil {
			ch <- StreamEvent{Type: "error", Content: ctx.Err().Error()}
			return
		}

		var apiResp ollamaAgentResp
		apiResp, err = p.ollamaAgentPost(ctx, apiURL, req.Model, msgs, toolSchemas)
		if err != nil {
			ch <- StreamEvent{Type: "error", Content: err.Error()}
			return
		}

		msgs, cont, err = ollamaDispatchTools(ctx, executor, usePseudoTools, msgs, apiResp, toolSchemas, ch)
		if err != nil {
			ch <- StreamEvent{Type: "error", Content: err.Error()}
			return
		}
		if cont {
			continue
		}

		if apiResp.Message.Content != "" {
			ch <- StreamEvent{Type: "chunk", Content: apiResp.Message.Content}
		}
		ch <- StreamEvent{Type: "done", Content: apiResp.Message.Content}
		return
	}
	ch <- StreamEvent{Type: "error", Content: "ollama execute-agent: max iterations reached"}
}

// ollamaDispatchTools routes a model response to the native-tool or pseudo-tool
// handler and returns the updated message history and whether the loop should continue.
func ollamaDispatchTools(ctx context.Context, executor *ToolExecutor, usePseudoTools bool, msgs []ollamaAgentMsg, apiResp ollamaAgentResp, toolSchemas []map[string]any, ch chan<- StreamEvent) ([]ollamaAgentMsg, bool, error) {
	if len(toolSchemas) > 0 {
		return runNativeToolCalls(ctx, executor, msgs, apiResp, toolSchemas, ch)
	}
	if usePseudoTools {
		updated, found := runPseudoToolCall(ctx, executor, msgs, apiResp.Message.Content, ch)
		return updated, found, nil
	}
	return msgs, false, nil
}

// ollamaInitMessages builds the initial message slice for the agent loop,
// applying prompt wrapping and pseudo-tool augmentation as needed.
func ollamaInitMessages(req AgentRequest, caps OllamaModelCaps) []ollamaAgentMsg {
	systemPrompt := ollamaWrapSystemPrompt(req.SystemPrompt, caps)
	if !caps.NativeTools && len(req.Tools) > 0 {
		systemPrompt = buildPseudoToolSystemPrompt(systemPrompt, req.Tools)
	}
	return []ollamaAgentMsg{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: req.UserMessage},
	}
}

// ollamaBuildToolSchemas converts the tool list to LLM schemas for native-tool models.
// Returns nil for pseudo-tool models so the "tools" key is omitted from the request.
func ollamaBuildToolSchemas(tools []AgentTool, caps OllamaModelCaps) []map[string]any {
	if !caps.NativeTools {
		return nil
	}
	schemas := make([]map[string]any, len(tools))
	for i, t := range tools {
		schemas[i] = ToolSchemaForLLM(t)
	}
	return schemas
}

// ollamaAgentPost sends one non-streaming request to /api/chat and returns the decoded response.
func (p *OllamaProvider) ollamaAgentPost(ctx context.Context, apiURL, model string, msgs []ollamaAgentMsg, toolSchemas []map[string]any) (ollamaAgentResp, error) {
	caps := p.resolveModelCaps(model)
	body := map[string]any{
		"model":    model,
		"messages": msgs,
		"stream":   false,
		"options": map[string]any{
			"num_ctx":     caps.EffectiveNumCtx(),
			"temperature": 0.3, // agent/tool-use path benefits from low temperature
		},
	}
	if len(toolSchemas) > 0 {
		body["tools"] = toolSchemas
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return ollamaAgentResp{}, fmt.Errorf("ollama execute-agent: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ollamaAgentResp{}, fmt.Errorf("ollama execute-agent: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ollamaAgentResp{}, fmt.Errorf("ollama execute-agent: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ollamaAgentResp{}, fmt.Errorf("ollama execute-agent: HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	var apiResp ollamaAgentResp
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return ollamaAgentResp{}, fmt.Errorf("ollama execute-agent: decode: %w", err)
	}
	if apiResp.Error != "" {
		return ollamaAgentResp{}, fmt.Errorf("ollama execute-agent: %s", apiResp.Error)
	}
	return apiResp, nil
}

// runNativeToolCalls handles tool_calls from a native-tool model response.
// Returns the updated message slice, whether the loop should continue, and any error.
func runNativeToolCalls(ctx context.Context, executor *ToolExecutor, msgs []ollamaAgentMsg, apiResp ollamaAgentResp, toolSchemas []map[string]any, ch chan<- StreamEvent) ([]ollamaAgentMsg, bool, error) {
	if len(apiResp.Message.ToolCalls) == 0 {
		if len(toolSchemas) > 0 && apiResp.Message.Content == "" {
			return msgs, false, fmt.Errorf("ollama: model does not support tool use; switch to a tool-capable model")
		}
		return msgs, false, nil
	}

	msgs = append(msgs, apiResp.Message)
	for _, tc := range apiResp.Message.ToolCalls {
		name := tc.Function.Name
		ch <- StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] executing...", name)}
		output := executeTool(ctx, executor, name, tc.Function.Arguments)
		ch <- StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] done", name)}
		msgs = append(msgs, ollamaAgentMsg{Role: "tool", Content: output})
	}
	return msgs, true, nil
}

// runPseudoToolCall checks for a <tool_call> marker in content, executes it, and
// injects the result as a user message. Returns updated msgs and whether a call was found.
func runPseudoToolCall(ctx context.Context, executor *ToolExecutor, msgs []ollamaAgentMsg, content string, ch chan<- StreamEvent) ([]ollamaAgentMsg, bool) {
	name, args, found := parsePseudoToolCall(content)
	if !found {
		return msgs, false
	}
	ch <- StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] executing...", name)}
	output := executeTool(ctx, executor, name, args)
	ch <- StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] done", name)}
	msgs = append(msgs, ollamaAgentMsg{Role: "assistant", Content: content})
	result := fmt.Sprintf("<tool_result tool=%q>\n%s\n</tool_result>", name, output)
	msgs = append(msgs, ollamaAgentMsg{Role: "user", Content: result})
	return msgs, true
}

// executeTool runs a single tool and merges any error into the output string.
func executeTool(ctx context.Context, executor *ToolExecutor, name string, args map[string]any) string {
	output, err := executor.Execute(ctx, name, args)
	if err != nil {
		return fmt.Sprintf("error: %v\n%s", err, output)
	}
	return output
}

// ── Stop-token helpers ────────────────────────────────────────────────────────

// knownStopTokens are instruction-format tokens that some Llama-family models
// emit verbatim in their output when they lose track of the chat template
// boundary. Encountering one means the model has finished its real response and
// is about to repeat itself (or output training-data artefacts). We truncate the
// stream at the first occurrence to prevent duplicated artifact content.
var knownStopTokens = []string{
	"[/INST]",    // Llama-2 end-of-instruction
	"[/INSTS]",   // Llama-2 variant seen in the wild
	"<|eot_id|>", // Llama-3 end-of-turn
	"<|end|>",    // Phi-family end token
	"<|im_end|>", // ChatML (Qwen, Mistral-Nemo, etc.)
}

// truncateAtStopToken returns (content[:idx], true) when a known stop token is
// found in content, or (content, false) when none are present.
func truncateAtStopToken(content string) (string, bool) {
	for _, tok := range knownStopTokens {
		if idx := strings.Index(content, tok); idx >= 0 {
			return content[:idx], true
		}
	}
	return content, false
}

// ── Pseudo-tool helpers ───────────────────────────────────────────────────────

// pseudoToolCallOpen and Close are the markers used in the text-based tool protocol.
const (
	pseudoToolCallOpen  = "<tool_call>"
	pseudoToolCallClose = "</tool_call>"
)

// parsePseudoToolCall searches text for the first <tool_call>...</tool_call>
// marker and parses the JSON payload within it.
// Returns the tool name, args map, and whether a valid call was found.
// Malformed JSON inside a marker is treated as "not found" to prevent infinite loops.
func parsePseudoToolCall(text string) (name string, args map[string]any, found bool) {
	start := strings.Index(text, pseudoToolCallOpen)
	if start < 0 {
		return "", nil, false
	}
	inner := text[start+len(pseudoToolCallOpen):]
	end := strings.Index(inner, pseudoToolCallClose)
	if end < 0 {
		return "", nil, false
	}
	payload := strings.TrimSpace(inner[:end])

	var call struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(payload), &call); err != nil {
		return "", nil, false
	}
	if call.Tool == "" {
		return "", nil, false
	}
	return call.Tool, call.Args, true
}

// buildPseudoToolSystemPrompt appends a plain-text tool manifest to basePrompt.
// This is called for models that do not support the native "tools" API field.
// The manifest teaches the model how to express tool calls in text form.
func buildPseudoToolSystemPrompt(basePrompt string, tools []AgentTool) string {
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n## Available Tools\n\n")
	sb.WriteString("You may invoke tools by writing a tool_call block in your response.\n")
	sb.WriteString("Use this exact format (one block per tool call, on its own line):\n\n")
	sb.WriteString("<tool_call>{\"tool\": \"ToolName\", \"args\": {\"param\": \"value\"}}</tool_call>\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Emit exactly one <tool_call> block per invocation.\n")
	sb.WriteString("- Wait for the <tool_result> before calling the next tool.\n")
	sb.WriteString("- When you have all the information you need, respond normally without any <tool_call> block.\n")
	sb.WriteString("- Only use the tools listed below. Unknown tool names will return an error.\n\n")
	sb.WriteString("### Tool Reference\n\n")
	for _, t := range tools {
		sb.WriteString(pseudoToolDescription(t))
		sb.WriteString("\n")
	}
	return sb.String()
}

// pseudoToolDescription renders a single tool as a compact plain-text description
// for inclusion in the pseudo-tool system prompt.
func pseudoToolDescription(t AgentTool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%s** — %s\n", t.Name, t.Description))
	props, ok := t.InputSchema["properties"].(map[string]any)
	if !ok {
		return sb.String()
	}
	required := extractRequiredSet(t.InputSchema)
	for name, def := range props {
		sb.WriteString(renderPropLine(name, def, required[name]))
	}
	return sb.String()
}

// extractRequiredSet returns the set of required property names from a JSON Schema object.
// Handles both []string and []any (the latter from JSON unmarshalling into map[string]any).
func extractRequiredSet(schema map[string]any) map[string]bool {
	required := map[string]bool{}
	switch v := schema["required"].(type) {
	case []string:
		for _, r := range v {
			required[r] = true
		}
	case []any:
		for _, r := range v {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}
	return required
}

// renderPropLine formats one property entry for the pseudo-tool manifest.
func renderPropLine(name string, def any, isRequired bool) string {
	propMap, ok := def.(map[string]any)
	if !ok {
		return ""
	}
	typ, _ := propMap["type"].(string)
	desc, _ := propMap["description"].(string)
	req := ""
	if isRequired {
		req = " (required)"
	}
	if desc != "" {
		return fmt.Sprintf("  - %s: %s%s — %s\n", name, typ, req, desc)
	}
	return fmt.Sprintf("  - %s: %s%s\n", name, typ, req)
}

// fetchModels calls GET /api/tags and returns the list of locally available
// models. The caller is responsible for applying a deadline via ctx.
func (p *OllamaProvider) fetchModels(ctx context.Context) ([]ModelInfo, error) {
	url := fmt.Sprintf("%s/api/tags", p.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		models = append(models, ModelInfo{
			ID:   m.Name,
			Name: m.Name,
		})
	}
	return models, nil
}
