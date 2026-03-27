package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicDefaultBaseURL = "https://api.anthropic.com"
	anthropicVersion        = "2023-06-01"
	// anthropicTestMaxTokens is intentionally minimal (1 token) so that
	// TestConnection is fast and cheap regardless of the model used.
	anthropicTestMaxTokens = 1
	// anthropicChatMaxTokens is a generous default for pipeline use.
	// Callers that want a higher/lower limit should specify it via a future
	// ChatRequest field; for now a fixed ceiling is sufficient.
	anthropicChatMaxTokens = 8192
)

// AnthropicProvider implements Provider using the Anthropic Messages API
// (POST /v1/messages).  Chat requests are streamed using Anthropic's typed
// SSE format; TestConnection sends a minimal 1-token request to verify the
// API key and network connectivity; ListModels returns a hardcoded list of
// known Anthropic model IDs because the Anthropic API does not expose a
// public model-list endpoint.
type AnthropicProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAnthropicProvider returns an AnthropicProvider.  If baseURL is empty the
// production Anthropic endpoint (https://api.anthropic.com) is used.
func NewAnthropicProvider(baseURL, apiKey string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &AnthropicProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			// Streaming chat may run for many minutes; TestConnection and
			// ListModels apply their own short deadlines via context.
			Timeout: 10 * time.Minute,
		},
	}
}

// ── Anthropic request / response types ───────────────────────────────────────

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicChatRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
}

// Anthropic streaming uses typed SSE events.  Only the fields we need are
// decoded; unknown fields are silently ignored.

type anthropicDelta struct {
	Type string `json:"type"` // "text_delta" for content
	Text string `json:"text"`
}

// anthropicSSEEvent is the envelope parsed from each SSE "data:" line.
// The event type is read from the preceding "event:" line.
type anthropicSSEEvent struct {
	Type  string         `json:"type"`
	Delta anthropicDelta `json:"delta"`
	Error *anthropicErr  `json:"error,omitempty"`
}

type anthropicErr struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ── Provider interface ────────────────────────────────────────────────────────

// Chat sends the conversation to POST /v1/messages with stream:true and
// converts the Anthropic typed SSE response into a channel of StreamEvent.
//
// Anthropic streams the following event types (among others):
//
//	content_block_delta  — carries incremental text in delta.text
//	message_stop         — signals that generation is complete
//	error                — fatal streaming error
//
// Only content_block_delta events with delta.type=="text_delta" are forwarded
// as "chunk" events.  All other event types are consumed and discarded.
func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("anthropic: model must not be empty")
	}

	body, err := p.buildChatRequest(req, anthropicChatMaxTokens, true)
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/messages", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.streamResponse(ctx, resp.Body, ch)
	}()
	return ch, nil
}

