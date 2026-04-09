// Package refinement implements a closed-loop iterative refinement engine.
// The controller drives many small, focused LLM calls instead of one large one,
// making it suitable for small models (1B-7B) that struggle with complex prompts.
//
// The package is stage-agnostic: Vision is the first consumer, but any pipeline
// stage can plug in by implementing the PromptSet interface with stage-specific
// micro-prompts and section definitions.
package refinement

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// SectionName identifies a domain section within the refinement state.
type SectionName string

// Vision stage sections.
const (
	SectionProblem     SectionName = "problem"
	SectionUsers       SectionName = "users"
	SectionFeatures    SectionName = "features"
	SectionUX          SectionName = "ux"
	SectionMetrics     SectionName = "metrics"
	SectionConstraints SectionName = "constraints"
	SectionOutOfScope  SectionName = "out_of_scope"
)

// VisionSections lists all sections for the Vision stage.
var VisionSections = []SectionName{
	SectionProblem, SectionUsers, SectionFeatures,
	SectionUX, SectionMetrics, SectionConstraints, SectionOutOfScope,
}

// LoopPhase tracks which step of the refinement loop the controller is in.
type LoopPhase string

const (
	PhaseInit            LoopPhase = "init"
	PhaseSummarize       LoopPhase = "summarize"
	PhaseGenerateQs      LoopPhase = "generate_questions"
	PhaseAwaitAnswer     LoopPhase = "await_answer"
	PhaseExtractFacts    LoopPhase = "extract_facts"
	PhaseUpdateSections  LoopPhase = "update_sections"
	PhaseScoreConfidence LoopPhase = "score_confidence"
	PhaseFindGaps        LoopPhase = "find_gaps"
	PhaseEvaluateQs      LoopPhase = "evaluate_questions"
	PhaseCoherence       LoopPhase = "coherence_check"
	PhaseTension         LoopPhase = "tension_check"
	PhaseCritique        LoopPhase = "critique"
	PhaseSynthesize      LoopPhase = "synthesize"
	PhaseComplete        LoopPhase = "complete"
)

// ReviewItem represents an AI-identified concern surfaced for user triage.
// Items are produced by critique, coherence, tension, and gap-finding steps.
type ReviewItem struct {
	ID        string      `json:"id"`
	Text      string      `json:"text"`
	Source    string      `json:"source"`    // "critique", "coherence", "tension", "gap"
	Section   SectionName `json:"section"`
	Iteration int         `json:"iteration"`
	Status    string      `json:"status"`    // "pending", "addressed", "discarded"
	CreatedAt time.Time   `json:"created_at"`
}

// ReviewItem status constants.
const (
	ReviewPending   = "pending"
	ReviewAddressed = "addressed"
	ReviewDiscarded = "discarded"
)

// Question represents a gap that needs to be answered to refine the vision.
type Question struct {
	ID        string      `json:"id"`
	Text      string      `json:"text"`
	Section   SectionName `json:"section"`
	Impact    float64     `json:"impact"`
	Relevance float64     `json:"relevance"`
	Novelty   float64     `json:"novelty"`
	Score     float64     `json:"score"` // Composite: 0.5*Impact + 0.3*Relevance + 0.2*Novelty
	Answered  bool        `json:"answered"`
	Answer    string      `json:"answer,omitempty"`
	Source    string      `json:"source,omitempty"` // "generated", "gap", "critique", "coherence", "tension"
}

// LoopState is the externalized memory for the refinement loop.
// It is persisted to disk after each step so the loop can survive restarts.
type LoopState struct {
	IdeaRaw       string                  `json:"idea_raw"`
	IdeaSummary   string                  `json:"idea_summary"`
	Sections      map[SectionName]string  `json:"sections"`
	KnownFacts    []string                `json:"known_facts"`
	OpenQuestions []Question              `json:"open_questions"`
	Confidence    map[SectionName]float64 `json:"confidence"`
	Iteration     int                     `json:"iteration"`
	MaxIterations int                     `json:"max_iterations"`
	Phase         LoopPhase               `json:"phase"`
	Error         string                  `json:"error,omitempty"`

	// ReviewItems holds AI-identified concerns for user triage (review queue).
	ReviewItems []ReviewItem `json:"review_items"`
}

