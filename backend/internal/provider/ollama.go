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

// ollamaChatRequest is the JSON body sent to POST /api/chat.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// ollamaStreamChunk is one NDJSON line from the /api/chat stream.
// Only the fields we need are decoded; unknown fields are silently dropped.
type ollamaStreamChunk struct {
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
	Error   string        `json:"error,omitempty"`
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

// ── Internal helpers ──────────────────────────────────────────────────────────

// buildChatRequest converts a ChatRequest into the Ollama /api/chat JSON body.
// If a system prompt is provided it is prepended as a message with role "system".
// History messages are appended in order, followed by the new user message.
func (p *OllamaProvider) buildChatRequest(req ChatRequest) ([]byte, error) {
	msgs := make([]ollamaMessage, 0, len(req.History)+2)

	// Ollama supports an explicit "system" role message. Prepend it so the
	// model receives the XML-envelope instruction before any conversation turns.
	if req.SystemPrompt != "" {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: req.SystemPrompt})
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

	return json.Marshal(ollamaChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   true,
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
			// Generation complete; emit the sentinel and stop.
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Emit content as a chunk event; skip empty content lines.
		if chunk.Message.Content != "" {
			ch <- StreamEvent{Type: "chunk", Content: chunk.Message.Content}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("ollama: stream read error: %v", err)}
	}

	// Fallback done sentinel in case the stream ended without a done:true line.
	ch <- StreamEvent{Type: "done"}
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
