package refinement

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// --- Mock Provider ---

type mockProvider struct {
	responses map[string]string // keyed by first word of userMessage
	callCount int
}

func (m *mockProvider) Chat(_ context.Context, req provider.ChatRequest) (<-chan model.StreamEvent, error) {
	m.callCount++
	ch := make(chan model.StreamEvent, 2)

	// Find response based on system prompt keyword matching
	resp := m.findResponse(req.SystemPrompt, req.UserMessage)

	go func() {
		ch <- model.StreamEvent{Type: "done", Content: resp}
		ch <- model.StreamEvent{Type: "tokens", Content: "10"}
		close(ch)
	}()
	return ch, nil
}

func (m *mockProvider) findResponse(sys, usr string) string {
	sysLower := strings.ToLower(sys)
	switch {
	case strings.Contains(sysLower, "summarize"):
		return "A tool for managing tasks. It helps teams collaborate."
	case strings.Contains(sysLower, "critical unknowns") || strings.Contains(sysLower, "missing information"):
		return "1. Who are the primary users?\n2. What is the main problem being solved?\n3. How will success be measured?"
	case strings.Contains(sysLower, "extract structured items"):
		return "- Teams need a way to track tasks\n- Collaboration is a key feature\n- The tool is for project management"
	case strings.Contains(sysLower, "classify"):
		if strings.Contains(strings.ToLower(usr), "team") || strings.Contains(strings.ToLower(usr), "collaborat") {
			return "users"
		}
		if strings.Contains(strings.ToLower(usr), "track") || strings.Contains(strings.ToLower(usr), "manage") {
			return "features"
		}
		return "problem"
	case strings.Contains(sysLower, "merge") || strings.Contains(sysLower, "add the new fact"):
		existing := ""
		if idx := strings.Index(usr, "Existing content:"); idx >= 0 {
			existing = usr[idx:]
		}
		fact := ""
		if idx := strings.Index(usr, "New fact"); idx >= 0 {
			fact = usr[idx:]
		}
		if existing == "" {
			return fact
		}
		return existing + "\n" + fact
	case strings.Contains(sysLower, "rate the completeness") || strings.Contains(sysLower, "completeness"):
		if strings.Contains(usr, "(empty") {
			return "0.1"
		}
		return "0.7"
	case strings.Contains(sysLower, "missing or unclear"):
		return "NONE"
	case strings.Contains(sysLower, "product owner"):
		return "The primary users are small development teams of 3-10 people. They struggle with tracking who is working on what."
	case strings.Contains(sysLower, "score each question"):
		return `[{"relevance": 0.8, "novelty": 0.7, "impact": 0.9}]`
	case strings.Contains(sysLower, "rewrite this question"):
		return "Specifically, " + usr
	case strings.Contains(sysLower, "consistent with the existing"):
		return "COHERENT"
	case strings.Contains(sysLower, "critical advisor"):
		return "NONE"
	case strings.Contains(sysLower, "self-contained statements"):
		// Pass-through: return the answer portion unchanged
		if idx := strings.Index(usr, "User's answer: "); idx >= 0 {
			parts := strings.SplitN(usr[idx+len("User's answer: "):], "\n\nRewrite:", 2)
			return strings.TrimSpace(parts[0])
		}
		return usr
	case strings.Contains(sysLower, "critique"):
		return "NONE"
	case strings.Contains(sysLower, "generate a structured"):
		return "# Product Vision: Task Tracker\n\n## Problem Statement\nTeams lack visibility.\n\n## Target Users\nSmall dev teams."
	default:
		return "OK"
	}
}

func (m *mockProvider) ExecuteAgent(_ context.Context, _ provider.AgentRequest) (<-chan model.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockProvider) TestConnection(_ context.Context) error         { return nil }
func (m *mockProvider) ListModels(_ context.Context) ([]provider.ModelInfo, error) {
	return nil, provider.ErrModelListUnsupported
}

// --- Tests ---

func TestNewLoopState(t *testing.T) {
	state := NewLoopState("build a task tracker", VisionSections, 10)
	if state.IdeaRaw != "build a task tracker" {
		t.Errorf("expected idea_raw to be set")
	}
	if len(state.Sections) != len(VisionSections) {
		t.Errorf("expected %d sections, got %d", len(VisionSections), len(state.Sections))
	}
	if state.MaxIterations != 10 {
		t.Errorf("expected max_iterations=10, got %d", state.MaxIterations)
	}
	if state.Phase != PhaseInit {
		t.Errorf("expected phase=init, got %s", state.Phase)
	}
}