// NewLoopState creates a fresh state for a given idea and section list.
func NewLoopState(ideaRaw string, sections []SectionName, maxIter int) *LoopState {
	if maxIter <= 0 {
		maxIter = 10
	}
	secs := make(map[SectionName]string, len(sections))
	conf := make(map[SectionName]float64, len(sections))
	for _, s := range sections {
		secs[s] = ""
		conf[s] = 0
	}
	return &LoopState{
		IdeaRaw:       ideaRaw,
		Sections:      secs,
		KnownFacts:    []string{},
		OpenQuestions: []Question{},
		Confidence:    conf,
		MaxIterations: maxIter,
		Phase:         PhaseInit,
		ReviewItems:   []ReviewItem{},
	}
}

const stateFileName = "refinement-state.json"

func statePath(dataDir string) string {
	return filepath.Join(dataDir, "vision", stateFileName)
}

// SaveState writes the loop state atomically to {dataDir}/vision/refinement-state.json.
func SaveState(dataDir string, state *LoopState) error {
	p := statePath(dataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir for state: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	return os.Rename(tmp, p)
}

// LoadState reads the loop state from disk. Returns (nil, nil) if not found.
func LoadState(dataDir string) (*LoopState, error) {
	data, err := os.ReadFile(statePath(dataDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var state LoopState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &state, nil
}

// ClearState removes the state file.
func ClearState(dataDir string) error {
	err := os.Remove(statePath(dataDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// AverageConfidence returns the mean confidence across all sections.
func AverageConfidence(state *LoopState) float64 {
	if len(state.Confidence) == 0 {
		return 0
	}
	var sum float64
	for _, v := range state.Confidence {
		sum += v
	}
	return sum / float64(len(state.Confidence))
}

// OpenQuestionCount returns the number of unanswered questions.
func OpenQuestionCount(state *LoopState) int {
	n := 0
	for _, q := range state.OpenQuestions {
		if !q.Answered {
			n++
		}
	}
	return n
}

// IsConverged returns true if the loop should stop iterating.
func IsConverged(state *LoopState, minConfidence float64) bool {
	if minConfidence <= 0 {
		minConfidence = 0.85
	}
	if AverageConfidence(state) >= minConfidence && OpenQuestionCount(state) == 0 {
		return true
	}
	return state.Iteration >= state.MaxIterations
}

// SelectHighestImpactQuestion picks the unanswered question with the highest
// composite score. Falls back to impact + inverse confidence if scores are unset.
func SelectHighestImpactQuestion(state *LoopState) *Question {
	var best *Question
	bestScore := math.Inf(-1)
	for i := range state.OpenQuestions {
		q := &state.OpenQuestions[i]
		if q.Answered {
			continue
		}
		score := q.Score
		if score <= 0 {
			// Fallback for questions not yet evaluated.
			sectionConf := state.Confidence[q.Section]
			score = q.Impact + (1 - sectionConf)
		}
		if score > bestScore {
			bestScore = score
			best = q
		}
	}
	return best
}

// FilterAndRankQuestions removes questions below the score threshold and
// sorts the rest by score descending.
func FilterAndRankQuestions(questions []Question, threshold float64) []Question {
	kept := make([]Question, 0, len(questions))
	for _, q := range questions {
		if q.Score >= threshold || q.Score <= 0 {
			// Keep: above threshold, or unscored (don't discard unevaluated questions)
			kept = append(kept, q)
		}
	}
	sort.Slice(kept, func(i, j int) bool {
		return kept[i].Score > kept[j].Score
	})
	return kept
}

// collectUnanswered returns a slice of unanswered questions from state.
func collectUnanswered(state *LoopState) []Question {
	var out []Question
	for _, q := range state.OpenQuestions {
		if !q.Answered {
			out = append(out, q)
		}
	}
	return out
}

// applyEvaluatedScores copies scores from evaluated questions back into
// the state's OpenQuestions by matching on question ID.
func applyEvaluatedScores(state *LoopState, evaluated []Question) {
	byID := make(map[string]*Question, len(evaluated))
	for i := range evaluated {
		byID[evaluated[i].ID] = &evaluated[i]
	}
	for i := range state.OpenQuestions {
		if ev, ok := byID[state.OpenQuestions[i].ID]; ok {
			state.OpenQuestions[i].Relevance = ev.Relevance
			state.OpenQuestions[i].Novelty = ev.Novelty
			state.OpenQuestions[i].Impact = ev.Impact
			state.OpenQuestions[i].Score = ev.Score
			state.OpenQuestions[i].Text = ev.Text // may have been rewritten
		}
	}
}

// PruneResolved removes questions whose sections have reached the given threshold.
func PruneResolved(state *LoopState, threshold float64) {
	kept := state.OpenQuestions[:0]
	for _, q := range state.OpenQuestions {
		if q.Answered {
			continue // drop answered
		}
		if state.Confidence[q.Section] >= threshold {
			continue // section is confident enough
		}
		kept = append(kept, q)
	}
	state.OpenQuestions = kept
}

// MergeQuestions adds new questions, deduplicating by exact text and
// normalized similarity (substring containment, word overlap with known facts).
func MergeQuestions(state *LoopState, newQuestions []Question) {
	existing := make(map[string]bool, len(state.OpenQuestions))
	normalizedExisting := make([]string, 0, len(state.OpenQuestions))
	for _, q := range state.OpenQuestions {
		existing[q.Text] = true
		normalizedExisting = append(normalizedExisting, normalizeText(q.Text))
	}
	for _, q := range newQuestions {
		if existing[q.Text] {
			continue // exact match
		}
		norm := normalizeText(q.Text)
		if isSimilarToAny(norm, normalizedExisting) {
			continue // too similar to an existing question
		}
		if isRedundantWithFacts(norm, state.KnownFacts) {
			continue // already covered by known facts
		}
		state.OpenQuestions = append(state.OpenQuestions, q)
		existing[q.Text] = true
		normalizedExisting = append(normalizedExisting, norm)
	}
}

// normalizeText lowercases, strips punctuation, and collapses whitespace.
func normalizeText(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// isSimilarToAny checks if norm is a substring of any existing text or vice versa.
func isSimilarToAny(norm string, existing []string) bool {
	for _, e := range existing {
		if strings.Contains(norm, e) || strings.Contains(e, norm) {
			return true
		}
	}
	return false
}

// isRedundantWithFacts returns true if >70% of the question's words appear
// in any single known fact.
func isRedundantWithFacts(normQuestion string, facts []string) bool {
	words := strings.Fields(normQuestion)
	if len(words) < 3 {
		return false // too short to meaningfully compare
	}
	for _, fact := range facts {
		normFact := strings.ToLower(fact)
		matches := 0
		for _, w := range words {
			if len(w) > 2 && strings.Contains(normFact, w) {
				matches++
			}
		}
		if float64(matches)/float64(len(words)) > 0.7 {
			return true
		}
	}
	return false
}

// PendingReviewCount returns the number of review items with status "pending".
func PendingReviewCount(state *LoopState) int {
	n := 0
	for _, item := range state.ReviewItems {
		if item.Status == ReviewPending {
			n++
		}
	}
	return n
}

// MergeReviewItems appends new review items, deduplicating by normalized text
// against existing items. Returns only the newly added items.
func MergeReviewItems(state *LoopState, items []ReviewItem) []ReviewItem {
	existing := make([]string, 0, len(state.ReviewItems))
	for _, item := range state.ReviewItems {
		existing = append(existing, normalizeText(item.Text))
	}
	var added []ReviewItem
	for _, item := range items {
		norm := normalizeText(item.Text)
		if isSimilarToAny(norm, existing) {
			continue
		}
		state.ReviewItems = append(state.ReviewItems, item)
		existing = append(existing, norm)
		added = append(added, item)
	}
	return added
}
