package provider

// openai_compat.go provides a shared implementation of the OpenAI Chat
// Completions API protocol. Both the OpenAI and LM Studio providers delegate
// to these helpers; they differ only in base URL defaults and auth headers.
//
// Streaming format: OpenAI-compatible APIs stream server-sent events (SSE)
// where each event is a line prefixed with "data: " containing a JSON object:
//
//	data: {"id":"...","choices":[{"delta":{"content":"hello"},"finish_reason":null}]}
//	data: {"id":"...","choices":[{"delta":{"content":" world"},"finish_reason":null}]}
//	data: [DONE]
//
// Each delta.content fragment is emitted as a "chunk" StreamEvent.
// The "data: [DONE]" sentinel triggers the final "done" StreamEvent.

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

// ── OpenAI-compatible request / response types ────────────────────────────────

// openAIMessage is a single entry in the OpenAI messages array.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatRequest is the JSON body sent to POST /v1/chat/completions.
type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// openAIStreamChunk is one SSE payload from the /v1/chat/completions stream.
// Only the fields we need are decoded; unused fields are silently ignored.
type openAIStreamChunk struct {
	Choices []openAIChoice `json:"choices"`
	Error   *openAIError   `json:"error,omitempty"`
}

type openAIChoice struct {
	Delta        openAIDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type openAIDelta struct {
	Content string `json:"content"`
	Role    string `json:"role,omitempty"`
}

// openAIError is the error object that may appear inside an SSE payload or a
// non-2xx JSON response body.
type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

// openAIModelsResponse is the JSON body returned by GET /v1/models.
type openAIModelsResponse struct {
	Data []openAIModelEntry `json:"data"`
}

// openAIModelEntry is a single model record from GET /v1/models.
type openAIModelEntry struct {
	ID string `json:"id"`
}

// ── Shared HTTP clients ───────────────────────────────────────────────────────

// newOpenAICompatChatClient returns an HTTP client sized for long-running SSE
// streams (10-minute timeout, no automatic redirect following for POST bodies).
func newOpenAICompatChatClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute}
}

// newOpenAICompatTestClient returns an HTTP client sized for short probe
// requests (30-second timeout).
func newOpenAICompatTestClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// ── Message conversion ────────────────────────────────────────────────────────

// buildOpenAIMessages converts a ChatRequest into an OpenAI messages array.
// The system prompt (if non-empty) is prepended as a "system" role message,
// followed by the conversation history, then the new user turn.
func buildOpenAIMessages(req ChatRequest) []openAIMessage {
	msgs := make([]openAIMessage, 0, len(req.History)+2)

	if req.SystemPrompt != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: req.SystemPrompt})
	}

	for _, m := range req.History {
		msgs = append(msgs, openAIMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}

	msgs = append(msgs, openAIMessage{Role: "user", Content: req.UserMessage})
	return msgs
}

// ── Core shared functions ─────────────────────────────────────────────────────

// openAICompatChat sends a streaming chat request to baseURL/v1/chat/completions
// and returns a channel of StreamEvent values.
//
// authHeaders is a map of HTTP header names to values that will be added to
// the request (e.g. "Authorization": "Bearer sk-…"). Pass nil for no auth.
//
// The caller-provided ctx controls cancellation; the function itself imposes no
// additional timeout on the chat stream (use newOpenAICompatChatClient for the
// 10-minute transport-level timeout).
func openAICompatChat(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	authHeaders map[string]string,
	req ChatRequest,
) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("openai-compat: model must not be empty")
	}

	bodyData, err := json.Marshal(openAIChatRequest{
		Model:    req.Model,
		Messages: buildOpenAIMessages(req),
		Stream:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("openai-compat: marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("openai-compat: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range authHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai-compat: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		// Attempt to parse structured OpenAI error body.
		var apiErr struct {
			Error openAIError `json:"error"`
		}
		if json.Unmarshal(errBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("openai-compat: HTTP %d: %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("openai-compat: HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		openAICompatStreamSSE(ctx, resp.Body, ch)
	}()
	return ch, nil
}

// openAICompatStreamSSE reads the SSE stream from an OpenAI-compatible
// /v1/chat/completions endpoint and emits StreamEvent values on ch.
//
// SSE format processed:
//
//	data: {"choices":[{"delta":{"content":"token"},"finish_reason":null}]}
//	data: [DONE]
//
// Each non-empty delta.content fragment emits a "chunk" event.
// "data: [DONE]" emits the final "done" event and returns.
// A "done" event is always the last event emitted, even on the error path.
func openAICompatStreamSSE(ctx context.Context, body io.Reader, ch chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	// Expand scanner buffer to handle large JSON payloads.
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

		// SSE comment or empty keep-alive line — skip.
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Only "data: " lines carry payload; ignore "event:", "id:", "retry:".
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")

		// The [DONE] sentinel marks the end of the stream.
		if payload == "[DONE]" {
			ch <- StreamEvent{Type: "done"}
			return
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// Non-fatal: skip malformed SSE payloads.
			ch <- StreamEvent{
				Type:    "error",
				Content: fmt.Sprintf("openai-compat: malformed SSE payload: %v", err),
			}
			continue
		}

		// Surface provider-level errors embedded in the SSE stream.
		if chunk.Error != nil && chunk.Error.Message != "" {
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("openai-compat: %s", chunk.Error.Message)}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Emit non-empty content fragments as "chunk" events.
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			ch <- StreamEvent{Type: "chunk", Content: chunk.Choices[0].Delta.Content}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamEvent{
			Type:    "error",
			Content: fmt.Sprintf("openai-compat: stream read error: %v", err),
		}
	}

	// Fallback "done" sentinel in case the stream ended without a [DONE] line.
	ch <- StreamEvent{Type: "done"}
}

// openAICompatListModels calls GET baseURL/v1/models with the provided auth
// headers and returns the list of available models. A 30-second deadline is
// applied via the context. The returned ModelInfo slice uses the model ID as
// both the ID and Name fields since /v1/models does not return display names.
func openAICompatListModels(
	ctx context.Context,
	baseURL string,
	authHeaders map[string]string,
) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := newOpenAICompatTestClient()
	url := strings.TrimRight(baseURL, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openai-compat: list models: create request: %w", err)
	}
	for k, v := range authHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai-compat: list models: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var apiErr struct {
			Error openAIError `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("openai-compat: list models: HTTP %d: %s", resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("openai-compat: list models: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai-compat: list models: decode response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelInfo{ID: m.ID, Name: m.ID})
	}
	return models, nil
}

// openAICompatTestConnection verifies that the endpoint at baseURL is reachable
// and accepts the provided credentials by calling GET /v1/models. Returns nil
// on success. A 30-second deadline is enforced internally.
func openAICompatTestConnection(
	ctx context.Context,
	baseURL string,
	authHeaders map[string]string,
) error {
	_, err := openAICompatListModels(ctx, baseURL, authHeaders)
	if err != nil {
		return fmt.Errorf("openai-compat: test connection: %w", err)
	}
	return nil
}
