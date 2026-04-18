// Package handler — connection error recovery flow tests (Milestone 6.4).
//
// These tests simulate the full end-to-end connection error recovery journey
// described in JRN-v0.2.0-008:
//
//  1. User sends a message → backend attempts to call the configured LLM provider.
//  2. Provider is unreachable → error event emitted immediately (no silent hang).
//  3. Error event carries a structured connectionErrorPayload so the frontend
//     can render the actionable ConnectionErrorBanner (SCR-012).
//  4. User message is persisted to disk before the provider attempt, so it is
//     not lost when the provider fails (no data loss).
//  5. User fixes the connection (updates URL in the Configure page) and retries
//     via the Resume endpoint — the saved message is replayed without duplication.
//
// Acceptance criteria tested here:
//   - Error banner appears immediately (no silent hang): verified via StartChatRun
//     returning synchronously with an error event when the provider fails.
//   - "Go to Configure" deep-link correctness is a frontend concern covered by
//     the ConnectionErrorBanner component and ConfigurePage; the backend side is
//     that connectionId and connectionName are populated in the error payload.
//   - After fixing connection, retry works without data loss: verified via
//     resumeChatRun succeeding and producing exactly one user + one assistant
//     message with no duplicates.
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/michelroberge/paulette/backend/internal/agent"
	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
	fsrepo "github.com/michelroberge/paulette/backend/internal/repository/fs"
	"github.com/michelroberge/paulette/backend/internal/stream"
)

// ---------------------------------------------------------------------------
// Mock HTTP servers
// ---------------------------------------------------------------------------

// newBrokenOllamaServer returns an httptest.Server whose /api/chat endpoint
// always returns HTTP 500, simulating a misconfigured or unreachable Ollama
// instance (e.g. wrong base URL, server not running).
func newBrokenOllamaServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "simulated provider failure", http.StatusInternalServerError)
	}))
}

