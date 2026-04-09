package refinement

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// llmCall performs a single-turn LLM call and collects the full response.
// It drains the stream channel to build the complete text.
// We deliberately leave Stream unset (defaults to streaming) because some
// providers (e.g. Ollama) drop content in the done-line when stream=false.
//
// stepName identifies this call in the trace log (e.g. "summarize", "classify_fact_2").
func (c *Controller) llmCall(ctx context.Context, stepName, systemPrompt, userMessage string) (string, int, error) {
	start := time.Now()
	temp := floatPtr(0.3)
	if c.temperature != nil {
		temp = c.temperature
	}

	events, err := c.prov.Chat(ctx, provider.ChatRequest{
		Model:        c.modelID,
		SystemPrompt: systemPrompt,
		UserMessage:  userMessage,
		ProjectDir:   c.projectDir,
		Stage:        c.stage,
		Temperature:  temp,
		NumCtx:       c.numCtx,
	})
	if err != nil {
		c.trace.LogCall(stepName, string(c.currentPhase), c.currentIteration, systemPrompt, userMessage, "", "", 0, time.Since(start), err)
		return "", 0, fmt.Errorf("llm call: %w", err)
	}

	var sb strings.Builder
	var tokens int
	for event := range events {
		switch event.Type {
		case "chunk":
			sb.WriteString(event.Content)
		case "done":
			if event.Content != "" {
				sb.Reset()
				sb.WriteString(event.Content)
			}
		case "tokens":
			var n int
			fmt.Sscanf(event.Content, "%d", &n)
			tokens += n
		case "error":
			callErr := fmt.Errorf("llm error: %s", event.Content)
			c.trace.LogCall(stepName, string(c.currentPhase), c.currentIteration, systemPrompt, userMessage, event.Content, "", tokens, time.Since(start), callErr)
			return "", tokens, callErr
		}
	}
	resp := strings.TrimSpace(sb.String())
	c.trace.LogCall(stepName, string(c.currentPhase), c.currentIteration, systemPrompt, userMessage, resp, "", tokens, time.Since(start), nil)
	return resp, tokens, nil
}

// doSummarize calls the LLM to produce a 2-sentence summary of the idea.
func (c *Controller) doSummarize(ctx context.Context, state *LoopState) (int, error) {
	sys, usr := c.prompts.Summarize(state.IdeaRaw)
	resp, tokens, err := c.llmCall(ctx, "summarize", sys, usr)
	if err != nil {
		return tokens, err
	}
	state.IdeaSummary = resp
	return tokens, nil
}

// doGenerateQuestions calls the LLM to produce initial gap questions.
func (c *Controller) doGenerateQuestions(ctx context.Context, state *LoopState) (int, error) {
	sys, usr := c.prompts.GenerateQuestions(state)
	resp, tokens, err := c.llmCall(ctx, "generate_questions", sys, usr)
	if err != nil {
		return tokens, err
	}
	questions := parseQuestionList(resp, state)
	MergeQuestions(state, questions)
	return tokens, nil
}

// doExtractFacts extracts atomic facts from an answer.
// It validates LLM output against the source and falls back to deterministic
// extraction if the LLM hallucinated content not present in the answer.
func (c *Controller) doExtractFacts(ctx context.Context, summary, answer string) ([]string, int, error) {
	sys, usr := c.prompts.ExtractFacts(summary, answer)
	resp, tokens, err := c.llmCall(ctx, "extract_facts", sys, usr)
	if err != nil {
		// LLM failed entirely — use deterministic fallback
		return deterministicExtractFacts(answer), tokens, nil
	}
	facts := parseFactList(resp)
	if !validateFacts(facts, answer) {
		// LLM hallucinated — use deterministic fallback
		log.Printf("refinement: extract_facts validation failed, using deterministic fallback")
		return deterministicExtractFacts(answer), tokens, nil
	}
	return facts, tokens, nil
}

// validateFacts checks that extracted facts have reasonable overlap with the source answer.
// Returns true if >50% of facts share significant word overlap with the source.
func validateFacts(facts []string, sourceAnswer string) bool {
	if len(facts) == 0 {
		return false
	}
	sourceWords := strings.Fields(strings.ToLower(sourceAnswer))
	valid := 0
	for _, fact := range facts {
		factWords := strings.Fields(strings.ToLower(fact))
		if len(factWords) == 0 {
			continue
		}
		matches := 0
		for _, w := range factWords {
			if len(w) > 2 {
				for _, sw := range sourceWords {
					if strings.Contains(sw, w) || strings.Contains(w, sw) {
						matches++
						break
					}
				}
			}
		}
		if float64(matches)/float64(len(factWords)) >= 0.3 {
			valid++
		}
	}
	return float64(valid)/float64(len(facts)) > 0.5
}

