package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/michelroberge/paulette/backend/internal/model"
	ollamaprompts "github.com/michelroberge/paulette/backend/internal/prompts/ollama"
)

// stubExecutor returns canned responses keyed by the system prompt it receives.
// Each entry is emitted as a single "chunk" plus a "done" event. If a key is
// missing, the executor returns the errResp error.
type stubExecutor struct {
	byPrompt map[string]string
	errResp  error
	calls    int
}

func (s *stubExecutor) Chat(_ context.Context, req ChatExecutorRequest) (<-chan StreamEvent, error) {
	s.calls++
	if s.errResp != nil {
		return nil, s.errResp
	}
	body, ok := s.byPrompt[req.SystemPrompt]
	if !ok {
		// Differentiate draft vs critique using prompt substring.
		for k, v := range s.byPrompt {
			if strings.HasPrefix(req.SystemPrompt, k) {
				body = v
				ok = true
				break
			}
		}
	}
	ch := make(chan StreamEvent, 4)
	go func() {
		defer close(ch)
		if !ok {
			ch <- StreamEvent{Type: "error", Content: "stub: no response for prompt"}
			ch <- StreamEvent{Type: "done"}
			return
		}
		ch <- StreamEvent{Type: "chunk", Content: body}
		ch <- StreamEvent{Type: "tokens", Content: "42", Tokens: 42}
		ch <- StreamEvent{Type: "done"}
	}()
	return ch, nil
}