// TestConnection verifies API key validity and network connectivity by sending
// a minimal 1-token request to POST /v1/messages.  A 30-second deadline is
// applied via the context.
func (p *AnthropicProvider) TestConnection(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Use claude-3-haiku-20240307 for the probe: it is the smallest/cheapest
	// model and is always available on the Anthropic API.
	probeReq := ChatRequest{
		Model:       "claude-haiku-4-5",
		UserMessage: "Hi",
	}
	body, err := p.buildChatRequest(probeReq, anthropicTestMaxTokens, false)
	if err != nil {
		return fmt.Errorf("anthropic: build test request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/messages", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("anthropic: create test request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("anthropic: test connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("anthropic: authentication failed (status %d) — check your API key", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("anthropic: test request failed (status %d): %s", resp.StatusCode, string(errBody))
	}
	return nil
}

// ListModels returns a hardcoded list of known Anthropic models.  The
// Anthropic API does not expose a public model-list endpoint, so this list
// is maintained manually.  Users who need a model that is not listed here
// can type it directly in the free-text fallback field in the UI.
func (p *AnthropicProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return []ModelInfo{
		// Claude 4 family
		{ID: "claude-opus-4-5", Name: "Claude Opus 4.5"},
		{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5"},
		// Claude 3.7
		{ID: "claude-sonnet-3-7-20250219", Name: "Claude Sonnet 3.7"},
		// Claude 3.5 family
		{ID: "claude-opus-4-6", Name: "Claude Opus 4.6"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
		{ID: "claude-haiku-4-6", Name: "Claude Haiku 4.6"},
		// Claude 3.5 legacy aliases (still widely used)
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet (Oct 2024)"},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku (Oct 2024)"},
		// Claude 3 family
		{ID: "claude-3-opus-20240229", Name: "Claude 3 Opus"},
		{ID: "claude-3-sonnet-20240229", Name: "Claude 3 Sonnet"},
		{ID: "claude-3-haiku-20240307", Name: "Claude 3 Haiku"},
	}, nil
}

// ExecuteAgent runs an agentic tool-use loop via the Anthropic Messages API.
func (p *AnthropicProvider) ExecuteAgent(ctx context.Context, req AgentRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("anthropic execute-agent: model must not be empty")
	}
	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		runAnthropicAgentLoop(ctx, p, req, ch)
	}()
	return ch, nil
}

// anthropicAgentTool is the tool definition format for the Anthropic Messages API.
type anthropicAgentTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// anthropicToolUseBlock is a content block returned when the model calls a tool.
type anthropicToolUseBlock struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// anthropicToolResultBlock is the user-role content block for tool results.
type anthropicToolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

// runAnthropicAgentLoop is the Anthropic agentic loop body.
func runAnthropicAgentLoop(ctx context.Context, p *AnthropicProvider, req AgentRequest, ch chan<- StreamEvent) {
	executor := &ToolExecutor{ProjectDir: req.ProjectDir}

	tools := make([]anthropicAgentTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = anthropicAgentTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	// Anthropic uses separate top-level "system" field; messages are user/assistant only.
	type anthropicMsg struct {
		Role    string `json:"role"`
		Content any    `json:"content"` // string or []map[string]any
	}

	msgs := []anthropicMsg{
		{Role: "user", Content: req.UserMessage},
	}

	apiURL := fmt.Sprintf("%s/v1/messages", p.baseURL)

	for iter := 0; iter < maxAgentIterations; iter++ {
		if ctx.Err() != nil {
			ch <- StreamEvent{Type: "error", Content: ctx.Err().Error()}
			return
		}

		body := map[string]any{
			"model":      req.Model,
			"max_tokens": anthropicChatMaxTokens,
			"system":     req.SystemPrompt,
			"messages":   msgs,
			"tools":      tools,
			"stream":     false,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic execute-agent: marshal: %v", err)}
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
		if err != nil {
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic execute-agent: create request: %v", err)}
			return
		}
		p.setHeaders(httpReq)

		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic execute-agent: request: %v", err)}
			return
		}
		if resp.StatusCode != http.StatusOK {
			errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic execute-agent: HTTP %d: %s", resp.StatusCode, string(errBody))}
			return
		}

		// Parse the non-streaming response. The content field is []ContentBlock.
		var apiResp struct {
			StopReason string `json:"stop_reason"` // "tool_use" or "end_turn"
			Content    []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text,omitempty"`
				ID    string          `json:"id,omitempty"`
				Name  string          `json:"name,omitempty"`
				Input map[string]any  `json:"input,omitempty"`
			} `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			resp.Body.Close()
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic execute-agent: decode: %v", err)}
			return
		}
		resp.Body.Close()

		if apiResp.StopReason == "tool_use" {
			// Build assistant message containing all content blocks.
			var assistantBlocks []map[string]any
			var toolUseBlocks []anthropicToolUseBlock
			for _, blk := range apiResp.Content {
				switch blk.Type {
				case "text":
					assistantBlocks = append(assistantBlocks, map[string]any{"type": "text", "text": blk.Text})
				case "tool_use":
					assistantBlocks = append(assistantBlocks, map[string]any{
						"type":  "tool_use",
						"id":    blk.ID,
						"name":  blk.Name,
						"input": blk.Input,
					})
					toolUseBlocks = append(toolUseBlocks, anthropicToolUseBlock{
						Type: blk.Type, ID: blk.ID, Name: blk.Name, Input: blk.Input,
					})
				}
			}
			msgs = append(msgs, anthropicMsg{Role: "assistant", Content: assistantBlocks})

			// Build tool results as a single user message with multiple content blocks.
			var resultBlocks []map[string]any
			for _, tub := range toolUseBlocks {
				ch <- StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] executing...", tub.Name)}
				toolOutput, toolErr := executor.Execute(ctx, tub.Name, tub.Input)
				if toolErr != nil {
					toolOutput = fmt.Sprintf("error: %v\n%s", toolErr, toolOutput)
				}
				ch <- StreamEvent{Type: "log", Content: fmt.Sprintf("[%s] done", tub.Name)}
				resultBlocks = append(resultBlocks, map[string]any{
					"type":        "tool_result",
					"tool_use_id": tub.ID,
					"content":     toolOutput,
				})
			}
			msgs = append(msgs, anthropicMsg{Role: "user", Content: resultBlocks})
			continue
		}

		// Final answer: collect text blocks.
		var sb strings.Builder
		for _, blk := range apiResp.Content {
			if blk.Type == "text" && blk.Text != "" {
				sb.WriteString(blk.Text)
			}
		}
		text := sb.String()
		if text != "" {
			ch <- StreamEvent{Type: "chunk", Content: text}
		}
		ch <- StreamEvent{Type: "done", Content: text}
		return
	}
	ch <- StreamEvent{Type: "error", Content: "anthropic execute-agent: max iterations reached"}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// setHeaders applies the authentication and versioning headers required by
// the Anthropic Messages API on every outbound request.
func (p *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
}

// buildChatRequest converts a ChatRequest into the Anthropic Messages API
// JSON wire format.  When stream is true the request body includes
// "stream": true which switches the API into SSE mode.
//
// The Anthropic API requires:
//   - messages must contain at least one element.
//   - roles must alternate between "user" and "assistant".
//   - The last message must have role "user".
func (p *AnthropicProvider) buildChatRequest(req ChatRequest, maxTokens int, stream bool) ([]byte, error) {
	msgs := make([]anthropicMessage, 0, len(req.History)+1)

	for _, m := range req.History {
		role := string(m.Role)
		if role == "assistant" {
			// Anthropic uses "assistant"; most of our history already uses this
			// but guard against alternative conventions.
			role = "assistant"
		}
		msgs = append(msgs, anthropicMessage{Role: role, Content: m.Content})
	}

	// Append the new user turn.
	msgs = append(msgs, anthropicMessage{Role: "user", Content: req.UserMessage})

	ar := anthropicChatRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  msgs,
		Stream:    stream,
	}
	if req.SystemPrompt != "" {
		ar.System = req.SystemPrompt
	}

	return json.Marshal(ar)
}

// streamResponse reads the Anthropic SSE response body and emits StreamEvent
// values on ch.  The Anthropic streaming format consists of paired lines:
//
//	event: <event-type>
//	data:  <json-payload>
//
// The event types we act on are:
//
//	content_block_delta  — incremental text; emit "chunk" if delta.type=="text_delta"
//	message_stop         — generation complete; emit "done"
//	error                — streaming error; emit "error" then "done"
//
// All other event types (message_start, content_block_start,
// content_block_stop, message_delta, ping) are consumed and discarded.
//
// A "done" event is always the final event emitted, even on the error path,
// so callers that drain until "done" never block indefinitely.
func (p *AnthropicProvider) streamResponse(ctx context.Context, body io.Reader, ch chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentEventType string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: "error", Content: ctx.Err().Error()}
			ch <- StreamEvent{Type: "done"}
			return
		default:
		}

		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "event:"):
			// Capture the event type; the following "data:" line carries the payload.
			currentEventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))

		case strings.HasPrefix(line, "data:"):
			jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if jsonStr == "" || jsonStr == "[DONE]" {
				continue
			}

			p.handleSSEData(currentEventType, jsonStr, ch)

			// Reset after consumption so a stray data-only line is not
			// misattributed to the previous event type.
			currentEventType = ""

		default:
			// Blank lines separate SSE events; other lines are ignored.
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic: stream read error: %v", err)}
	}

	ch <- StreamEvent{Type: "done"}
}

// handleSSEData dispatches a single data payload based on the event type that
// preceded it.  It is extracted from streamResponse to keep the scan loop
// readable.
func (p *AnthropicProvider) handleSSEData(eventType, jsonStr string, ch chan<- StreamEvent) {
	switch eventType {
	case "content_block_delta":
		var ev anthropicSSEEvent
		if err := json.Unmarshal([]byte(jsonStr), &ev); err != nil {
			// Non-fatal: log and continue.
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic: malformed content_block_delta: %v", err)}
			return
		}
		if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
			ch <- StreamEvent{Type: "chunk", Content: ev.Delta.Text}
		}

	case "message_stop":
		// The message is complete.  We emit "done" at the end of streamResponse
		// after the scanner finishes, so we do nothing here — the sentinel is
		// already guaranteed.

	case "error":
		var ev anthropicSSEEvent
		if err := json.Unmarshal([]byte(jsonStr), &ev); err != nil {
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic: streaming error (malformed): %v", err)}
			return
		}
		if ev.Error != nil {
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("anthropic: %s: %s", ev.Error.Type, ev.Error.Message)}
		}

	default:
		// message_start, content_block_start, content_block_stop,
		// message_delta, ping — silently ignore.
	}
}