// deterministicExtractFacts splits an answer into facts using punctuation and structure.
// Used as fallback when LLM extraction produces hallucinated content.
func deterministicExtractFacts(answer string) []string {
	// Try splitting on newlines first (handles bullet lists)
	lines := strings.Split(answer, "\n")
	var facts []string
	for _, line := range lines {
		cleaned := strings.TrimSpace(line)
		cleaned = strings.TrimLeft(cleaned, "-*•")
		cleaned = trimNumberedPrefix(cleaned)
		cleaned = strings.TrimSpace(cleaned)
		if len(cleaned) >= 3 && containsAlphanumeric(cleaned) {
			facts = append(facts, cleaned)
		}
	}
	if len(facts) > 1 {
		return facts
	}
	// Single line — try splitting on commas/semicolons
	parts := strings.FieldsFunc(answer, func(r rune) bool {
		return r == ',' || r == ';'
	})
	facts = nil
	for _, p := range parts {
		cleaned := strings.TrimSpace(p)
		if len(cleaned) >= 3 && containsAlphanumeric(cleaned) {
			facts = append(facts, cleaned)
		}
	}
	if len(facts) > 0 {
		return facts
	}
	// Last resort: the whole answer is one fact
	if len(strings.TrimSpace(answer)) >= 3 {
		return []string{strings.TrimSpace(answer)}
	}
	return nil
}

// doNormalizeAnswer rewrites a vague answer into explicit statements using
// the project summary and question as context. Returns the original answer
// unchanged on any error (soft failure).
func (c *Controller) doNormalizeAnswer(ctx context.Context, summary, question, answer string) (string, int, error) {
	sys, usr := c.prompts.NormalizeAnswer(summary, question, answer)
	resp, tokens, err := c.llmCall(ctx, "normalize_answer", sys, usr)
	if err != nil || strings.TrimSpace(resp) == "" {
		return answer, tokens, err
	}
	return resp, tokens, nil
}

// doEvaluateQuestions batch-evaluates unanswered questions for relevance,
// novelty, and impact. Returns the questions with scores populated.
// Soft failure: on parse error, assigns Score=0.5 to all (pass-through).
func (c *Controller) doEvaluateQuestions(ctx context.Context, state *LoopState, questions []Question) ([]Question, int, error) {
	sys, usr := c.prompts.EvaluateQuestions(state, questions)
	resp, tokens, err := c.llmCall(ctx, "evaluate_questions", sys, usr)
	if err != nil {
		// Soft failure: assign default scores
		for i := range questions {
			questions[i].Score = 0.5
		}
		return questions, tokens, nil
	}
	scores, parseErr := parseQuestionScores(resp, len(questions))
	if parseErr != nil {
		for i := range questions {
			questions[i].Score = 0.5
		}
		return questions, tokens, nil
	}
	for i := range questions {
		if i < len(scores) {
			questions[i].Relevance = scores[i].Relevance
			questions[i].Novelty = scores[i].Novelty
			questions[i].Impact = scores[i].Impact
			questions[i].Score = 0.5*scores[i].Impact + 0.3*scores[i].Relevance + 0.2*scores[i].Novelty
		} else {
			questions[i].Score = 0.5
		}
	}
	return questions, tokens, nil
}

// doRewriteQuestion rewrites a vague question to be more specific.
// Soft failure: returns the original question on error.
func (c *Controller) doRewriteQuestion(ctx context.Context, state *LoopState, question string) (string, int, error) {
	sys, usr := c.prompts.RewriteQuestion(state, question)
	resp, tokens, err := c.llmCall(ctx, "rewrite_question", sys, usr)
	if err != nil || strings.TrimSpace(resp) == "" {
		return question, tokens, nil
	}
	return strings.TrimSpace(resp), tokens, nil
}

