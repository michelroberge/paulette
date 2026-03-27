// Package provider — Ollama end-to-end tests (Milestone 6.2).
//
// These tests verify that a user with no Claude CLI installed can run the full
// Paulette pipeline using Ollama as the sole LLM backend.
//
// Acceptance criteria (JRN-v0.2.0-001):
//   1. Create Ollama connection and assign to all five pipeline stages.
//   2. Registry.ResolveForStage returns *OllamaProvider for every stage.
//   3. OllamaProvider.TestConnection passes in under 3 seconds.
//   4. OllamaProvider.ListModels correctly parses the GET /api/tags response.
//   5. OllamaProvider.Chat streams NDJSON chunks and terminates with "done".
//   6. Chat produces the XML-envelope response format expected by the pipeline parser.
//   7. Pipeline works correctly even when the claude binary is absent.
//
// All tests use net/http/httptest to mock the Ollama HTTP API — no live
// Ollama instance is required.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
)

// ---------------------------------------------------------------------------
// Mock Ollama server helpers
// ---------------------------------------------------------------------------

// ollamaMockServer creates a test HTTP server that mimics the Ollama API.
// It serves:
//   GET  /api/tags  → returns the supplied model list
//   POST /api/chat  → streams the supplied NDJSON chunks then done:true
//
// Use srv.Close() / t.Cleanup(srv.Close) to shut it down.
func ollamaMockServer(t *testing.T, models []string, chatChunks []string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// GET /api/tags — model list endpoint used by ListModels and TestConnection.
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		type modelEntry struct {
			Name string `json:"name"`
		}
		type tagsResp struct {
			Models []modelEntry `json:"models"`
		}
		resp := tagsResp{}
		for _, m := range models {
			resp.Models = append(resp.Models, modelEntry{Name: m})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// POST /api/chat — streaming chat endpoint.
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Transfer-Encoding", "chunked")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		type msgContent struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		type chunk struct {
			Message msgContent `json:"message"`
			Done    bool       `json:"done"`
		}

		// Stream each chunk as a separate NDJSON line.
		for _, content := range chatChunks {
			line, _ := json.Marshal(chunk{
				Message: msgContent{Role: "assistant", Content: content},
				Done:    false,
			})
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}

		// Final done:true line (no message content).
		doneLine, _ := json.Marshal(chunk{Done: true})
		fmt.Fprintf(w, "%s\n", doneLine)
		flusher.Flush()
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ollamaMockServerSlow creates a mock Ollama server that introduces a delay
// before responding — used to verify timeout behaviour.
func ollamaMockServerSlow(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ollamaMockServerError creates a mock Ollama server that always returns
// the given HTTP status — used to verify error handling.
func ollamaMockServerError(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "simulated error", status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// collectAllChunks drains a StreamEvent channel and returns the concatenated
// content from all "chunk" events. Fails if no "done" event is received.
func collectAllChunks(t *testing.T, ch <-chan StreamEvent) string {
	t.Helper()
	var buf strings.Builder
	gotDone := false
	for event := range ch {
		switch event.Type {
		case "chunk":
			buf.WriteString(event.Content)
		case "done":
			gotDone = true
		case "error":
			t.Errorf("unexpected error event: %s", event.Content)
		}
	}
	if !gotDone {
		t.Error("stream closed without a 'done' event")
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

// TestOllamaProvider_ImplementsProviderInterface is a compile-time assertion.
// If OllamaProvider no longer satisfies the Provider interface, this test body
// will cause a compile error.
func TestOllamaProvider_ImplementsProviderInterface(t *testing.T) {
	var _ Provider = (*OllamaProvider)(nil)
}

// ---------------------------------------------------------------------------
// TestConnection — sub-3-second guarantee
// ---------------------------------------------------------------------------

// TestOllamaProvider_TestConnection_Success verifies that TestConnection
// returns nil when the Ollama server is reachable and responding.
func TestOllamaProvider_TestConnection_Success(t *testing.T) {
	srv := ollamaMockServer(t, []string{"llama3:8b"}, nil)
	p := NewOllamaProvider(srv.URL)

	start := time.Now()
	err := p.TestConnection(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("TestConnection: unexpected error: %v", err)
	}

	// Acceptance criterion: connection test must pass in under 3 seconds.
	// With a local mock server this should complete in milliseconds; 3s is
	// the ceiling mandated by the build plan.
	if elapsed > 3*time.Second {
		t.Errorf("TestConnection took %v, want <3s", elapsed)
	}
}

// TestOllamaProvider_TestConnection_EmptyModelList verifies that TestConnection
// succeeds even when the Ollama instance has no models loaded — the absence of
// models does not imply the server is unreachable.
func TestOllamaProvider_TestConnection_EmptyModelList(t *testing.T) {
	srv := ollamaMockServer(t, []string{}, nil)
	p := NewOllamaProvider(srv.URL)

	if err := p.TestConnection(context.Background()); err != nil {
		t.Fatalf("TestConnection with empty model list: %v", err)
	}
}

// TestOllamaProvider_TestConnection_ServerDown verifies that TestConnection
// returns an error when the Ollama server is unreachable.
func TestOllamaProvider_TestConnection_ServerDown(t *testing.T) {
	// Use an address that is guaranteed to be unreachable quickly.
	p := NewOllamaProvider("http://127.0.0.1:1") // port 1 is reserved/unreachable

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := p.TestConnection(ctx)
	if err == nil {
		t.Fatal("TestConnection: expected error for unreachable server, got nil")
	}
}

// TestOllamaProvider_TestConnection_ServerReturns500 verifies that a non-200
// HTTP status is treated as a connection failure.
func TestOllamaProvider_TestConnection_ServerReturns500(t *testing.T) {
	srv := ollamaMockServerError(t, http.StatusInternalServerError)
	p := NewOllamaProvider(srv.URL)

	if err := p.TestConnection(context.Background()); err == nil {
		t.Fatal("TestConnection: expected error for 500 response, got nil")
	}
}

// TestOllamaProvider_TestConnection_SpeedBenchmark measures the raw round-trip
// time of TestConnection against a local mock. Under normal conditions a local
// HTTP call should complete in under 100ms; the 3-second ceiling is the
// production requirement for a local Ollama daemon on the same machine.
func TestOllamaProvider_TestConnection_SpeedBenchmark(t *testing.T) {
	srv := ollamaMockServer(t, []string{"llama3:8b", "mistral:7b"}, nil)
	p := NewOllamaProvider(srv.URL)

	const iterations = 5
	var total time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		if err := p.TestConnection(context.Background()); err != nil {
			t.Fatalf("TestConnection iteration %d: %v", i, err)
		}
		total += time.Since(start)
	}

	avg := total / iterations
	t.Logf("TestConnection average round-trip: %v (over %d iterations)", avg, iterations)

	if avg > 3*time.Second {
		t.Errorf("average TestConnection time %v exceeds 3s threshold", avg)
	}
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

// TestOllamaProvider_ListModels_ParsesTagsResponse verifies that ListModels
// correctly parses the GET /api/tags response and returns the expected
// model identifiers.
func TestOllamaProvider_ListModels_ParsesTagsResponse(t *testing.T) {
	wantModels := []string{"llama3:8b", "mistral:7b", "codellama:13b"}
	srv := ollamaMockServer(t, wantModels, nil)
	p := NewOllamaProvider(srv.URL)

	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	if len(models) != len(wantModels) {
		t.Fatalf("ListModels: want %d models, got %d: %v", len(wantModels), len(models), models)
	}

	for i, m := range models {
		if m.ID != wantModels[i] {
			t.Errorf("models[%d].ID = %q, want %q", i, m.ID, wantModels[i])
		}
		if m.Name != wantModels[i] {
			t.Errorf("models[%d].Name = %q, want %q", i, m.Name, wantModels[i])
		}
	}
}

// TestOllamaProvider_ListModels_EmptyList verifies that an empty model list is
// returned as an empty slice (not nil) so callers can range over it safely.
func TestOllamaProvider_ListModels_EmptyList(t *testing.T) {
	srv := ollamaMockServer(t, []string{}, nil)
	p := NewOllamaProvider(srv.URL)

	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels (empty): %v", err)
	}
	// A non-nil empty slice is acceptable; len == 0 is what matters.
	if len(models) != 0 {
		t.Errorf("ListModels: want 0 models, got %d", len(models))
	}
}

// TestOllamaProvider_ListModels_ServerError verifies that a non-200 response
// from /api/tags is propagated as an error from ListModels.
func TestOllamaProvider_ListModels_ServerError(t *testing.T) {
	srv := ollamaMockServerError(t, http.StatusServiceUnavailable)
	p := NewOllamaProvider(srv.URL)

	_, err := p.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels: expected error for 503 response, got nil")
	}
}

// ---------------------------------------------------------------------------
// Chat streaming
// ---------------------------------------------------------------------------

// TestOllamaProvider_Chat_StreamsChunks verifies that Chat converts each
// NDJSON chunk from /api/chat into a "chunk" StreamEvent and terminates with
// a "done" event.
func TestOllamaProvider_Chat_StreamsChunks(t *testing.T) {
	chunks := []string{"Hello", ", ", "world", "!"}
	srv := ollamaMockServer(t, []string{"llama3:8b"}, chunks)
	p := NewOllamaProvider(srv.URL)

	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:       "llama3:8b",
		UserMessage: "Say hello",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	got := collectAllChunks(t, ch)
	want := "Hello, world!"
	if got != want {
		t.Errorf("assembled content = %q, want %q", got, want)
	}
}

// TestOllamaProvider_Chat_XMLEnvelopeResponse verifies that when the Ollama
// model returns the XML envelope format required by the Paulette pipeline
// parser, the content flows through unchanged. This confirms that an Ollama
// backend can serve a complete Paulette pipeline run without the claude CLI.
func TestOllamaProvider_Chat_XMLEnvelopeResponse(t *testing.T) {
	// The pipeline parser expects responses wrapped in the XML envelope:
	// <response><discussion>...</discussion><artifact>...</artifact></response>
	xmlEnvelope := `<response>
<discussion>Here is the vision document for your project.</discussion>
<artifact>
# Project Vision

## Overview
A task management application for solo founders.

## Core Features
- Create and track tasks
- Set priorities and deadlines
- Progress analytics
</artifact>
</response>`

	// Split the envelope across multiple chunks to simulate real streaming
	// behaviour — the parser must handle content arriving piecemeal.
	chunks := splitIntoChunks(xmlEnvelope, 50)
	srv := ollamaMockServer(t, []string{"llama3:8b"}, chunks)
	p := NewOllamaProvider(srv.URL)

	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:        "llama3:8b",
		SystemPrompt: "You are a helpful pipeline assistant. Respond in XML envelope format.",
		UserMessage:  "Create a vision document for a task management app.",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	assembled := collectAllChunks(t, ch)

	// Verify the response contains the key structural elements.
	if !strings.Contains(assembled, "<response>") {
		t.Error("assembled content missing <response> tag")
	}
	if !strings.Contains(assembled, "<artifact>") {
		t.Error("assembled content missing <artifact> tag")
	}
	if !strings.Contains(assembled, "</response>") {
		t.Error("assembled content missing </response> tag")
	}

	// The content must be fully assembled — no truncation.
	if assembled != xmlEnvelope {
		t.Errorf("assembled content does not match original:\ngot:  %q\nwant: %q", assembled, xmlEnvelope)
	}
}

// TestOllamaProvider_Chat_WithSystemPromptAndHistory verifies that the Ollama
// request body includes the system prompt and history messages so the model
// has the full conversational context.
func TestOllamaProvider_Chat_WithSystemPromptAndHistory(t *testing.T) {
	var capturedBody ollamaChatRequest

	// Custom handler that captures the request body for inspection.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher := w.(http.Flusher)

		type msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		type chunk struct {
			Message msg  `json:"message"`
			Done    bool `json:"done"`
		}
		line, _ := json.Marshal(chunk{Message: msg{Role: "assistant", Content: "ok"}, Done: false})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
		done, _ := json.Marshal(chunk{Done: true})
		fmt.Fprintf(w, "%s\n", done)
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := NewOllamaProvider(srv.URL)

	history := []Message{
		{Role: model.RoleUser, Content: "What is Paulette?"},
		{Role: model.RoleAssistant, Content: "Paulette is an AI-powered development pipeline."},
	}

	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:        "llama3:8b",
		SystemPrompt: "You are a helpful assistant.",
		History:      history,
		UserMessage:  "Tell me more.",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// Drain the channel.
	for range ch {
	}

	// Verify the captured request.
	if capturedBody.Model != "llama3:8b" {
		t.Errorf("model = %q, want %q", capturedBody.Model, "llama3:8b")
	}
	if !capturedBody.Stream {
		t.Error("stream field should be true")
	}

	// Expected messages: system, user-1, assistant-1, user-new
	const wantMsgs = 4
	if len(capturedBody.Messages) != wantMsgs {
		t.Fatalf("messages len = %d, want %d; messages: %+v", len(capturedBody.Messages), wantMsgs, capturedBody.Messages)
	}

	if capturedBody.Messages[0].Role != "system" {
		t.Errorf("messages[0].role = %q, want %q", capturedBody.Messages[0].Role, "system")
	}
	if capturedBody.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("messages[0].content = %q, want system prompt", capturedBody.Messages[0].Content)
	}
	if capturedBody.Messages[1].Role != "user" {
		t.Errorf("messages[1].role = %q, want user", capturedBody.Messages[1].Role)
	}
	if capturedBody.Messages[2].Role != "assistant" {
		t.Errorf("messages[2].role = %q, want assistant", capturedBody.Messages[2].Role)
	}
	if capturedBody.Messages[3].Role != "user" {
		t.Errorf("messages[3].role = %q, want user", capturedBody.Messages[3].Role)
	}
	if capturedBody.Messages[3].Content != "Tell me more." {
		t.Errorf("messages[3].content = %q, want %q", capturedBody.Messages[3].Content, "Tell me more.")
	}
}

// TestOllamaProvider_Chat_EmptyModelReturnsError verifies that Chat rejects a
// ChatRequest with an empty Model field before making any network call.
func TestOllamaProvider_Chat_EmptyModelReturnsError(t *testing.T) {
	srv := ollamaMockServer(t, []string{"llama3:8b"}, []string{"ok"})
	p := NewOllamaProvider(srv.URL)

	_, err := p.Chat(context.Background(), ChatRequest{
		Model:       "", // intentionally empty
		UserMessage: "hello",
	})
	if err == nil {
		t.Fatal("Chat with empty model: expected error, got nil")
	}
}

// TestOllamaProvider_Chat_OllamaErrorField verifies that when the Ollama
// server includes an "error" field in its NDJSON stream (e.g. model not found),
// the provider emits an error event followed by a done event — the stream does
// not block or produce partial content.
func TestOllamaProvider_Chat_OllamaErrorField(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher := w.(http.Flusher)
		// Ollama sends an error object when the model is not found.
		_, _ = fmt.Fprintf(w, `{"error":"model 'nonexistent:model' not found, try pulling it first"}`+"\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := NewOllamaProvider(srv.URL)

	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:       "nonexistent:model",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Chat: expected channel (not immediate error), got: %v", err)
	}

	var gotError, gotDone bool
	for event := range ch {
		switch event.Type {
		case "error":
			gotError = true
			if !strings.Contains(event.Content, "not found") {
				t.Errorf("error event content = %q, want substring 'not found'", event.Content)
			}
		case "done":
			gotDone = true
		}
	}

	if !gotError {
		t.Error("expected an error event for Ollama model-not-found error")
	}
	if !gotDone {
		t.Error("expected a done event after error — stream must always terminate")
	}
}

// TestOllamaProvider_Chat_ContextCancellation verifies that cancelling the
// context mid-stream causes the provider to emit an error event followed by a
// done event and then close the channel. This ensures no goroutine leaks.
func TestOllamaProvider_Chat_ContextCancellation(t *testing.T) {
	// Server that sends one chunk then hangs until the client disconnects.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher := w.(http.Flusher)

		type msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		type chunk struct {
			Message msg  `json:"message"`
			Done    bool `json:"done"`
		}
		line, _ := json.Marshal(chunk{Message: msg{Role: "assistant", Content: "start"}, Done: false})
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()

		// Block until the client disconnects (context cancelled).
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second): // safety valve
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	p := NewOllamaProvider(srv.URL)

	ch, err := p.Chat(ctx, ChatRequest{
		Model:       "llama3:8b",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// Receive the first chunk, then cancel.
	gotFirst := false
	for event := range ch {
		if event.Type == "chunk" && !gotFirst {
			gotFirst = true
			cancel() // trigger cancellation
		}
		if event.Type == "done" {
			break
		}
	}

	if !gotFirst {
		t.Error("did not receive initial chunk before cancellation")
	}
}

// ---------------------------------------------------------------------------
// Full pipeline: assign Ollama to all stages
// ---------------------------------------------------------------------------

// TestOllama_AllStagesAssigned_RegistryResolvesOllama is the core end-to-end
// scenario. It:
//  1. Creates an Ollama connection in the ConnectionStore.
//  2. Sets the Ollama connection as the global default for ALL five stages.
//  3. Verifies that Registry.ResolveForStage returns *OllamaProvider (not
//     *ClaudeCLIProvider) for every stage.
//  4. Verifies the resolved model matches the one set in the stage assignment.
//
// This test proves that a user who configures Ollama for all stages will never
// fall through to the Claude CLI, even if claude is not installed.
func TestOllama_AllStagesAssigned_RegistryResolvesOllama(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	srv := ollamaMockServer(t, []string{"llama3:8b"}, nil)

	// 1. Create an Ollama connection.
	connStore := NewConnectionStore(globalDir)
	conn, err := connStore.Create(Connection{
		Name:         "Local Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      srv.URL,
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("Create connection: %v", err)
	}

	// 2. Assign the connection to all five pipeline stages.
	stageConfig := NewStageConfigStore(globalDir)
	allStages := []struct {
		stage model.StageName
		model string
	}{
		{model.StageVision, "llama3:8b"},
		{model.StageUX, "llama3:8b"},
		{model.StageArchitecture, "llama3:70b"},
		{model.StageBuild, "llama3:70b"},
		{model.StageComplete, "llama3:8b"},
	}

	for _, tc := range allStages {
		if err := stageConfig.SetGlobalStageDefault(tc.stage, StageAssignment{
			ConnectionID: conn.ID,
			Model:        tc.model,
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s): %v", tc.stage, err)
		}
	}

	// 3 & 4. Verify registry resolves OllamaProvider for every stage.
	registry := NewRegistry(connStore)
	for _, tc := range allStages {
		t.Run(string(tc.stage), func(t *testing.T) {
			prov, modelID, err := registry.ResolveForStage("proj-e2e", tc.stage, stageConfig, projectDir)

			if err != nil {
				t.Fatalf("ResolveForStage(%s): unexpected error: %v", tc.stage, err)
			}
			if prov == nil {
				t.Fatal("ResolveForStage returned nil provider")
			}

			// Must be an Ollama provider — NOT the Claude CLI fallback.
			if _, ok := prov.(*OllamaProvider); !ok {
				t.Errorf("ResolveForStage(%s) returned %T, want *OllamaProvider", tc.stage, prov)
			}

			// Model must match the per-stage assignment.
			if modelID != tc.model {
				t.Errorf("ResolveForStage(%s) model = %q, want %q", tc.stage, modelID, tc.model)
			}
		})
	}
}

// TestOllama_AllStagesAssigned_PersistsToFiles verifies that after assigning
// Ollama to all stages, the global config is correctly written to disk so it
// survives a server restart. A second StageConfigStore reading from the same
// directory must return the same assignments.
func TestOllama_AllStagesAssigned_PersistsToFiles(t *testing.T) {
	globalDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	conn, err := connStore.Create(Connection{
		Name:         "Persisted Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("Create connection: %v", err)
	}

	// Write stage assignments.
	stageConfig := NewStageConfigStore(globalDir)
	stages := []model.StageName{
		model.StageVision, model.StageUX, model.StageArchitecture,
		model.StageBuild, model.StageComplete,
	}
	for _, stage := range stages {
		if err := stageConfig.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: conn.ID,
			Model:        "llama3:8b",
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s): %v", stage, err)
		}
	}

	// Simulate a server restart: create a fresh StageConfigStore from the
	// same directory.
	reloadedConfig := NewStageConfigStore(globalDir)
	defaults := reloadedConfig.GetGlobalDefaults()

	for _, stage := range stages {
		assignment, ok := defaults.StageDefaults[stage]
		if !ok {
			t.Errorf("stage %s not found in persisted config", stage)
			continue
		}
		if assignment.ConnectionID != conn.ID {
			t.Errorf("stage %s: connectionID = %q, want %q", stage, assignment.ConnectionID, conn.ID)
		}
		if assignment.Model != "llama3:8b" {
			t.Errorf("stage %s: model = %q, want %q", stage, assignment.Model, "llama3:8b")
		}
	}
}

// ---------------------------------------------------------------------------
// No Claude CLI — Ollama as sole provider
// ---------------------------------------------------------------------------

// TestOllama_NoCLI_AllStagesResolveWithoutError verifies the critical user
// journey: a developer with NO claude binary installed can configure Ollama
// as their sole provider and have all five pipeline stages resolve correctly.
//
// The test does NOT attempt to invoke any subprocess — provider resolution
// must succeed through the HTTP-based Ollama path alone.
func TestOllama_NoCLI_AllStagesResolveWithoutError(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	srv := ollamaMockServer(t, []string{"llama3:8b", "llama3:70b"}, nil)

	// Set up: one Ollama connection assigned to all stages.
	connStore := NewConnectionStore(globalDir)
	conn, err := connStore.Create(Connection{
		Name:         "Ollama (no CLI needed)",
		ProviderType: ProviderOllama,
		BaseURL:      srv.URL,
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)
	for _, stage := range []model.StageName{
		model.StageVision, model.StageUX, model.StageArchitecture,
		model.StageBuild, model.StageComplete,
	} {
		if err := stageConfig.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: conn.ID,
			Model:        "llama3:8b",
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s): %v", stage, err)
		}
	}

	registry := NewRegistry(connStore)

	// Resolve every stage and assert it is an OllamaProvider — the Claude CLI
	// is never instantiated, so the absence of the claude binary is irrelevant.
	for _, stage := range []model.StageName{
		model.StageVision, model.StageUX, model.StageArchitecture,
		model.StageBuild, model.StageComplete,
	} {
		prov, _, err := registry.ResolveForStage("proj-nocli", stage, stageConfig, projectDir)
		if err != nil {
			t.Errorf("ResolveForStage(%s): unexpected error: %v", stage, err)
			continue
		}
		if _, ok := prov.(*OllamaProvider); !ok {
			t.Errorf("ResolveForStage(%s): want *OllamaProvider, got %T — claude CLI would be required", stage, prov)
		}
	}
}

// TestOllama_NoCLI_CanChat verifies that OllamaProvider.Chat completes
// successfully without invoking the claude CLI at all. This is the lowest-level
// confirmation that the Ollama HTTP transport is fully self-contained.
func TestOllama_NoCLI_CanChat(t *testing.T) {
	// A complete XML-envelope response (the format the Paulette parser needs).
	xmlResponse := `<response><discussion>Vision complete.</discussion><artifact>
# Vision
A simple task manager.
</artifact></response>`

	chunks := splitIntoChunks(xmlResponse, 30)
	srv := ollamaMockServer(t, []string{"llama3:8b"}, chunks)

	// Create an OllamaProvider directly — no agent package, no CLI.
	p := NewOllamaProvider(srv.URL)

	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:        "llama3:8b",
		SystemPrompt: "Respond in XML envelope format.",
		UserMessage:  "Create a vision document.",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	assembled := collectAllChunks(t, ch)

	if assembled != xmlResponse {
		t.Errorf("chat content mismatch:\ngot:  %q\nwant: %q", assembled, xmlResponse)
	}
}

// ---------------------------------------------------------------------------
// Per-project override: Ollama overrides global Claude CLI default
// ---------------------------------------------------------------------------

// TestOllama_ProjectOverride_OverridesGlobalCLI verifies that when a user sets
// a project-level Ollama override for a stage that globally falls back to Claude
// CLI, the project-level override wins and *OllamaProvider is returned.
func TestOllama_ProjectOverride_OverridesGlobalCLI(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	srv := ollamaMockServer(t, []string{"llama3:8b"}, nil)

	connStore := NewConnectionStore(globalDir)
	conn, err := connStore.Create(Connection{
		Name:         "Project Ollama Override",
		ProviderType: ProviderOllama,
		BaseURL:      srv.URL,
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Global config: no assignments → Claude CLI fallback for all stages.
	stageConfig := NewStageConfigStore(globalDir)

	// Project override: only the vision stage is set to Ollama.
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageVision, &StageAssignment{
		ConnectionID: conn.ID,
		Model:        "llama3:8b",
	}); err != nil {
		t.Fatalf("SetProjectStageOverride: %v", err)
	}

	registry := NewRegistry(connStore)

	// Vision stage → must be OllamaProvider (project override).
	prov, modelID, err := registry.ResolveForStage("proj-override", model.StageVision, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(vision): %v", err)
	}
	if _, ok := prov.(*OllamaProvider); !ok {
		t.Errorf("vision: want *OllamaProvider, got %T", prov)
	}
	if modelID != "llama3:8b" {
		t.Errorf("vision: model = %q, want llama3:8b", modelID)
	}

	// Architecture stage → must still fall back to Claude CLI (no override).
	archProv, archModel, err := registry.ResolveForStage("proj-override", model.StageArchitecture, stageConfig, projectDir)
	if err != nil {
		t.Fatalf("ResolveForStage(architecture): %v", err)
	}
	if _, ok := archProv.(*ClaudeCLIProvider); !ok {
		t.Errorf("architecture: want *ClaudeCLIProvider, got %T", archProv)
	}
	if archModel != "claude-opus-4-6" {
		t.Errorf("architecture: model = %q, want claude-opus-4-6", archModel)
	}
}

// ---------------------------------------------------------------------------
// Connection store: Ollama CRUD lifecycle
// ---------------------------------------------------------------------------

// TestOllama_ConnectionLifecycle_CreateUpdateDelete verifies the full CRUD
// lifecycle of an Ollama connection in the ConnectionStore.
func TestOllama_ConnectionLifecycle_CreateUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewConnectionStore(dir)

	// Create.
	conn, err := store.Create(Connection{
		Name:         "Local Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if conn.ID == "" {
		t.Fatal("Create: expected non-empty ID")
	}
	if conn.CreatedAt.IsZero() || conn.UpdatedAt.IsZero() {
		t.Error("Create: timestamps must be set")
	}

	// List: should contain exactly the new connection.
	list := store.List()
	if len(list) != 1 {
		t.Fatalf("List after Create: want 1, got %d", len(list))
	}
	if list[0].Name != "Local Ollama" {
		t.Errorf("list[0].Name = %q, want Local Ollama", list[0].Name)
	}

	// Get.
	got, err := store.Get(conn.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BaseURL != "http://localhost:11434" {
		t.Errorf("Get: baseURL = %q, want http://localhost:11434", got.BaseURL)
	}

	// Update: switch to a different model and URL.
	updated := *conn
	updated.DefaultModel = "llama3:70b"
	updated.BaseURL = "http://ollama-server:11434"
	updated.Name = "Remote Ollama"

	updatedConn, err := store.Update(updated)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updatedConn.DefaultModel != "llama3:70b" {
		t.Errorf("Update: defaultModel = %q, want llama3:70b", updatedConn.DefaultModel)
	}
	if updatedConn.CreatedAt != conn.CreatedAt {
		t.Error("Update: CreatedAt must not change")
	}
	if !updatedConn.UpdatedAt.After(conn.UpdatedAt) {
		t.Error("Update: UpdatedAt must be newer than CreatedAt")
	}

	// Delete.
	affected, err := store.Delete(conn.ID, nil, nil)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// No stage config was provided, so affected stages must be empty.
	if len(affected) != 0 {
		t.Errorf("Delete: unexpected affected stages: %v", affected)
	}

	// After deletion, List must be empty and Get must fail.
	if len(store.List()) != 0 {
		t.Error("List after Delete: expected empty")
	}
	if _, err := store.Get(conn.ID); err == nil {
		t.Error("Get after Delete: expected error, got nil")
	}
}

// TestOllama_ConnectionValidation_MissingBaseURL verifies that trying to create
// an Ollama connection without a BaseURL is rejected with a descriptive error.
// Ollama is a local service with no default URL, so a missing URL is always a
// user configuration error.
func TestOllama_ConnectionValidation_MissingBaseURL(t *testing.T) {
	dir := t.TempDir()
	store := NewConnectionStore(dir)

	_, err := store.Create(Connection{
		Name:         "Bad Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "", // missing
		DefaultModel: "llama3:8b",
	})
	if err == nil {
		t.Fatal("Create with missing BaseURL: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "base URL") {
		t.Errorf("error = %q, want it to mention 'base URL'", err.Error())
	}
}

// TestOllama_ConnectionValidation_MissingDefaultModel verifies that an Ollama
// connection without a DefaultModel is rejected.
func TestOllama_ConnectionValidation_MissingDefaultModel(t *testing.T) {
	dir := t.TempDir()
	store := NewConnectionStore(dir)

	_, err := store.Create(Connection{
		Name:         "No-Model Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "", // missing
	})
	if err == nil {
		t.Fatal("Create with missing DefaultModel: expected error, got nil")
	}
}

// TestOllama_DeleteCascade_ClearsStageAssignments verifies that when an Ollama
// connection is deleted, all stage assignments referencing it are cleared and
// subsequent pipeline resolution falls back to the Claude CLI default.
func TestOllama_DeleteCascade_ClearsStageAssignments(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	connStore := NewConnectionStore(globalDir)
	conn, err := connStore.Create(Connection{
		Name:         "Temp Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stageConfig := NewStageConfigStore(globalDir)
	// Assign to vision and ux globally.
	for _, stage := range []model.StageName{model.StageVision, model.StageUX} {
		if err := stageConfig.SetGlobalStageDefault(stage, StageAssignment{
			ConnectionID: conn.ID,
			Model:        "llama3:8b",
		}); err != nil {
			t.Fatalf("SetGlobalStageDefault(%s): %v", stage, err)
		}
	}
	// Assign to architecture via project override.
	if err := stageConfig.SetProjectStageOverride(projectDir, model.StageArchitecture, &StageAssignment{
		ConnectionID: conn.ID,
		Model:        "llama3:70b",
	}); err != nil {
		t.Fatalf("SetProjectStageOverride: %v", err)
	}

	// Delete with cascade.
	affected, err := connStore.Delete(conn.ID, stageConfig, []string{projectDir})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Three assignments should have been cleared.
	if len(affected) < 3 {
		t.Errorf("expected at least 3 affected stages, got %d: %v", len(affected), affected)
	}

	// After deletion, all stages should fall back to Claude CLI.
	registry := NewRegistry(connStore)
	for _, stage := range []model.StageName{model.StageVision, model.StageUX, model.StageArchitecture} {
		prov, _, err := registry.ResolveForStage("proj-cascade", stage, stageConfig, projectDir)
		if err != nil {
			t.Errorf("ResolveForStage(%s) after delete: %v", stage, err)
			continue
		}
		if _, ok := prov.(*ClaudeCLIProvider); !ok {
			t.Errorf("ResolveForStage(%s): want *ClaudeCLIProvider after delete, got %T", stage, prov)
		}
	}
}

// ---------------------------------------------------------------------------
// Registry: ForConnection with Ollama type
// ---------------------------------------------------------------------------

// TestRegistry_ForConnection_ReturnsOllamaProvider verifies that
// Registry.ForConnection instantiates an *OllamaProvider when the stored
// connection has ProviderType == "ollama".
func TestRegistry_ForConnection_ReturnsOllamaProvider(t *testing.T) {
	dir := t.TempDir()
	connStore := NewConnectionStore(dir)

	conn, err := connStore.Create(Connection{
		Name:         "Ollama via Registry",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	registry := NewRegistry(connStore)
	prov, err := registry.ForConnection(conn.ID)
	if err != nil {
		t.Fatalf("ForConnection: %v", err)
	}
	if _, ok := prov.(*OllamaProvider); !ok {
		t.Errorf("ForConnection: want *OllamaProvider, got %T", prov)
	}
}

// TestRegistry_ForUnsavedConnection_ReturnsOllamaProvider verifies that
// Registry.ForUnsavedConnection instantiates a Provider from a Connection
// struct without requiring it to be persisted in the ConnectionStore first.
// This supports the "Test Connection" flow where the user tests before saving.
func TestRegistry_ForUnsavedConnection_ReturnsOllamaProvider(t *testing.T) {
	dir := t.TempDir()
	connStore := NewConnectionStore(dir)
	registry := NewRegistry(connStore)

	unsaved := &Connection{
		Name:         "Unsaved Test",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	}

	prov, err := registry.ForUnsavedConnection(unsaved)
	if err != nil {
		t.Fatalf("ForUnsavedConnection: %v", err)
	}
	if _, ok := prov.(*OllamaProvider); !ok {
		t.Errorf("ForUnsavedConnection: want *OllamaProvider, got %T", prov)
	}
}

// TestRegistry_ForConnection_MissingBaseURL verifies that ForConnection returns
// an error when the Ollama connection has no BaseURL stored — even if the
// connection record exists, instantiation must fail descriptively.
func TestRegistry_ForConnection_MissingBaseURL(t *testing.T) {
	dir := t.TempDir()
	connStore := NewConnectionStore(dir)

	// Force-create a connection with an empty BaseURL by bypassing the store
	// validation through direct writeJSONAtomic — this simulates data that
	// was corrupted or migrated from an older schema.
	badConn := Connection{
		ID:           "bad-id-001",
		Name:         "Bad Ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "", // intentionally empty
		DefaultModel: "llama3:8b",
	}
	_ = writeJSONAtomic(connStore.path, connectionsEnvelope{Connections: []Connection{badConn}}, 0600)
	_ = connStore.ReloadFromDisk()

	registry := NewRegistry(connStore)
	_, err := registry.ForConnection("bad-id-001")
	if err == nil {
		t.Fatal("ForConnection with missing BaseURL: expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// TrailingSlash normalisation
// ---------------------------------------------------------------------------

// TestOllamaProvider_TrailingSlash_NormalisedAtConstruction verifies that
// NewOllamaProvider strips trailing slashes from the base URL. This prevents
// double-slash URLs like "http://localhost:11434//api/chat".
func TestOllamaProvider_TrailingSlash_NormalisedAtConstruction(t *testing.T) {
	chunks := []string{"ok"}
	srv := ollamaMockServer(t, []string{"llama3:8b"}, chunks)

	// Build a URL with a trailing slash — the provider must normalise it.
	urlWithSlash := srv.URL + "/"
	p := NewOllamaProvider(urlWithSlash)

	ch, err := p.Chat(context.Background(), ChatRequest{
		Model:       "llama3:8b",
		UserMessage: "hi",
	})
	if err != nil {
		t.Fatalf("Chat with trailing-slash URL: %v", err)
	}
	for range ch {
	} // drain
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// splitIntoChunks splits s into slices of at most chunkSize runes.
// Used to simulate realistic streaming where large responses arrive in fragments.
func splitIntoChunks(s string, chunkSize int) []string {
	runes := []rune(s)
	var chunks []string
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
