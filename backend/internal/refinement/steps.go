package refinement

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
// doExtractFacts splits an answer into atomic facts deterministically.
// No LLM call — small models consistently produce single-word garbage or
// hallucinated content. The deterministic splitter handles newlines, commas,
// and semicolons reliably.
func (c *Controller) doExtractFacts(_ context.Context, _, answer string) ([]string, int, error) {
	facts := deterministicExtractFacts(answer)
	return filterAndCapFacts(facts), 0, nil
}

// maxFactsPerAnswer is the hard cap on facts extracted from a single answer.
// Small models often ignore the prompt's "Maximum 5 items" instruction.
const maxFactsPerAnswer = 5

// minFactWords is the minimum word count for a fact to be considered meaningful.
// Labels like "Target Audience" or "Game Mode" are too vague for vision-level extraction.
const minFactWords = 3

// filterAndCapFacts removes facts that are too short (labels, single words) and
// enforces the hard cap. This prevents small models from generating dozens of
// trivial facts that each require a classify+merge LLM call.
func filterAndCapFacts(facts []string) []string {
	var kept []string
	for _, f := range facts {
		if len(strings.Fields(f)) >= minFactWords {
			kept = append(kept, f)
		}
	}
	if len(kept) > maxFactsPerAnswer {
		kept = kept[:maxFactsPerAnswer]
	}
	return kept
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

// doNormalizeAnswer passes the raw answer through unchanged.
// Small models consistently hallucinate during normalization (e.g. "any casual
// gamer" → 6 invented demographics, "no monetization" → invented ad/IAP details).
// The raw answer feeds into deterministicExtractFacts which handles splitting.
func (c *Controller) doNormalizeAnswer(_ context.Context, _, _, answer string) (string, int, error) {
	return answer, 0, nil
}

// wordOverlap returns the fraction of words (len>3) in text that appear in corpus.
func wordOverlap(text, corpus string) float64 {
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 || corpus == "" {
		return 0
	}
	lowerCorpus := strings.ToLower(corpus)
	matches := 0
	for _, w := range words {
		if len(w) > 3 && strings.Contains(lowerCorpus, w) {
			matches++
		}
	}
	return math.Min(float64(matches)/float64(len(words)), 1.0)
}

// doEvaluateQuestions scores questions deterministically using heuristics:
//   - Impact: inverse of section confidence (weak sections need more info)
//   - Relevance: keyword overlap between question and section content
//   - Novelty: inverse word overlap with known facts
func (c *Controller) doEvaluateQuestions(_ context.Context, state *LoopState, questions []Question) ([]Question, int, error) {
	factCorpus := strings.Join(state.KnownFacts, " ")
	for i := range questions {
		q := &questions[i]
		q.Impact = 1.0 - state.Confidence[q.Section]

		sectionContent := state.Sections[q.Section]
		if sectionContent != "" {
			q.Relevance = wordOverlap(q.Text, sectionContent)
		} else {
			q.Relevance = 0.5
		}

		if len(state.KnownFacts) > 0 {
			q.Novelty = 1.0 - wordOverlap(q.Text, factCorpus)
		} else {
			q.Novelty = 1.0
		}

		q.Score = 0.5*q.Impact + 0.3*q.Relevance + 0.2*q.Novelty
	}
	return questions, 0, nil
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

// negationPhrases are indicators that a fact expresses exclusion or denial.
var negationPhrases = []string{"not ", "no ", "won't ", "will not ", "without ", "never ", "exclude", "don't "}

// isNegation returns true if the text contains a negation phrase.
func isNegation(lower string) bool {
	for _, neg := range negationPhrases {
		if strings.Contains(lower, neg) {
			return true
		}
	}
	return false
}

// stripNegations removes negation phrases from text to extract the subject.
func stripNegations(s string) string {
	for _, neg := range negationPhrases {
		s = strings.ReplaceAll(s, neg, "")
	}
	return s
}

// checkFactCoherence checks a single fact for contradictions against features
// and out_of_scope content, returning any review items found.
func checkFactCoherence(fact, featContent, oosContent string, iteration int) []ReviewItem {
	lower := strings.ToLower(fact)
	neg := isNegation(lower)
	var items []ReviewItem

	if neg && featContent != "" {
		subject := stripNegations(lower)
		if len(subject) > 3 && wordOverlap(subject, featContent) > 0.4 {
			items = append(items, ReviewItem{
				ID: fmt.Sprintf("rv-%d-coh-neg", iteration), Source: "coherence",
				Text: fmt.Sprintf("Possible contradiction: '%s' conflicts with features content", fact),
				Section: SectionFeatures, Iteration: iteration, Status: ReviewPending, CreatedAt: time.Now(),
			})
		}
	}

	if !neg && oosContent != "" && wordOverlap(fact, oosContent) > 0.5 {
		items = append(items, ReviewItem{
			ID: fmt.Sprintf("rv-%d-coh-oos", iteration), Source: "coherence",
			Text: fmt.Sprintf("Possible contradiction: '%s' overlaps with out-of-scope items", fact),
			Section: SectionOutOfScope, Iteration: iteration, Status: ReviewPending, CreatedAt: time.Now(),
		})
	}
	return items
}

// doCoherenceCheck verifies newly added facts are consistent with existing state.
// Deterministic: checks for structural contradictions (negated facts appearing
// in feature-positive sections, out_of_scope items in features, etc.).
func (c *Controller) doCoherenceCheck(_ context.Context, state *LoopState, newFacts []string) (bool, []ReviewItem, int, error) {
	featContent := strings.ToLower(state.Sections[SectionFeatures])
	oosContent := strings.ToLower(state.Sections[SectionOutOfScope])

	var items []ReviewItem
	for _, fact := range newFacts {
		items = append(items, checkFactCoherence(fact, featContent, oosContent, state.Iteration)...)
	}
	return len(items) == 0, items, 0, nil
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
	// Cap to 2 — small models ignore the "up to 2" prompt instruction
	if len(questions) > 2 {
		questions = questions[:2]
	}
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

// sectionKeywords maps each section to keywords that signal a fact belongs there.
var sectionKeywords = map[SectionName][]string{
	SectionProblem:     {"problem", "issue", "challenge", "pain", "struggle", "need", "gap", "frustrat", "difficult"},
	SectionUsers:       {"user", "player", "gamer", "audience", "customer", "demographic", "age", "target", "people", "who"},
	SectionFeatures:    {"feature", "mode", "level", "power-up", "ability", "gameplay", "mechanic", "sound", "animation", "effect", "wave", "balloon", "collision", "physic", "speed", "drift"},
	SectionUX:          {"interface", "ui", "ux", "screen", "layout", "design", "click", "tap", "interaction", "experience", "visual", "display"},
	SectionMetrics:     {"metric", "measure", "score", "kpi", "success", "track", "analytics", "retention", "engagement", "percent", "rate"},
	SectionConstraints: {"constraint", "limit", "budget", "timeline", "platform", "technology", "cost", "monetiz", "free", "device", "browser"},
	SectionOutOfScope:  {"scope", "exclude", "defer", "later", "won't", "will not", "no ", "not ", "without"},
}

// doClassifyFact determines which section a fact belongs to using keyword matching.
// Deterministic — no LLM call. Falls back to lowest-confidence section on no match.
func (c *Controller) doClassifyFact(_ context.Context, fact string, _ int) (SectionName, int, error) {
	sections := c.prompts.Sections()
	lower := strings.ToLower(fact)

	bestSection := SectionName("")
	bestCount := 0
	for _, section := range sections {
		keywords := sectionKeywords[section]
		count := 0
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestSection = section
		}
	}
	if bestSection == "" {
		bestSection = lowestConfidenceSection(&LoopState{Confidence: map[SectionName]float64{}}, sections)
	}
	return bestSection, 0, nil
}

// doMergeFact appends a fact to a section's content as a bullet point.
// This is deterministic — no LLM call — to prevent hallucination snowball
// where the model generates creative writing around facts (e.g. "scientists
// baffled by bizarre phenomenon"). The final synthesize step produces the
// polished narrative from the accumulated bullet points.
func (c *Controller) doMergeFact(_ context.Context, _, existing, fact string, _ int) (string, int, error) {
	if strings.TrimSpace(existing) == "" {
		return "- " + fact, 0, nil
	}
	return existing + "\n- " + fact, 0, nil
}

// deterministicScoreSection scores a section based on bullet count and word count.
// Sections are bullet-point lists (from deterministic merge), so counting bullets
// is a reliable measure of completeness.
func deterministicScoreSection(content string) float64 {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0.0
	}
	// Count bullets: first bullet starts with "- ", subsequent have "\n- "
	bullets := 0
	if strings.HasPrefix(content, "- ") {
		bullets = 1 + strings.Count(content, "\n- ")
	} else {
		bullets = strings.Count(content, "\n- ")
		if bullets == 0 {
			bullets = 1 // non-empty, non-bulleted content counts as 1
		}
	}
	words := len(strings.Fields(content))

	bulletScore := math.Min(float64(bullets)/3.0, 1.0) // 3+ bullets = full breadth
	wordScore := math.Min(float64(words)/15.0, 1.0)   // 15+ words = full depth
	return 0.6*bulletScore + 0.4*wordScore
}

// doScoreSection scores a single section's completeness deterministically.
func (c *Controller) doScoreSection(_ context.Context, _ SectionName, content string) (float64, int, error) {
	return deterministicScoreSection(content), 0, nil
}

// doScoreAllSections scores all sections deterministically.
func (c *Controller) doScoreAllSections(_ context.Context, state *LoopState) (int, error) {
	for _, s := range c.prompts.Sections() {
		state.Confidence[s] = deterministicScoreSection(state.Sections[s])
	}
	return 0, nil
}

// doFindGaps identifies remaining gaps deterministically by checking section
// completeness. Generates template questions for empty or thin sections.
func (c *Controller) doFindGaps(_ context.Context, state *LoopState) ([]ReviewItem, int, error) {
	var questions []Question
	var items []ReviewItem
	idx := 0

	for _, section := range c.prompts.Sections() {
		content := strings.TrimSpace(state.Sections[section])
		conf := state.Confidence[section]

		var text string
		switch {
		case content == "":
			text = fmt.Sprintf("What is the %s for this product?", section)
		case conf < 0.4:
			text = fmt.Sprintf("Can you provide more detail about %s?", section)
		default:
			continue
		}

		q := Question{
			ID:      fmt.Sprintf("q-%d-gap-%d", state.Iteration, idx),
			Text:    text,
			Section: section,
			Impact:  1.0 - conf,
			Source:  "gap",
		}
		questions = append(questions, q)
		items = append(items, ReviewItem{
			ID: fmt.Sprintf("rv-%d-%d-gap", state.Iteration, idx), Source: "gap",
			Text: text, Section: section, Iteration: state.Iteration,
			Status: ReviewPending, CreatedAt: time.Now(),
		})
		idx++
	}

	MergeQuestions(state, questions)
	return items, 0, nil
}

// doSimulateAnswer generates an answer for autonomous mode.
func (c *Controller) doSimulateAnswer(ctx context.Context, state *LoopState, question string) (string, int, error) {
	sys, usr := c.prompts.SimulateAnswer(state, question)
	return c.llmCall(ctx, "simulate_answer", sys, usr)
}

// doCritique checks for structural issues deterministically.
// Flags sections that lack specificity (e.g., metrics without numbers).
func (c *Controller) doCritique(_ context.Context, state *LoopState) ([]ReviewItem, int, error) {
	var items []ReviewItem

	// Check: metrics section exists but has no numbers
	metricsContent := state.Sections[SectionMetrics]
	if metricsContent != "" && !containsDigit(metricsContent) {
		items = append(items, ReviewItem{
			ID: fmt.Sprintf("rv-%d-cri-metrics", state.Iteration), Source: "critique",
			Text: "Metrics section has no numeric targets — consider adding specific numbers",
			Section: SectionMetrics, Iteration: state.Iteration,
			Status: ReviewPending, CreatedAt: time.Now(),
		})
	}

	// Check: features section is large but no constraints defined
	featContent := state.Sections[SectionFeatures]
	constContent := state.Sections[SectionConstraints]
	if len(strings.Fields(featContent)) > 20 && strings.TrimSpace(constContent) == "" {
		items = append(items, ReviewItem{
			ID: fmt.Sprintf("rv-%d-cri-scope", state.Iteration), Source: "critique",
			Text: "Many features listed but no constraints defined — risk of scope creep",
			Section: SectionConstraints, Iteration: state.Iteration,
			Status: ReviewPending, CreatedAt: time.Now(),
		})
	}

	return items, 0, nil
}

// containsDigit returns true if s contains at least one digit.
func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
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

// stripCodeFences removes lines that are code fence markers (``` lines).
func stripCodeFences(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), "```") {
			cleaned = append(cleaned, l)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// extractFirstJSONArray finds the first complete [...] block using bracket
// depth matching and attempts to unmarshal it into the target slice.
func extractFirstJSONArray(s string, target *[]questionScore) bool {
	start := strings.Index(s, "[")
	if start < 0 {
		return false
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				if err := json.Unmarshal([]byte(s[start:i+1]), target); err == nil && len(*target) > 0 {
					return true
				}
				return false
			}
		}
	}
	return false
}

// parseQuestionScores parses a JSON array of question scores from LLM output.
// Strips code fences and extracts the first complete JSON array, so extra
// LLM verbosity after the scores is ignored.
func parseQuestionScores(resp string, count int) ([]questionScore, error) {
	resp = stripCodeFences(resp)

	var scores []questionScore
	if err := json.Unmarshal([]byte(resp), &scores); err == nil && len(scores) > 0 {
		return scores, nil
	}
	if extractFirstJSONArray(resp, &scores) {
		return scores, nil
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