// doCoherenceCheck verifies newly added facts are consistent with existing state.
// Returns true if coherent. If incoherent, parses contradictions into corrective
// questions and merges them into state. Soft failure: returns true on error.
// Also returns review items for concerns surfaced to the user.
func (c *Controller) doCoherenceCheck(ctx context.Context, state *LoopState, newFacts []string) (bool, []ReviewItem, int, error) {
	sys, usr := c.prompts.CoherenceCheck(state, newFacts)
	resp, tokens, err := c.llmCall(ctx, "coherence_check", sys, usr)
	if err != nil {
		return true, nil, tokens, nil // soft failure: assume coherent
	}
	if strings.TrimSpace(strings.ToUpper(resp)) == "COHERENT" {
		return true, nil, tokens, nil
	}
	// Incoherent: parse contradictions as corrective questions
	questions := parseQuestionList(resp, state)
	var items []ReviewItem
	for i := range questions {
		questions[i].Source = "coherence"
		items = append(items, ReviewItem{
			ID:        fmt.Sprintf("rv-%d-%d-coh", state.Iteration, i),
			Text:      questions[i].Text,
			Source:    "coherence",
			Section:   questions[i].Section,
			Iteration: state.Iteration,
			Status:    ReviewPending,
			CreatedAt: time.Now(),
		})
	}
	MergeQuestions(state, questions)
	return false, items, tokens, nil
}

// doTensionCheck challenges the product vision and generates adversarial questions.
// Soft failure: skips on error. Also returns review items for the review queue.
func (c *Controller) doTensionCheck(ctx context.Context, state *LoopState) ([]ReviewItem, int, error) {
	sys, usr := c.prompts.TensionCheck(state)
	resp, tokens, err := c.llmCall(ctx, "tension_check", sys, usr)
	if err != nil {
		return nil, tokens, nil // soft failure
	}
	if strings.TrimSpace(strings.ToUpper(resp)) == "NONE" {
		return nil, tokens, nil
	}
	questions := parseQuestionList(resp, state)
	var items []ReviewItem
	for i := range questions {
		questions[i].Source = "tension"
		items = append(items, ReviewItem{
			ID:        fmt.Sprintf("rv-%d-%d-ten", state.Iteration, i),
			Text:      questions[i].Text,
			Source:    "tension",
			Section:   questions[i].Section,
			Iteration: state.Iteration,
			Status:    ReviewPending,
			CreatedAt: time.Now(),
		})
	}
	MergeQuestions(state, questions)
	return items, tokens, nil
}

// doClassifyFact determines which section a fact belongs to.
func (c *Controller) doClassifyFact(ctx context.Context, fact string, factIdx int) (SectionName, int, error) {
	sections := c.prompts.Sections()
	sys, usr := c.prompts.ClassifyFact(fact, sections)
	resp, tokens, err := c.llmCall(ctx, fmt.Sprintf("classify_fact_%d", factIdx), sys, usr)
	if err != nil {
		return "", tokens, err
	}
	return parseSectionName(resp, sections), tokens, nil
}

// doMergeFact merges a fact into a section's content.
func (c *Controller) doMergeFact(ctx context.Context, summary, existing, fact string, factIdx int) (string, int, error) {
	sys, usr := c.prompts.MergeFact(summary, existing, fact)
	resp, tokens, err := c.llmCall(ctx, fmt.Sprintf("merge_fact_%d", factIdx), sys, usr)
	if err != nil {
		return existing, tokens, err
	}
	return resp, tokens, nil
}

// doScoreSection scores a single section's completeness.
func (c *Controller) doScoreSection(ctx context.Context, name SectionName, content string) (float64, int, error) {
	sys, usr := c.prompts.ScoreSection(name, content)
	resp, tokens, err := c.llmCall(ctx, fmt.Sprintf("score_%s", name), sys, usr)
	if err != nil {
		return 0, tokens, err
	}
	return parseFloat(resp), tokens, nil
}

// doScoreAllSections scores all sections sequentially (safe for small Ollama instances).
func (c *Controller) doScoreAllSections(ctx context.Context, state *LoopState) (int, error) {
	var totalTokens int
	for _, s := range c.prompts.Sections() {
		score, tokens, err := c.doScoreSection(ctx, s, state.Sections[s])
		totalTokens += tokens
		if err != nil {
			return totalTokens, err
		}
		state.Confidence[s] = score
	}
	return totalTokens, nil
}

