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

const geminiDefaultBaseURL = "https://generativelanguage.googleapis.com"

// GeminiProvider implements Provider using the Google Gemini API.
// Chat streams via POST /v1beta/models/{model}:streamGenerateContent?key={apiKey}&alt=sse.
// The API key is passed as a query parameter (not in an Authorization header).
//
// Streaming format: the &alt=sse query parameter requests Server-Sent Events
// (SSE) format. The API returns lines of the form:
//
//	data: {"candidates":[{"content":{"parts":[{"text":"..."}],"role":"model"}}]}
//
// Each "data:" line is parsed as a geminiStreamChunk and the text of the first
// candidate's first part is emitted as a "chunk" event. Blank lines and
// non-data lines (e.g. "event:", "id:") are skipped.
type GeminiProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewGeminiProvider returns a GeminiProvider. If baseURL is empty the
// production Gemini endpoint is used. A nil apiKey is valid (the probe
// will simply fail with a 403 from the API).
func NewGeminiProvider(baseURL, apiKey string) *GeminiProvider {
	if baseURL == "" {
		baseURL = geminiDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &GeminiProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			// Streaming chat can run for many minutes; test/model-list ops
			// use their own context with a 30 s deadline.
			Timeout: 10 * time.Minute,
		},
	}
}

// ── Gemini request/response types ────────────────────────────────────────────

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiChatRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiStreamChunk struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiModel struct {
	Name        string `json:"name"`        // e.g. "models/gemini-1.5-pro"
	DisplayName string `json:"displayName"` // e.g. "Gemini 1.5 Pro"
}

type geminiListModelsResponse struct {
	Models []geminiModel `json:"models"`
}

// ── Provider interface ────────────────────────────────────────────────────────

// Chat sends the conversation to the Gemini streamGenerateContent endpoint and
// converts the SSE response stream into a channel of StreamEvent values.
//
// The Gemini API requires:
//   - Contents: alternating user/model turns (must start with "user")
//   - SystemInstruction: optional, contains the system prompt
//   - API key in the "key" query parameter
//
// The pipeline's XML envelope format is preserved by embedding the same
// system prompt used by all other providers.
func (p *GeminiProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("gemini: model must not be empty")
	}

	body, err := p.buildChatRequest(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}

	url := fmt.Sprintf(
		"%s/v1beta/models/%s:streamGenerateContent?key=%s&alt=sse",
		p.baseURL, req.Model, p.apiKey,
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("gemini: unexpected status %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.streamResponse(ctx, resp.Body, ch)
	}()
	return ch, nil
}

// TestConnection probes the model-list endpoint to verify the API key and
// network connectivity. A 30-second deadline is applied via the context.
func (p *GeminiProvider) TestConnection(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := p.fetchModels(ctx)
	if err != nil {
		return fmt.Errorf("gemini: test connection: %w", err)
	}
	return nil
}

// ListModels returns all Gemini models available to the configured API key
// by querying GET /v1beta/models?key={apiKey}.
func (p *GeminiProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models, err := p.fetchModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("gemini: list models: %w", err)
	}
	return models, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// buildChatRequest converts a ChatRequest into the Gemini JSON wire format.
//
// Gemini requires:
//   - Contents must contain only "user" and "model" roles.
//   - The turn order must alternate; it must start with "user".
//   - If only a system prompt exists, it goes in SystemInstruction; the
//     first content turn must still be a user message.
func (p *GeminiProvider) buildChatRequest(req ChatRequest) ([]byte, error) {
	var contents []geminiContent

	// Convert history turns (model.Message.Role is "user" or "assistant").
	for _, msg := range req.History {
		role := msg.Role
		if role == "assistant" {
			role = "model" // Gemini uses "model", not "assistant"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	// Append the new user message.
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: req.UserMessage}},
	})

	gr := geminiChatRequest{Contents: contents}

	// System prompt becomes SystemInstruction (supported on Gemini 1.5+).
	if req.SystemPrompt != "" {
		gr.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.SystemPrompt}},
		}
	}

	return json.Marshal(gr)
}

// streamResponse reads the SSE response body from the Gemini API and emits
// StreamEvent values on ch. The Gemini streaming endpoint returns lines in
// SSE format (via the &alt=sse query parameter):
//
//	data: {"candidates":[{"content":{"parts":[{"text":"..."}],"role":"model"}}]}
//
// Lines that are not "data:" prefixed (e.g. blank lines, "event:", "id:")
// are skipped. Each data line is unmarshalled as a geminiStreamChunk and
// the text of the first candidate's first part is emitted as a "chunk" event.
//
// A "done" event is always emitted as the last event, even on error, so that
// callers draining the channel until "done" never block indefinitely.
func (p *GeminiProvider) streamResponse(ctx context.Context, body io.Reader, ch chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	// Increase the scanner buffer for large response lines.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		// Check for context cancellation between lines.
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: "error", Content: ctx.Err().Error()}
			ch <- StreamEvent{Type: "done"}
			return
		default:
		}

		line := scanner.Text()

		// SSE format: "data: {...}" lines carry content; blank lines separate events.
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if jsonStr == "" {
			continue
		}

		var chunk geminiStreamChunk
		if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
			// Emit a non-fatal error event so callers have diagnostic info,
			// then continue processing subsequent lines rather than aborting.
			ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("gemini: malformed chunk: %v", err)}
			continue
		}

		text := extractGeminiText(chunk)
		if text != "" {
			ch <- StreamEvent{Type: "chunk", Content: text}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("gemini: stream read error: %v", err)}
		ch <- StreamEvent{Type: "done"}
		return
	}

	ch <- StreamEvent{Type: "done"}
}

// extractGeminiText safely extracts candidates[0].content.parts[0].text.
// Returns an empty string if the path does not exist.
func extractGeminiText(chunk geminiStreamChunk) string {
	if len(chunk.Candidates) == 0 {
		return ""
	}
	parts := chunk.Candidates[0].Content.Parts
	if len(parts) == 0 {
		return ""
	}
	return parts[0].Text
}

// fetchModels calls GET /v1beta/models?key={apiKey} and parses the response.
// It returns a ModelInfo slice ordered as returned by the API. The caller is
// responsible for applying a deadline via ctx.
func (p *GeminiProvider) fetchModels(ctx context.Context) ([]ModelInfo, error) {
	url := fmt.Sprintf("%s/v1beta/models?key=%s", p.baseURL, p.apiKey)

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

	var result geminiListModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		// The API returns names like "models/gemini-1.5-pro".
		// Strip the "models/" prefix to get the bare model ID used in API calls.
		id := strings.TrimPrefix(m.Name, "models/")
		displayName := m.DisplayName
		if displayName == "" {
			displayName = id
		}
		models = append(models, ModelInfo{
			ID:   id,
			Name: displayName,
		})
	}
	return models, nil
}