// newWorkingOllamaServer returns an httptest.Server whose /api/chat endpoint
// streams a minimal valid Ollama NDJSON response, simulating a properly
// configured Ollama instance after the user has fixed the broken connection.
func newWorkingOllamaServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			// Minimal valid streaming response: one content chunk then done.
			fmt.Fprintln(w, `{"message":{"role":"assistant","content":"Recovery successful!"},"done":false}`)
			fmt.Fprintln(w, `{"done":true}`)
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"models":[{"name":"llama3:8b"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

// ---------------------------------------------------------------------------
// Test helper: drain all events from a run (waits for completion)
// ---------------------------------------------------------------------------

// collectRunEvents subscribes to the run from the beginning and drains all
// events until the run is done and the channel is closed. This acts as a
// synchronisation barrier — it returns only after the background goroutine
// (if any) has finished all processing and called run.Finish.
func collectRunEvents(run *stream.Run) []agent.StreamEvent {
	ch, _ := run.Subscribe(0)
	var events []agent.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// hasEventOfType returns true if events contains at least one event with the
// given type string.
func hasEventOfType(events []agent.StreamEvent, eventType string) bool {
	for _, e := range events {
		if e.Type == eventType {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helper: build a handler wired with real persistence + real provider registry
// ---------------------------------------------------------------------------

// buildRecoveryHandler constructs a ChatHandler backed by real filesystem
// repositories in temporary directories and a real provider registry. It
// returns the handler, the ConnectionStore (for seeding / updating connections),
// the StageConfigStore (for assigning providers to stages), and the project
// directory (as the project's HostDir).
func buildRecoveryHandler(t *testing.T) (
	h *ChatHandler,
	chatRepo *fsrepo.ChatRepo,
	connStore *provider.ConnectionStore,
	stageConfig *provider.StageConfigStore,
	projectDir string,
) {
	t.Helper()

	globalDir := t.TempDir()
	projectDir = t.TempDir()

	connStore = provider.NewConnectionStore(globalDir)
	stageConfig = provider.NewStageConfigStore(globalDir)
	provRegistry := provider.NewRegistry(connStore)

	chatRepo = fsrepo.NewChatRepo()
	runs := stream.NewManager()

	h = NewChatHandler(
		&mockRegistryRepo{},
		chatRepo,
		&nopArtifactRepo{},
		&nopActivityRepo{},
		runs,
		provRegistry,
		stageConfig,
		connStore,
		nil, // ragClient
		"",  // logBase empty in tests
		"",  // regPath empty in tests
	)
	return
}

// ---------------------------------------------------------------------------
// Test 1: Error event emitted immediately — no silent hang
// ---------------------------------------------------------------------------

// TestConnectionErrorRecovery_ErrorEventEmittedImmediately verifies that when
// the configured provider is unreachable, StartChatRun returns synchronously
// with a structured error event rather than hanging until a timeout fires.
//
// This covers the AC: "Error banner appears immediately (no silent hang)".
func TestConnectionErrorRecovery_ErrorEventEmittedImmediately(t *testing.T) {
	brokenSrv := newBrokenOllamaServer(t)
	defer brokenSrv.Close()

	h, _, connStore, stageConfig, projectDir := buildRecoveryHandler(t)

	conn, err := connStore.Create(provider.Connection{
		Name:         "Broken Ollama",
		ProviderType: provider.ProviderOllama,
		BaseURL:      brokenSrv.URL,
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}
	if err := stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{
		ConnectionID: conn.ID, Model: "llama3:8b",
	}); err != nil {
		t.Fatalf("set stage default: %v", err)
	}

	proj := &model.Project{ID: "err-immediate", HostDir: projectDir, Version: "1.0"}

	// StartChatRun should return synchronously — not block until a network timeout.
	run, err := h.StartChatRun(proj, model.StageVision, "Hello from the user")
	if err != nil {
		t.Fatalf("StartChatRun: unexpected error: %v", err)
	}
	if run == nil {
		t.Fatal("StartChatRun returned nil run")
	}

	// Collect events — the run should already be done (error path is synchronous).
	events := collectRunEvents(run)

	// Must contain at least one error event.
	if !hasEventOfType(events, "error") {
		t.Fatalf("no error event emitted; got %d events: %v", len(events), events)
	}

	// The error event Content must be a structured connectionErrorPayload.
	for _, e := range events {
		if e.Type != "error" {
			continue
		}
		var payload struct {
			IsConnectionError bool   `json:"isConnectionError"`
			ConnectionID      string `json:"connectionId"`
			ConnectionName    string `json:"connectionName"`
			Reason            string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(e.Content), &payload); err != nil {
			t.Fatalf("error event Content is not valid JSON: %v\ncontent: %s", err, e.Content)
		}
		if !payload.IsConnectionError {
			t.Error("want isConnectionError=true so the frontend renders the ConnectionErrorBanner")
		}
		if payload.ConnectionID == "" {
			t.Error("want non-empty connectionId for the 'Go to Configure' deep-link")
		}
		if payload.ConnectionName != "Broken Ollama" {
			t.Errorf("connectionName = %q, want 'Broken Ollama'", payload.ConnectionName)
		}
		if payload.Reason == "" {
			t.Error("want non-empty reason so the banner can describe the failure")
		}
		break
	}
}

// ---------------------------------------------------------------------------
// Test 2: User message preserved after provider failure (no data loss)
// ---------------------------------------------------------------------------

// TestConnectionErrorRecovery_UserMessagePreservedAfterError verifies that the
// user's message is persisted to the chat repository before the provider attempt
// is made. This ensures data is not lost when the provider fails.
//
// This covers the AC: "After fixing connection, retry works without data loss".
func TestConnectionErrorRecovery_UserMessagePreservedAfterError(t *testing.T) {
	brokenSrv := newBrokenOllamaServer(t)
	defer brokenSrv.Close()

	h, chatRepo, connStore, stageConfig, projectDir := buildRecoveryHandler(t)

	conn, _ := connStore.Create(provider.Connection{
		Name:         "Broken Ollama",
		ProviderType: provider.ProviderOllama,
		BaseURL:      brokenSrv.URL,
		DefaultModel: "llama3:8b",
	})
	stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{ //nolint:errcheck
		ConnectionID: conn.ID, Model: "llama3:8b",
	})

	proj := &model.Project{ID: "msg-preserved", HostDir: projectDir, Version: "1.0"}

	const userMessage = "Build me a task-tracking application"

	run, err := h.StartChatRun(proj, model.StageVision, userMessage)
	if err != nil {
		t.Fatalf("StartChatRun: %v", err)
	}
	collectRunEvents(run) // wait for completion

	// The user message must be on disk even though the provider call failed.
	history, err := chatRepo.GetHistory(projectDir, model.StageVision)
	if err != nil {
		t.Fatalf("GetHistory after failed run: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("chat history is empty — user message was not preserved after provider failure")
	}
	if history[0].Role != model.RoleUser {
		t.Errorf("first message role = %q, want %q", history[0].Role, model.RoleUser)
	}
	if history[0].Content != userMessage {
		t.Errorf("user message = %q, want %q", history[0].Content, userMessage)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Retry succeeds after connection is fixed
// ---------------------------------------------------------------------------

// TestConnectionErrorRecovery_RetrySucceedsAfterFix simulates the complete
// user journey:
//
//  1. Send message → provider fails → error event emitted.
//  2. User updates the connection URL (e.g. via the Configure page).
//  3. User retries (Resume endpoint) → provider now succeeds → done event emitted.
//
// This covers the AC: "After fixing connection, retry works without data loss".
func TestConnectionErrorRecovery_RetrySucceedsAfterFix(t *testing.T) {
	brokenSrv := newBrokenOllamaServer(t)
	defer brokenSrv.Close()

	workingSrv := newWorkingOllamaServer(t)
	defer workingSrv.Close()

	h, chatRepo, connStore, stageConfig, projectDir := buildRecoveryHandler(t)

	// Configure stage to use the broken server initially.
	conn, err := connStore.Create(provider.Connection{
		Name:         "Ollama (to be fixed)",
		ProviderType: provider.ProviderOllama,
		BaseURL:      brokenSrv.URL,
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}
	stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{ //nolint:errcheck
		ConnectionID: conn.ID, Model: "llama3:8b",
	})

	proj := &model.Project{ID: "retry-fix", HostDir: projectDir, Version: "1.0"}

	// Step 1: Initial attempt — should fail and emit a structured error event.
	const userMessage = "Design a product roadmap tool"
	run1, err := h.StartChatRun(proj, model.StageVision, userMessage)
	if err != nil {
		t.Fatalf("StartChatRun: %v", err)
	}
	events1 := collectRunEvents(run1)

	if !hasEventOfType(events1, "error") {
		t.Fatal("expected error event from broken provider; none found")
	}

	// Step 2: Simulate user fixing the connection by updating the base URL.
	// This mirrors what the PUT /api/connections/:id endpoint does.
	_, err = connStore.Update(provider.Connection{
		ID:           conn.ID,
		Name:         conn.Name,
		ProviderType: provider.ProviderOllama,
		BaseURL:      workingSrv.URL, // now points to the working server
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("update connection: %v", err)
	}

	// Step 3: Retry via resumeChatRun — mirrors what the Resume HTTP handler does:
	//   1. Read history from disk.
	//   2. Split into priorHistory (all but last) and lastUserMsg.
	//   3. Call resumeChatRun(project, stage, priorHistory, lastUserMsg).
	history, err := chatRepo.GetHistory(projectDir, model.StageVision)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) == 0 || history[len(history)-1].Role != model.RoleUser {
		t.Fatal("expected at least one user message in history for resume")
	}

	lastMsg := history[len(history)-1]
	priorHistory := history[:len(history)-1]

	run2, err := h.resumeChatRun(proj, model.StageVision, priorHistory, lastMsg.Content)
	if err != nil {
		t.Fatalf("resumeChatRun: %v", err)
	}
	if run2 == nil {
		t.Fatal("resumeChatRun returned nil run (race condition?)")
	}

	// Collect events from the retry run.
	events2 := collectRunEvents(run2)

	// The retry must NOT produce an error event.
	if hasEventOfType(events2, "error") {
		// Print all error event contents for diagnosis.
		for _, e := range events2 {
			if e.Type == "error" {
				t.Errorf("unexpected error event after connection fix: %s", e.Content)
			}
		}
	}

	// The retry must produce a "done" event to confirm the conversation completed.
	if !hasEventOfType(events2, "done") {
		t.Errorf("no done event from retry run; events: %v", events2)
	}
}

// ---------------------------------------------------------------------------
// Test 4: No message duplication on retry
// ---------------------------------------------------------------------------

// TestConnectionErrorRecovery_NoMessageDuplicationOnRetry verifies that the
// retry path (Resume) does not create duplicate user messages. After a failed
// attempt followed by a successful retry, the chat history should contain
// exactly one user message and one assistant message.
//
// This covers the AC: "retry works without data loss" — specifically ensuring
// that data is not corrupted (duplicated) in the process.
func TestConnectionErrorRecovery_NoMessageDuplicationOnRetry(t *testing.T) {
	brokenSrv := newBrokenOllamaServer(t)
	defer brokenSrv.Close()

	workingSrv := newWorkingOllamaServer(t)
	defer workingSrv.Close()

	h, chatRepo, connStore, stageConfig, projectDir := buildRecoveryHandler(t)

	conn, _ := connStore.Create(provider.Connection{
		Name:         "Ollama Dedup Test",
		ProviderType: provider.ProviderOllama,
		BaseURL:      brokenSrv.URL,
		DefaultModel: "llama3:8b",
	})
	stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{ //nolint:errcheck
		ConnectionID: conn.ID, Model: "llama3:8b",
	})

	proj := &model.Project{ID: "dedup-test", HostDir: projectDir, Version: "1.0"}

	const userMessage = "What tech stack should I use for a SaaS product?"

	// Attempt 1: fails with broken provider.
	run1, _ := h.StartChatRun(proj, model.StageVision, userMessage)
	collectRunEvents(run1)

	// Fix connection.
	connStore.Update(provider.Connection{ //nolint:errcheck
		ID:           conn.ID,
		Name:         conn.Name,
		ProviderType: provider.ProviderOllama,
		BaseURL:      workingSrv.URL,
		DefaultModel: "llama3:8b",
	})

	// Retry via resume (mirrors the Resume HTTP handler).
	history, _ := chatRepo.GetHistory(projectDir, model.StageVision)
	if len(history) == 0 {
		t.Fatal("no messages in history — user message was not preserved")
	}
	lastMsg := history[len(history)-1]
	priorHistory := history[:len(history)-1]

	run2, _ := h.resumeChatRun(proj, model.StageVision, priorHistory, lastMsg.Content)
	if run2 == nil {
		t.Fatal("resumeChatRun returned nil run")
	}
	collectRunEvents(run2) // wait for goroutine to save assistant message

	// Verify final message count: exactly 1 user + 1 assistant, no duplicates.
	finalHistory, err := chatRepo.GetHistory(projectDir, model.StageVision)
	if err != nil {
		t.Fatalf("GetHistory after retry: %v", err)
	}

	var userCount, assistantCount int
	for _, msg := range finalHistory {
		switch msg.Role {
		case model.RoleUser:
			userCount++
		case model.RoleAssistant:
			assistantCount++
		}
	}

	if userCount != 1 {
		t.Errorf("want exactly 1 user message (no duplication), got %d", userCount)
	}
	if assistantCount != 1 {
		t.Errorf("want exactly 1 assistant message after successful retry, got %d", assistantCount)
	}
	if len(finalHistory) != 2 {
		t.Errorf("want 2 messages total (1 user + 1 assistant), got %d", len(finalHistory))
	}

	// Confirm the user message content is preserved correctly.
	if finalHistory[0].Role == model.RoleUser && finalHistory[0].Content != userMessage {
		t.Errorf("user message content = %q, want %q", finalHistory[0].Content, userMessage)
	}
}

// ---------------------------------------------------------------------------
// Test 5: Error payload includes connection metadata for deep-link
// ---------------------------------------------------------------------------

// TestConnectionErrorRecovery_ErrorPayloadIncludesDeepLinkFields verifies
// that the connectionId and connectionName fields are always populated in the
// error payload when a stage assignment exists. These fields are required by
// the frontend to build the "Go to Configure →" deep-link URL:
//
//	/configure?tab=connections&highlight={connectionId}
//
// The connection card highlighted with a 3-second pulse is the primary CTA
// that lets users navigate directly to the broken connection.
func TestConnectionErrorRecovery_ErrorPayloadIncludesDeepLinkFields(t *testing.T) {
	brokenSrv := newBrokenOllamaServer(t)
	defer brokenSrv.Close()

	h, _, connStore, stageConfig, projectDir := buildRecoveryHandler(t)

	conn, err := connStore.Create(provider.Connection{
		Name:         "My Ollama for Deep-link Test",
		ProviderType: provider.ProviderOllama,
		BaseURL:      brokenSrv.URL,
		DefaultModel: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}
	stageConfig.SetGlobalStageDefault(model.StageVision, provider.StageAssignment{ //nolint:errcheck
		ConnectionID: conn.ID, Model: "llama3:8b",
	})

	proj := &model.Project{ID: "deeplink-test", HostDir: projectDir, Version: "1.0"}

	run, _ := h.StartChatRun(proj, model.StageVision, "Deep-link test message")
	if run == nil {
		t.Fatal("StartChatRun returned nil run")
	}
	events := collectRunEvents(run)

	for _, e := range events {
		if e.Type != "error" {
			continue
		}
		var payload struct {
			IsConnectionError bool   `json:"isConnectionError"`
			ConnectionID      string `json:"connectionId"`
			ConnectionName    string `json:"connectionName"`
			Reason            string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(e.Content), &payload); err != nil {
			t.Fatalf("error event is not valid JSON: %v", err)
		}

		// All three fields required for the deep-link to work.
		if payload.ConnectionID == "" {
			t.Error("connectionId is empty — 'Go to Configure' deep-link cannot highlight the card")
		}
		if payload.ConnectionID != conn.ID {
			t.Errorf("connectionId = %q, want %q", payload.ConnectionID, conn.ID)
		}
		if payload.ConnectionName == "" {
			t.Error("connectionName is empty — banner cannot display which connection failed")
		}
		if payload.ConnectionName != "My Ollama for Deep-link Test" {
			t.Errorf("connectionName = %q, want 'My Ollama for Deep-link Test'", payload.ConnectionName)
		}
		if payload.Reason == "" {
			t.Error("reason is empty — banner cannot describe what went wrong")
		}
		return
	}

	t.Fatal("no error event found in run events")
}