// doFindGaps asks the LLM to identify remaining gaps.
// Also returns review items for the review queue.
func (c *Controller) doFindGaps(ctx context.Context, state *LoopState) ([]ReviewItem, int, error) {
	sys, usr := c.prompts.FindGaps(state)
	resp, tokens, err := c.llmCall(ctx, "find_gaps", sys, usr)
	if err != nil {
		return nil, tokens, err
	}
	if strings.TrimSpace(strings.ToUpper(resp)) == "NONE" {
		return nil, tokens, nil
	}
	questions := parseQuestionList(resp, state)
	var items []ReviewItem
	for i := range questions {
		questions[i].Source = "gap"
		items = append(items, ReviewItem{
			ID:        fmt.Sprintf("rv-%d-%d-gap", state.Iteration, i),
			Text:      questions[i].Text,
			Source:    "gap",
			Section:   questions[i].Section,
			Iteration: state.Iteration,
			Status:    ReviewPending,
			CreatedAt: time.Now(),
		})
	}
	MergeQuestions(state, questions)
	return items, tokens, nil
}

// doSimulateAnswer generates an answer for autonomous mode.
func (c *Controller) doSimulateAnswer(ctx context.Context, state *LoopState, question string) (string, int, error) {
	sys, usr := c.prompts.SimulateAnswer(state, question)
	return c.llmCall(ctx, "simulate_answer", sys, usr)
}

// doCritique runs a self-critique pass on the current state.
// Also returns review items for the review queue.
func (c *Controller) doCritique(ctx context.Context, state *LoopState) ([]ReviewItem, int, error) {
	sys, usr := c.prompts.Critique(state)
	resp, tokens, err := c.llmCall(ctx, "critique", sys, usr)
	if err != nil {
		return nil, tokens, err
	}
	if strings.TrimSpace(strings.ToUpper(resp)) == "NONE" {
		return nil, tokens, nil
	}
	questions := parseQuestionList(resp, state)
	var items []ReviewItem
	for i := range questions {
		questions[i].Source = "critique"
		items = append(items, ReviewItem{
			ID:        fmt.Sprintf("rv-%d-%d-cri", state.Iteration, i),
			Text:      questions[i].Text,
			Source:    "critique",
			Section:   questions[i].Section,
			Iteration: state.Iteration,
			Status:    ReviewPending,
			CreatedAt: time.Now(),
		})
	}
	MergeQuestions(state, questions)
	return items, tokens, nil
}

// doSynthesize generates the final artifact from accumulated state.
func (c *Controller) doSynthesize(ctx context.Context, state *LoopState) (string, int, error) {
	sys, usr := c.prompts.Synthesize(state)
	return c.llmCall(ctx, "synthesize", sys, usr)
}

// --- Response Parsers ---

// questionScore holds the parsed scores from an EvaluateQuestions response.
type questionScore struct {
	Relevance float64 `json:"relevance"`
	Novelty   float64 `json:"novelty"`
	Impact    float64 `json:"impact"`
}

// parseQuestionScores parses a JSON array of question scores from LLM output.
// Falls back to regex extraction if direct unmarshal fails.
func parseQuestionScores(resp string, count int) ([]questionScore, error) {
	resp = strings.TrimSpace(resp)

	// Try direct unmarshal
	var scores []questionScore
	if err := json.Unmarshal([]byte(resp), &scores); err == nil && len(scores) > 0 {
		return scores, nil
	}

	// Fallback: find JSON array in response text
	start := strings.Index(resp, "[")
	end := strings.LastIndex(resp, "]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(resp[start:end+1]), &scores); err == nil && len(scores) > 0 {
			return scores, nil
		}
	}

	return nil, fmt.Errorf("could not parse question scores from response")
}

var floatRe = regexp.MustCompile(`(\d+\.?\d*)`)

// parseFloat extracts a float from messy LLM output. Falls back to 0.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	// Try direct parse first
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return clampFloat(f)
	}
	// Regex fallback
	if m := floatRe.FindString(s); m != "" {
		if f, err := strconv.ParseFloat(m, 64); err == nil {
			return clampFloat(f)
		}
	}
	return 0
}

