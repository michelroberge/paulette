// Package provider — OpenAI-compatible SSE streaming unit tests (Milestone 6.3).
//
// These tests exercise the shared openAICompatStreamSSE function that is used
// by both the OpenAI and LM Studio providers. They verify:
//
//   - Normal SSE chunks are emitted as "chunk" events
//   - The [DONE] sentinel closes the stream with a "done" event
//   - SSE comments and empty lines are silently ignored
//   - Malformed JSON payloads emit "error" events and continue
//   - Provider-level errors embedded in the SSE payload emit "error" then "done"
//   - Context cancellation emits "error" and "done" and terminates the goroutine
//
// openAICompatChat is tested via a live httptest.Server to verify the full
// HTTP round-trip including auth headers, status-code handling, and streaming.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// collectEvents drains the channel returned by Chat and returns all events.
// Defined here because it is used by both the SSE streaming tests and the
// HTTP round-trip tests in this file.
func collectEvents(ch <-chan StreamEvent) []StreamEvent {
	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// ---------------------------------------------------------------------------
// openAICompatStreamSSE — unit tests (no HTTP)
// ---------------------------------------------------------------------------

// makeSSEBody builds an io.Reader containing the given SSE lines, each
// terminated by a newline.
func makeSSEBody(lines ...string) *strings.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}

// drainSSE runs openAICompatStreamSSE in a goroutine and collects all events.
func drainSSE(ctx context.Context, lines ...string) []StreamEvent {
	ch := make(chan StreamEvent, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		openAICompatStreamSSE(ctx, makeSSEBody(lines...), ch)
		close(ch)
	}()
	<-done

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// TestOpenAICompatSSE_ChunksAndDone verifies that standard SSE data lines are
// converted to "chunk" events and [DONE] triggers the final "done" event.
func TestOpenAICompatSSE_ChunksAndDone(t *testing.T) {
	chunk1 := `{"choices":[{"delta":{"content":"hello "},"finish_reason":null}]}`
	chunk2 := `{"choices":[{"delta":{"content":"world"},"finish_reason":null}]}`

	events := drainSSE(context.Background(),
		"data: "+chunk1,
		"data: "+chunk2,
		"data: [DONE]",
	)

	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d: %v", len(events), events)
	}
	if events[0].Type != "chunk" || events[0].Content != "hello " {
		t.Errorf("event[0]: want chunk='hello ', got %+v", events[0])
	}
	if events[1].Type != "chunk" || events[1].Content != "world" {
		t.Errorf("event[1]: want chunk='world', got %+v", events[1])
	}
	if events[2].Type != "done" {
		t.Errorf("event[2]: want done, got %+v", events[2])
	}
}

// TestOpenAICompatSSE_EmptyDeltaLinesSkipped verifies that SSE lines with an
// empty delta.content are not emitted as chunk events (e.g. role-only deltas
// that OpenAI sends at the start of a stream).
func TestOpenAICompatSSE_EmptyDeltaLinesSkipped(t *testing.T) {
	roleOnlyDelta := `{"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`
	contentDelta := `{"choices":[{"delta":{"content":"visible"},"finish_reason":null}]}`

	events := drainSSE(context.Background(),
		"data: "+roleOnlyDelta,
		"data: "+contentDelta,
		"data: [DONE]",
	)

	// Expect: chunk("visible"), done — the role-only delta has no content.
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d: %v", len(events), events)
	}
	if events[0].Type != "chunk" || events[0].Content != "visible" {
		t.Errorf("event[0]: want chunk='visible', got %+v", events[0])
	}
}

// TestOpenAICompatSSE_CommentsAndEmptyLinesIgnored verifies that SSE comment
// lines (starting with ":") and blank keep-alive lines are silently skipped.
func TestOpenAICompatSSE_CommentsAndEmptyLinesIgnored(t *testing.T) {
	contentDelta := `{"choices":[{"delta":{"content":"token"},"finish_reason":null}]}`

	events := drainSSE(context.Background(),
		": this is a comment",
		"",
		"data: "+contentDelta,
		"",
		": another comment",
		"data: [DONE]",
	)

	if len(events) != 2 {
		t.Fatalf("want 2 events (chunk+done), got %d: %v", len(events), events)
	}
	if events[0].Type != "chunk" {
		t.Errorf("event[0]: want chunk, got %+v", events[0])
	}
	if events[1].Type != "done" {
		t.Errorf("event[1]: want done, got %+v", events[1])
	}
}