func TestIsConverged(t *testing.T) {
	state := NewLoopState("test", VisionSections, 5)

	// Not converged: all confidence at 0
	if IsConverged(state, 0.85) {
		t.Error("should not be converged at 0 confidence")
	}

	// Set all confidence to 0.9
	for _, s := range VisionSections {
		state.Confidence[s] = 0.9
	}
	// Still has questions
	state.OpenQuestions = []Question{{ID: "q1", Text: "test?", Answered: false}}
	if IsConverged(state, 0.85) {
		t.Error("should not be converged with open questions")
	}

	// Mark question answered and prune
	state.OpenQuestions[0].Answered = true
	PruneResolved(state, 0.85)
	if !IsConverged(state, 0.85) {
		t.Error("should be converged: high confidence + no open questions")
	}

	// Max iterations
	state2 := NewLoopState("test", VisionSections, 3)
	state2.Iteration = 3
	if !IsConverged(state2, 0.85) {
		t.Error("should be converged at max iterations")
	}
}

func TestSelectHighestImpactQuestion(t *testing.T) {
	state := NewLoopState("test", VisionSections, 10)
	state.Confidence[SectionProblem] = 0.8
	state.Confidence[SectionUsers] = 0.2
	state.OpenQuestions = []Question{
		{ID: "q1", Text: "What problem?", Section: SectionProblem, Impact: 0.5},
		{ID: "q2", Text: "Who are users?", Section: SectionUsers, Impact: 0.5},
	}

	q := SelectHighestImpactQuestion(state)
	if q == nil || q.ID != "q2" {
		t.Errorf("expected q2 (users has lower confidence), got %v", q)
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"0.75", 0.75},
		{"The score is about 0.6", 0.6},
		{"1.0", 1.0},
		{"0", 0},
		{"garbage", 0},
		{"2.5", 1.0}, // clamped
	}
	for _, tt := range tests {
		got := parseFloat(tt.input)
		if got != tt.expected {
			t.Errorf("parseFloat(%q) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestParseFactList(t *testing.T) {
	input := "Here are the facts:\n- Teams need tracking\n- Collaboration is key\n* Feature three\n1. Numbered fact"
	facts := parseFactList(input)
	if len(facts) != 4 {
		t.Errorf("expected 4 facts (preamble filtered), got %d: %v", len(facts), facts)
	}
}

func TestParseQuestionListFiltersPreamble(t *testing.T) {
	state := NewLoopState("test", VisionSections, 10)
	input := "Here are 5 critical unknowns as numbered questions:\n\n1. Who are the primary users?\n2. What problem does this solve?\n3. What are the constraints?"
	questions := parseQuestionList(input, state)
	if len(questions) != 3 {
		t.Errorf("expected 3 questions (preamble filtered), got %d: %v", len(questions), questions)
	}
	for _, q := range questions {
		if strings.Contains(q.Text, "Here are") {
			t.Errorf("preamble should have been filtered: %q", q.Text)
		}
	}
}

func TestParseSectionName(t *testing.T) {
	tests := []struct {
		input    string
		expected SectionName
	}{
		{"users", SectionUsers},
		{"USERS", SectionUsers},
		{"The category is users.", SectionUsers},
		{"out_of_scope", SectionOutOfScope},
		{"out of scope", SectionOutOfScope},
		{"completely unknown", SectionProblem}, // falls back to first
	}
	for _, tt := range tests {
		got := parseSectionName(tt.input, VisionSections)
		if got != tt.expected {
			t.Errorf("parseSectionName(%q) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestMergeQuestions(t *testing.T) {
	state := NewLoopState("test", VisionSections, 10)
	state.OpenQuestions = []Question{
		{ID: "q1", Text: "Who are the users?"},
	}
	MergeQuestions(state, []Question{
		{ID: "q2", Text: "Who are the users?"},  // dupe
		{ID: "q3", Text: "What is the problem?"}, // new
	})
	if len(state.OpenQuestions) != 2 {
		t.Errorf("expected 2 questions after merge (dedup), got %d", len(state.OpenQuestions))
	}
}

func TestRunLoopAutonomous(t *testing.T) {
	mock := &mockProvider{responses: map[string]string{}}
	ctrl := NewController(ControllerConfig{
		Provider:      mock,
		ModelID:       "test-model",
		ProjectDir:    "/tmp/test",
		Stage:         "vision",
		Prompts:       VisionPromptSet{},
		MaxIter:       3,
		MinConfidence: 0.5, // low threshold so mock responses converge quickly
	})

	state := NewLoopState("Build a task tracking tool for small development teams", VisionSections, 3)

	var events []LoopEvent
	emitFn := func(e LoopEvent) { events = append(events, e) }
	saveFn := func(_ *LoopState) {} // no-op for test

	artifact, tokens, err := ctrl.RunLoop(context.Background(), state, nil, emitFn, saveFn)
	if err != nil {
		t.Fatalf("RunLoop error: %v", err)
	}
	if artifact == "" {
		t.Error("expected non-empty artifact")
	}
	if tokens == 0 {
		t.Error("expected non-zero token count")
	}
	if len(events) == 0 {
		t.Error("expected progress events")
	}
	if state.Phase != PhaseComplete {
		t.Errorf("expected phase=complete, got %s", state.Phase)
	}
	if mock.callCount == 0 {
		t.Error("expected LLM calls")
	}

	t.Logf("Completed in %d iterations, %d LLM calls, %d tokens", state.Iteration, mock.callCount, tokens)
	t.Logf("Artifact preview: %s", artifact[:min(len(artifact), 200)])
}

func TestStatePersistence(t *testing.T) {
	dir := t.TempDir()
	state := NewLoopState("test idea", VisionSections, 5)
	state.IdeaSummary = "A test product"
	state.KnownFacts = []string{"fact 1", "fact 2"}
	state.Confidence[SectionProblem] = 0.7

	if err := SaveState(dir, state); err != nil {
		t.Fatalf("SaveState error: %v", err)
	}

	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState error: %v", err)
	}
	if loaded.IdeaRaw != "test idea" {
		t.Error("IdeaRaw mismatch")
	}
	if loaded.IdeaSummary != "A test product" {
		t.Error("IdeaSummary mismatch")
	}
	if len(loaded.KnownFacts) != 2 {
		t.Error("KnownFacts mismatch")
	}
	if loaded.Confidence[SectionProblem] != 0.7 {
		t.Error("Confidence mismatch")
	}

	// Clear
	if err := ClearState(dir); err != nil {
		t.Fatalf("ClearState error: %v", err)
	}
	cleared, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState after clear: %v", err)
	}
	if cleared != nil {
		t.Error("expected nil after clear")
	}
}

func TestFilterAndRankQuestions(t *testing.T) {
	questions := []Question{
		{ID: "q1", Text: "Low score?", Score: 0.3},
		{ID: "q2", Text: "High score?", Score: 0.9},
		{ID: "q3", Text: "Medium score?", Score: 0.6},
		{ID: "q4", Text: "Unscored?", Score: 0}, // unscored, should be kept
	}
	result := FilterAndRankQuestions(questions, 0.6)

	// q1 (0.3) should be discarded
	for _, q := range result {
		if q.ID == "q1" {
			t.Error("expected q1 (score 0.3) to be filtered out")
		}
	}
	// q2 should be first (highest score)
	if len(result) < 1 || result[0].ID != "q2" {
		t.Errorf("expected q2 first, got %v", result)
	}
	// q4 (unscored) should still be present
	found := false
	for _, q := range result {
		if q.ID == "q4" {
			found = true
		}
	}
	if !found {
		t.Error("expected unscored q4 to be kept")
	}
}

func TestMergeQuestionsSimilarity(t *testing.T) {
	state := NewLoopState("test", VisionSections, 10)
	state.OpenQuestions = []Question{
		{ID: "q1", Text: "Who are the target users?"},
	}
	state.KnownFacts = []string{"The target users are small development teams"}

	MergeQuestions(state, []Question{
		{ID: "q2", Text: "Who are the target users for this product?"}, // similar (substring)
		{ID: "q3", Text: "What deployment model is planned?"},          // genuinely new
	})

	if len(state.OpenQuestions) != 2 {
		t.Errorf("expected 2 questions (q2 filtered as similar), got %d", len(state.OpenQuestions))
	}
}

func TestParseQuestionScores(t *testing.T) {
	// Valid JSON
	scores, err := parseQuestionScores(`[{"relevance": 0.8, "novelty": 0.5, "impact": 0.9}]`, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(scores) != 1 || scores[0].Impact != 0.9 {
		t.Errorf("unexpected scores: %v", scores)
	}

	// JSON wrapped in text
	scores, err = parseQuestionScores(`Here are the scores: [{"relevance": 0.7, "novelty": 0.6, "impact": 0.8}]`, 1)
	if err != nil {
		t.Fatalf("expected fallback parse to work, got %v", err)
	}
	if scores[0].Relevance != 0.7 {
		t.Errorf("unexpected relevance: %f", scores[0].Relevance)
	}

	// Garbage
	_, err = parseQuestionScores("no json here", 1)
	if err == nil {
		t.Error("expected error for unparseable response")
	}
}

func TestSelectHighestImpactQuestionWithScore(t *testing.T) {
	state := NewLoopState("test", VisionSections, 10)
	state.OpenQuestions = []Question{
		{ID: "q1", Text: "Low score?", Score: 0.3},
		{ID: "q2", Text: "High score?", Score: 0.95},
		{ID: "q3", Text: "Medium score?", Score: 0.6},
	}

	q := SelectHighestImpactQuestion(state)
	if q == nil || q.ID != "q2" {
		t.Errorf("expected q2 (highest score), got %v", q)
	}
}

func TestNormalizeText(t *testing.T) {
	got := normalizeText("Who are the TARGET users?!")
	expected := "who are the target users"
	if got != expected {
		t.Errorf("normalizeText: got %q, want %q", got, expected)
	}
}

func TestParseFactListFiltersCodeFences(t *testing.T) {
	input := "```\n- Project Name\n- Current Step\n- Entity ID\n```"
	facts := parseFactList(input)
	for _, f := range facts {
		if strings.Contains(f, "```") {
			t.Errorf("code fence should have been filtered: %q", f)
		}
	}
	if len(facts) != 3 {
		t.Errorf("expected 3 facts, got %d: %v", len(facts), facts)
	}
}

func TestParseFactListMinContent(t *testing.T) {
	input := "- real fact here\n- --\n- ?\n- ab\n- ***"
	facts := parseFactList(input)
	if len(facts) != 1 {
		t.Errorf("expected 1 fact (others too short/punctuation), got %d: %v", len(facts), facts)
	}
	if len(facts) > 0 && facts[0] != "real fact here" {
		t.Errorf("expected 'real fact here', got %q", facts[0])
	}
}

func TestValidateFacts(t *testing.T) {
	source := "The JSON payload will include project name, current step, and entity ID"

	// Good facts — derived from source
	good := []string{"project name", "current step", "entity ID"}
	if !validateFacts(good, source) {
		t.Error("expected valid facts to pass validation")
	}

	// Hallucinated facts — unrelated to source
	bad := []string{
		"The shortest war in history was between Britain and Zanzibar",
		"COVID-19 pandemic has accelerated remote work",
	}
	if validateFacts(bad, source) {
		t.Error("expected hallucinated facts to fail validation")
	}

	// Empty facts
	if validateFacts(nil, source) {
		t.Error("expected nil facts to fail validation")
	}
}

func TestDeterministicExtractFacts(t *testing.T) {
	// Bullet list
	facts := deterministicExtractFacts("- project name\n- current step\n- entity ID")
	if len(facts) != 3 {
		t.Errorf("bullet list: expected 3 facts, got %d: %v", len(facts), facts)
	}

	// Comma-separated
	facts = deterministicExtractFacts("project name, current step, entity ID")
	if len(facts) != 3 {
		t.Errorf("comma-separated: expected 3 facts, got %d: %v", len(facts), facts)
	}

	// Single value
	facts = deterministicExtractFacts("just one thing")
	if len(facts) != 1 || facts[0] != "just one thing" {
		t.Errorf("single value: expected ['just one thing'], got %v", facts)
	}

	// Empty/too short
	facts = deterministicExtractFacts("ab")
	if len(facts) != 0 {
		t.Errorf("too short: expected 0 facts, got %d: %v", len(facts), facts)
	}
}