func clampFloat(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// isCodeFence returns true if the line consists only of backticks or tildes.
func isCodeFence(s string) bool {
	return strings.TrimSpace(strings.Trim(s, "`~")) == ""
}

// isPreamble returns true if the line looks like LLM preamble rather than a fact.
func isPreamble(cleaned string) bool {
	if strings.HasSuffix(cleaned, ":") {
		return true
	}
	lower := strings.ToLower(cleaned)
	return strings.HasPrefix(lower, "here are") ||
		strings.HasPrefix(lower, "the following") ||
		strings.HasPrefix(lower, "based on")
}

// containsAlphanumeric returns true if s contains at least one letter or digit.
func containsAlphanumeric(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}

// parseFactList parses bullet-point facts from LLM output.
// Filters out preamble lines by only keeping lines that were originally
// bulleted/numbered or are short enough to be atomic facts.
func parseFactList(s string) []string {
	lines := strings.Split(s, "\n")
	var facts []string
	for _, line := range lines {
		raw := strings.TrimSpace(line)
		if raw == "" || isCodeFence(raw) {
			continue
		}

		isNumbered := numberedRe.MatchString(raw)
		isBulleted := len(raw) > 0 && (raw[0] == '-' || raw[0] == '*' || strings.HasPrefix(raw, "• "))

		cleaned := strings.TrimLeft(raw, "-*• ")
		cleaned = trimNumberedPrefix(cleaned)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" || len(cleaned) < 3 || !containsAlphanumeric(cleaned) {
			continue
		}

		if !isNumbered && !isBulleted && isPreamble(cleaned) {
			continue
		}

		facts = append(facts, cleaned)
	}
	return facts
}

var numberedRe = regexp.MustCompile(`^\d+[\.\)]\s*`)

func trimNumberedPrefix(s string) string {
	return numberedRe.ReplaceAllString(s, "")
}

// parseSectionName fuzzy-matches LLM output to a known section name.
func parseSectionName(s string, sections []SectionName) SectionName {
	s = strings.TrimSpace(strings.ToLower(s))
	// Direct match
	for _, sec := range sections {
		if s == string(sec) {
			return sec
		}
	}
	// Contains match
	for _, sec := range sections {
		if strings.Contains(s, string(sec)) {
			return sec
		}
	}
	// Partial match fallback
	for _, sec := range sections {
		if strings.Contains(s, strings.ReplaceAll(string(sec), "_", " ")) {
			return sec
		}
	}
	// Default to first section
	if len(sections) > 0 {
		return sections[0]
	}
	return SectionProblem
}

// parseQuestionList parses numbered or bulleted questions from LLM output
// and assigns them to the lowest-confidence sections.
// It filters out preamble lines (e.g. "Here are 5 questions:") by only
// keeping lines that were originally numbered/bulleted or end with "?".
func parseQuestionList(s string, state *LoopState) []Question {
	lines := strings.Split(s, "\n")
	var questions []Question
	sections := VisionSections
	qIdx := 0

	for _, line := range lines {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.ToUpper(raw) == "NONE" {
			continue
		}

		// Check if this line was originally numbered or bulleted before stripping.
		isNumbered := numberedRe.MatchString(raw)
		isBulleted := len(raw) > 0 && (raw[0] == '-' || raw[0] == '*' || strings.HasPrefix(raw, "• "))

		// Strip prefixes to get the question text.
		cleaned := strings.TrimLeft(raw, "-*• ")
		cleaned = trimNumberedPrefix(cleaned)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}

		// Only keep lines that were numbered/bulleted or end with "?".
		// This filters out LLM preamble like "Here are 5 critical unknowns:"
		if !isNumbered && !isBulleted && !strings.HasSuffix(cleaned, "?") {
			continue
		}

		section := lowestConfidenceSection(state, sections)
		questions = append(questions, Question{
			ID:      fmt.Sprintf("q-%d-%d", state.Iteration, qIdx),
			Text:    cleaned,
			Section: section,
			Impact:  1.0 - state.Confidence[section],
		})
		qIdx++
	}
	return questions
}

// lowestConfidenceSection returns the section with the lowest current confidence.
func lowestConfidenceSection(state *LoopState, sections []SectionName) SectionName {
	best := sections[0]
	bestConf := state.Confidence[best]
	for _, s := range sections[1:] {
		c := state.Confidence[s]
		if c < bestConf {
			bestConf = c
			best = s
		}
	}
	return best
}

// --- Helpers ---

func floatPtr(f float64) *float64 { return &f }

// emitEvent is a convenience for building LoopEvents.
func emitEvent(phase LoopPhase, state *LoopState, msg string, question *Question) LoopEvent {
	return LoopEvent{
		Phase:      phase,
		Iteration:  state.Iteration,
		MaxIter:    state.MaxIterations,
		Confidence: copyConfidence(state.Confidence),
		Question:   question,
		Message:    msg,
	}
}

func copyConfidence(m map[SectionName]float64) map[SectionName]float64 {
	out := make(map[SectionName]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// StreamEvent re-exports for convenience within this package.
type StreamEvent = model.StreamEvent