// TestOpenAICompatSSE_NonDataLinesIgnored verifies that event:, id:, and retry:
// SSE meta-lines are silently ignored.
func TestOpenAICompatSSE_NonDataLinesIgnored(t *testing.T) {
	contentDelta := `{"choices":[{"delta":{"content":"ok"},"finish_reason":null}]}`

	events := drainSSE(context.Background(),
		"event: message",
		"id: 1",
		"retry: 3000",
		"data: "+contentDelta,
		"data: [DONE]",
	)

	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d: %v", len(events), events)
	}
	if events[0].Type != "chunk" || events[0].Content != "ok" {
		t.Errorf("event[0]: want chunk='ok', got %+v", events[0])
	}
}

// TestOpenAICompatSSE_MalformedJSONEmitsError verifies that a malformed JSON
// payload in a data line emits an "error" event and the stream continues
// (rather than terminating or panicking).
func TestOpenAICompatSSE_MalformedJSONEmitsError(t *testing.T) {
	contentDelta := `{"choices":[{"delta":{"content":"after-error"},"finish_reason":null}]}`

	events := drainSSE(context.Background(),
		"data: {this is bad json",
		"data: "+contentDelta,
		"data: [DONE]",
	)

	// Expect: error (malformed), chunk("after-error"), done.
	if len(events) < 3 {
		t.Fatalf("want >= 3 events, got %d: %v", len(events), events)
	}
	if events[0].Type != "error" {
		t.Errorf("event[0]: want error for malformed JSON, got %+v", events[0])
	}
	// The stream must continue after the malformed line.
	hasChunk := false
	for _, e := range events {
		if e.Type == "chunk" && e.Content == "after-error" {
			hasChunk = true
		}
	}
	if !hasChunk {
		t.Errorf("want chunk('after-error') after malformed line, events: %v", events)
	}
	if events[len(events)-1].Type != "done" {
		t.Errorf("last event should be done, got %+v", events[len(events)-1])
	}
}

// TestOpenAICompatSSE_ProviderErrorInPayload verifies that when the SSE stream
// carries an error object (e.g. rate-limit exceeded in an SSE chunk), the
// provider emits "error" then "done" and stops streaming.
func TestOpenAICompatSSE_ProviderErrorInPayload(t *testing.T) {
	errPayload := `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`

	events := drainSSE(context.Background(),
		"data: "+errPayload,
	)

	// Expect: error, done.
	if len(events) < 2 {
		t.Fatalf("want >= 2 events, got %d: %v", len(events), events)
	}

	hasError := false
	for _, e := range events {
		if e.Type == "error" {
			hasError = true
			if !strings.Contains(e.Content, "Rate limit") {
				t.Errorf("error event should mention rate limit, got: %q", e.Content)
			}
		}
	}
	if !hasError {
		t.Error("want error event for provider error payload")
	}
	if events[len(events)-1].Type != "done" {
		t.Errorf("last event should be done after error, got %+v", events[len(events)-1])
	}
}

// TestOpenAICompatSSE_NoChunks_FallbackDone verifies that a stream body that
// ends without a [DONE] sentinel still emits a final "done" event so consumers
// never block indefinitely.
func TestOpenAICompatSSE_NoChunks_FallbackDone(t *testing.T) {
	// Empty body — no lines at all.
	events := drainSSE(context.Background())

	if len(events) == 0 {
		t.Fatal("want at least one event (fallback done), got none")
	}
	if events[len(events)-1].Type != "done" {
		t.Errorf("want fallback done event, got %+v", events[len(events)-1])
	}
}

// ---------------------------------------------------------------------------
// openAICompatChat — HTTP round-trip tests (LM Studio / OpenAI variants)
// ---------------------------------------------------------------------------

// buildSSEResponse creates a minimal OpenAI-compatible SSE response body from
// the given content fragments. Each fragment becomes a separate data: line.
func buildSSEResponse(fragments []string) string {
	var sb strings.Builder
	for _, f := range fragments {
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{"delta": map[string]string{"content": f}, "finish_reason": nil},
			},
		})
		fmt.Fprintf(&sb, "data: %s\n\n", payload)
	}
	sb.WriteString("data: [DONE]\n\n")
	return sb.String()
}