// fullSections returns a VisionSections with every field populated — used as a
// valid draft/critique fixture.
func fullSections() VisionSections {
	return VisionSections{
		ProblemStatement:       "Teams lose context switching between docs.",
		TargetUsers:            "Small engineering teams of 3-10.",
		CoreFeatures:           "Unified context, smart search, team handoffs.",
		UserExperience:         "Fast, focused, feels like extending your brain.",
		SuccessMetrics:         "Weekly active users, context switches saved per day.",
		ConstraintsAssumptions: "English-only at launch, assumes Slack as source of truth.",
		OutOfScope:             "Mobile apps, voice interface.",
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(b)
}

// drainEvents collects every event from the channel into slices of chunks and
// returns the done event's content (if any) plus whether an error was emitted.
func drainEvents(t *testing.T, ch <-chan StreamEvent) (chunks []string, doneContent string, sawError bool, logs []string) {
	t.Helper()
	for e := range ch {
		switch e.Type {
		case "chunk":
			chunks = append(chunks, e.Content)
		case "done":
			doneContent = e.Content
		case "error":
			sawError = true
		case "log":
			logs = append(logs, e.Content)
		}
	}
	return
}

func TestRenderVisionMarkdown_AllSections(t *testing.T) {
	md := RenderVisionMarkdown(fullSections())
	required := []string{
		"# Product Vision",
		"## Problem Statement",
		"## Target Users",
		"## Core Features",
		"## User Experience",
		"## Success Metrics",
		"## Constraints & Assumptions",
		"## Out of Scope (V1)",
	}
	for _, r := range required {
		if !strings.Contains(md, r) {
			t.Errorf("rendered markdown missing heading %q\n---\n%s", r, md)
		}
	}
}

func TestRenderVisionMarkdown_EmptySectionsGetPlaceholder(t *testing.T) {
	md := RenderVisionMarkdown(VisionSections{ProblemStatement: "Only this is set."})
	if !strings.Contains(md, "Only this is set.") {
		t.Errorf("expected populated section in output, got:\n%s", md)
	}
	if !strings.Contains(md, "_To be determined._") {
		t.Errorf("expected placeholder for empty sections, got:\n%s", md)
	}
}

func TestHasAllSections(t *testing.T) {
	if !fullSections().HasAllSections() {
		t.Error("fullSections should have all sections")
	}
	incomplete := fullSections()
	incomplete.SuccessMetrics = ""
	if incomplete.HasAllSections() {
		t.Error("incomplete should not report all sections")
	}
	if got := incomplete.MissingSections(); len(got) != 1 || got[0] != "Success Metrics" {
		t.Errorf("MissingSections() = %v, want [Success Metrics]", got)
	}
}

func TestRunVisionTurn_HappyPath_Approved(t *testing.T) {
	draft := VisionDraftOutput{
		Mode:       "draft",
		Discussion: "Here is the initial vision.",
		Sections:   ptrSections(fullSections()),
	}
	critique := VisionCritiqueOutput{Approved: true}

	exec := &stubExecutor{
		byPrompt: map[string]string{
			ollamaprompts.VisionDraftJSON:    mustJSON(t, draft),
			ollamaprompts.VisionCritiqueJSON: mustJSON(t, critique),
		},
	}

	ch, err := RunVisionTurn(context.Background(), VisionTurnOptions{
		Executor:    exec,
		Model:       "test-model",
		UserMessage: "Build me a focus app for remote teams.",
	})
	if err != nil {
		t.Fatalf("RunVisionTurn returned error: %v", err)
	}
	chunks, doneContent, sawError, _ := drainEvents(t, ch)
	if sawError {
		t.Fatal("runner emitted an error on happy path")
	}
	if exec.calls != 2 {
		t.Errorf("expected 2 executor calls (draft + critique), got %d", exec.calls)
	}
	if !strings.Contains(doneContent, "<!-- RESPONSE:START -->") {
		t.Errorf("done content missing envelope start marker:\n%s", doneContent)
	}
	if !strings.Contains(doneContent, "<artifact>") {
		t.Errorf("done content missing <artifact> block:\n%s", doneContent)
	}
	// The rendered markdown should contain all seven headings.
	if !strings.Contains(doneContent, "## Problem Statement") {
		t.Errorf("done content missing Problem Statement heading:\n%s", doneContent)
	}
	if len(chunks) == 0 || !strings.Contains(chunks[0], "initial vision") {
		t.Errorf("expected discussion chunk, got chunks=%v", chunks)
	}

	// ExtractArtifact must be able to extract the artifact from the envelope.
	artifact, ok := ExtractArtifact(doneContent)
	if !ok {
		t.Fatalf("ExtractArtifact failed to pull artifact from envelope:\n%s", doneContent)
	}
	if !strings.Contains(artifact, "## Core Features") {
		t.Errorf("extracted artifact missing Core Features:\n%s", artifact)
	}
}

func TestRunVisionTurn_QuestionMode_NoArtifact(t *testing.T) {
	draft := VisionDraftOutput{
		Mode:       "question",
		Discussion: "Could you clarify who the end user is?",
	}
	exec := &stubExecutor{
		byPrompt: map[string]string{
			ollamaprompts.VisionDraftJSON: mustJSON(t, draft),
		},
	}

	ch, err := RunVisionTurn(context.Background(), VisionTurnOptions{
		Executor:    exec,
		Model:       "test-model",
		UserMessage: "Who is this for exactly?",
	})
	if err != nil {
		t.Fatalf("RunVisionTurn returned error: %v", err)
	}
	_, doneContent, sawError, _ := drainEvents(t, ch)
	if sawError {
		t.Error("question-mode turn should not emit error")
	}
	if exec.calls != 1 {
		t.Errorf("question mode should skip critique, want 1 call got %d", exec.calls)
	}
	if strings.Contains(doneContent, "<artifact>") {
		t.Errorf("question-mode envelope should have no <artifact>, got:\n%s", doneContent)
	}
	if !strings.Contains(doneContent, "Could you clarify") {
		t.Errorf("question-mode envelope missing discussion text:\n%s", doneContent)
	}
}

func TestRunVisionTurn_CritiqueRevision_Accepted(t *testing.T) {
	draftSections := fullSections()
	draftSections.SuccessMetrics = "TBD" // something critique will flag

	revised := fullSections() // full & proper

	draft := VisionDraftOutput{
		Mode:       "draft",
		Discussion: "First pass.",
		Sections:   &draftSections,
	}
	critique := VisionCritiqueOutput{
		Approved:        false,
		Issues:          []string{"success_metrics is a placeholder"},
		RevisedSections: &revised,
	}
	exec := &stubExecutor{
		byPrompt: map[string]string{
			ollamaprompts.VisionDraftJSON:    mustJSON(t, draft),
			ollamaprompts.VisionCritiqueJSON: mustJSON(t, critique),
		},
	}

	ch, err := RunVisionTurn(context.Background(), VisionTurnOptions{
		Executor:    exec,
		Model:       "test-model",
		UserMessage: "Refine it.",
	})
	if err != nil {
		t.Fatalf("RunVisionTurn error: %v", err)
	}
	_, doneContent, _, logs := drainEvents(t, ch)

	if exec.calls != 2 {
		t.Errorf("should not repair when critique provides revision; want 2 calls got %d", exec.calls)
	}
	// Final artifact should contain the revised Success Metrics, not "TBD".
	if strings.Contains(doneContent, "TBD") {
		t.Errorf("final artifact still has TBD; revision was not applied:\n%s", doneContent)
	}
	if !containsSubstring(logs, "accepted critique revision") {
		t.Errorf("expected log about accepted revision, got logs=%v", logs)
	}
}

func TestRunVisionTurn_MalformedJSON_FallsBackToRawDiscussion(t *testing.T) {
	const rawText = "this is not json at all"
	exec := &stubExecutor{
		byPrompt: map[string]string{
			ollamaprompts.VisionDraftJSON: rawText,
		},
	}
	ch, err := RunVisionTurn(context.Background(), VisionTurnOptions{
		Executor:    exec,
		Model:       "test-model",
		UserMessage: "Go.",
	})
	if err != nil {
		t.Fatalf("RunVisionTurn error: %v", err)
	}
	chunks, doneContent, sawError, logs := drainEvents(t, ch)
	if sawError {
		t.Error("malformed JSON draft should NOT emit an error; should fall back to raw discussion")
	}
	if exec.calls != 1 {
		t.Errorf("fallback should skip critique, want 1 call got %d", exec.calls)
	}
	if !containsSubstring(chunks, rawText) {
		t.Errorf("expected raw text emitted as chunk, got chunks=%v", chunks)
	}
	if strings.Contains(doneContent, "<artifact>") {
		t.Errorf("fallback envelope should have no <artifact>, got:\n%s", doneContent)
	}
	if !strings.Contains(doneContent, rawText) {
		t.Errorf("fallback envelope missing raw discussion text:\n%s", doneContent)
	}
	if !containsSubstring(logs, "draft not structured") {
		t.Errorf("expected log about unstructured fallback, got logs=%v", logs)
	}
}

func TestRunVisionTurn_EmptyDraft_EmitsError(t *testing.T) {
	exec := &stubExecutor{
		byPrompt: map[string]string{
			ollamaprompts.VisionDraftJSON: "   \n\t  ",
		},
	}
	ch, err := RunVisionTurn(context.Background(), VisionTurnOptions{
		Executor:    exec,
		Model:       "test-model",
		UserMessage: "Go.",
	})
	if err != nil {
		t.Fatalf("RunVisionTurn error: %v", err)
	}
	_, _, sawError, _ := drainEvents(t, ch)
	if !sawError {
		t.Error("empty/whitespace draft should emit an error event — nothing to show user")
	}
}

func TestRunVisionTurn_ExecutorStartError_BubblesUp(t *testing.T) {
	exec := &stubExecutor{errResp: errors.New("connection refused")}
	ch, err := RunVisionTurn(context.Background(), VisionTurnOptions{
		Executor:    exec,
		Model:       "test-model",
		UserMessage: "Go.",
	})
	// The first provider call is synchronous so HTTP/connection errors MUST
	// surface as RunVisionTurn's return error — that lets the handler wrap
	// them in a structured connection-error payload the same way the legacy
	// single-shot path does.
	if err == nil {
		t.Fatal("expected startup error to be returned synchronously, got nil")
	}
	if ch != nil {
		t.Error("expected nil channel when RunVisionTurn returns a startup error")
	}
}

func TestRunVisionTurn_MissingExecutor(t *testing.T) {
	_, err := RunVisionTurn(context.Background(), VisionTurnOptions{Model: "x"})
	if err == nil {
		t.Error("expected error when Executor is nil")
	}
}

// History is passed through to the draft call but not required to affect
// behaviour. Smoke test that it is accepted without panicking.
func TestRunVisionTurn_WithHistory(t *testing.T) {
	draft := VisionDraftOutput{
		Mode:       "draft",
		Discussion: "With history.",
		Sections:   ptrSections(fullSections()),
	}
	critique := VisionCritiqueOutput{Approved: true}
	exec := &stubExecutor{
		byPrompt: map[string]string{
			ollamaprompts.VisionDraftJSON:    mustJSON(t, draft),
			ollamaprompts.VisionCritiqueJSON: mustJSON(t, critique),
		},
	}
	ch, err := RunVisionTurn(context.Background(), VisionTurnOptions{
		Executor: exec,
		Model:    "m",
		History: []model.Message{
			{Role: model.RoleUser, Content: "prev question"},
			{Role: model.RoleAssistant, Content: "prev answer"},
		},
		UserMessage: "continue",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, doneContent, sawError, _ := drainEvents(t, ch)
	if sawError {
		t.Error("unexpected error with history")
	}
	if !strings.Contains(doneContent, "<artifact>") {
		t.Errorf("expected artifact envelope, got:\n%s", doneContent)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func ptrSections(s VisionSections) *VisionSections { return &s }

func containsSubstring(hay []string, needle string) bool {
	for _, h := range hay {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}