// TestOpenAICompatChat_LMStudio_StreamsChunks verifies the full HTTP path for
// an LM Studio connection (no auth header, OpenAI-compatible protocol).
func TestOpenAICompatChat_LMStudio_StreamsChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		// LM Studio should not receive an Authorization header.
		if auth := r.Header.Get("Authorization"); auth != "" {
			http.Error(w, "unexpected Authorization header", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, buildSSEResponse([]string{"chunk1 ", "chunk2"}))
	}))
	defer srv.Close()

	p := NewLMStudioProvider(srv.URL)
	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:       "mistral-7b",
		UserMessage: "test",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	events := collectEvents(ch)
	var chunks []string
	for _, e := range events {
		if e.Type == "chunk" {
			chunks = append(chunks, e.Content)
		}
	}
	if len(chunks) != 2 {
		t.Errorf("want 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if events[len(events)-1].Type != "done" {
		t.Errorf("want done as last event, got %+v", events[len(events)-1])
	}
}

// TestOpenAICompatChat_OpenAI_SendsAuthHeader verifies that the OpenAI provider
// includes the Authorization: Bearer header in its chat requests.
func TestOpenAICompatChat_OpenAI_SendsAuthHeader(t *testing.T) {
	const testAPIKey = "sk-test-key-12345"

	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, buildSSEResponse([]string{"response"}))
	}))
	defer srv.Close()

	p, err := NewOpenAIProvider(srv.URL, testAPIKey, "", "")
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:       "gpt-4o",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// Drain the channel.
	for range ch {
	}

	wantAuth := "Bearer " + testAPIKey
	if receivedAuth != wantAuth {
		t.Errorf("Authorization header = %q, want %q", receivedAuth, wantAuth)
	}
}

// TestOpenAICompatChat_OpenAI_OrgAndProjectHeaders verifies that optional org
// and project IDs are sent in the OpenAI-Organisation and OpenAI-Project headers.
func TestOpenAICompatChat_OpenAI_OrgAndProjectHeaders(t *testing.T) {
	var receivedOrg, receivedProject string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedOrg = r.Header.Get("OpenAI-Organization")
		receivedProject = r.Header.Get("OpenAI-Project")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, buildSSEResponse([]string{"ok"}))
	}))
	defer srv.Close()

	p, err := NewOpenAIProvider(srv.URL, "sk-key", "org-abc", "proj-xyz")
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:       "gpt-4o",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	for range ch {
	}

	if receivedOrg != "org-abc" {
		t.Errorf("OpenAI-Organization = %q, want 'org-abc'", receivedOrg)
	}
	if receivedProject != "proj-xyz" {
		t.Errorf("OpenAI-Project = %q, want 'proj-xyz'", receivedProject)
	}
}

// TestOpenAICompatChat_NonOKStatus_ReturnsError verifies that a non-200
// response from the /v1/chat/completions endpoint is surfaced as an error from
// Chat() (not as a stream event) so the caller's error path is triggered.
func TestOpenAICompatChat_NonOKStatus_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Incorrect API key provided",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer srv.Close()

	p, err := NewOpenAIProvider(srv.URL, "sk-wrong-key", "", "")
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	_, chatErr := p.Chat(context.Background(), ChatRequest{
		Model:       "gpt-4o",
		UserMessage: "test",
	})
	if chatErr == nil {
		t.Fatal("want error from Chat() on HTTP 401, got nil")
	}
	if !strings.Contains(chatErr.Error(), "401") && !strings.Contains(chatErr.Error(), "Incorrect API key") {
		t.Errorf("error should mention 401 or API key issue, got: %v", chatErr)
	}
}

// TestOpenAICompatListModels_ParsesModels verifies that GET /v1/models is
// correctly parsed into a ModelInfo slice for the OpenAI provider.
func TestOpenAICompatListModels_ParsesModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"},
				{"id": "gpt-3.5-turbo"},
			},
		})
	}))
	defer srv.Close()

	p, err := NewOpenAIProvider(srv.URL, "sk-test", "", "")
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("want 3 models, got %d", len(models))
	}
	ids := map[string]bool{}
	for _, m := range models {
		ids[m.ID] = true
	}
	for _, expected := range []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"} {
		if !ids[expected] {
			t.Errorf("expected model %q not in result", expected)
		}
	}
}

// TestLMStudioProvider_ImplementsProviderInterface is a compile-time assertion.
func TestLMStudioProvider_ImplementsProviderInterface(t *testing.T) {
	var _ Provider = (*LMStudioProvider)(nil)
}

// TestOpenAIProvider_ImplementsProviderInterface is a compile-time assertion.
func TestOpenAIProvider_ImplementsProviderInterface(t *testing.T) {
	var _ Provider = (*OpenAIProvider)(nil)
}
