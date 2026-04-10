package refinement

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/michelroberge/paulette/backend/internal/model"
	"github.com/michelroberge/paulette/backend/internal/provider"
)

// LoopEvent is the progress event emitted to the frontend during refinement.
type LoopEvent struct {
	Phase      LoopPhase              `json:"phase"`
	Iteration  int                    `json:"iteration"`
	MaxIter    int                    `json:"maxIter"`
	Confidence map[SectionName]float64 `json:"confidence,omitempty"`
	Question   *Question              `json:"question,omitempty"`
	Message    string                 `json:"message,omitempty"`
	ReviewItem *ReviewItem            `json:"reviewItem,omitempty"`
}

// Controller drives the closed-loop refinement process.
type Controller struct {
	prov        provider.Provider
	modelID     string
	temperature *float64
	numCtx      *int
	projectDir  string
	stage       string
	prompts     PromptSet
	trace       *Trace

	minConfidence    float64
	currentPhase     LoopPhase // set before each llmCall for trace context
	currentIteration int       // set before each llmCall for trace context
}

// ControllerConfig holds the configuration for constructing a Controller.
type ControllerConfig struct {
	Provider      provider.Provider
	ModelID       string
	Temperature   *float64
	NumCtx        *int
	ProjectDir    string
	Stage         string
	Prompts       PromptSet
	Trace         *Trace
	MaxIter       int
	MinConfidence float64
}

// NewController creates a refinement controller.
func NewController(cfg ControllerConfig) *Controller {
	minConf := cfg.MinConfidence
	if minConf <= 0 {
		minConf = 0.85
	}
	return &Controller{
		prov:          cfg.Provider,
		modelID:       cfg.ModelID,
		temperature:   cfg.Temperature,
		numCtx:        cfg.NumCtx,
		projectDir:    cfg.ProjectDir,
		stage:         cfg.Stage,
		prompts:       cfg.Prompts,
		trace:         cfg.Trace,
		minConfidence: minConf,
	}
}

// RunLoop executes the iterative refinement loop.
//
//   - answerCh: nil for autonomous mode; delivers user answers for interactive mode
//   - emitFn: sends progress events to the frontend (via SSE)
//   - saveFn: persists state after each step (to survive restarts)
//
// Returns the final artifact text and total token count.
func (c *Controller) RunLoop(
	ctx context.Context,
	state *LoopState,
	answerCh <-chan string,
	emitFn func(LoopEvent),
	saveFn func(*LoopState),
) (string, int, error) {
	var totalTokens int

	emit := func(phase LoopPhase, msg string, q *Question) {
		if emitFn != nil {
			emitFn(emitEvent(phase, state, msg, q))
		}
	}
	emitReview := func(phase LoopPhase, item *ReviewItem) {
		if emitFn != nil {
			ev := emitEvent(phase, state, "", nil)
			ev.ReviewItem = item
			emitFn(ev)
		}
	}
	save := func() {
		if saveFn != nil {
			saveFn(state)
		}
	}
	setPhase := func(phase LoopPhase) {
		state.Phase = phase
		c.currentPhase = phase
		c.currentIteration = state.Iteration
	}

	// Step 1: Summarize (if not already done)
	if state.Phase == PhaseInit || state.IdeaSummary == "" {
		setPhase(PhaseSummarize)
		emit(PhaseSummarize, "Summarizing your idea...", nil)
		save()

		tokens, err := c.doSummarize(ctx, state)
		totalTokens += tokens
		if err != nil {
			return "", totalTokens, fmt.Errorf("summarize: %w", err)
		}
		log.Printf("refinement: summarized idea → %q", state.IdeaSummary)
		emit(PhaseSummarize, state.IdeaSummary, nil)
		save()
	}

	// Step 2: Generate initial questions
	if state.Phase == PhaseInit || state.Phase == PhaseSummarize || len(state.OpenQuestions) == 0 {
		setPhase(PhaseGenerateQs)
		emit(PhaseGenerateQs, "Identifying gaps...", nil)
		save()

		tokens, err := c.doGenerateQuestions(ctx, state)
		totalTokens += tokens
		if err != nil {
			return "", totalTokens, fmt.Errorf("generate questions: %w", err)
		}
		log.Printf("refinement: generated %d questions", len(state.OpenQuestions))
		emit(PhaseGenerateQs, fmt.Sprintf("Found %d questions to explore", len(state.OpenQuestions)), nil)
		save()
	}

	// Step 3: Main refinement loop
	// Each iteration runs 5 sub-concerns:
	//   Exploration → Refinement → Coherence → Tension → Validation
	for !IsConverged(state, c.minConfidence) {
		if err := ctx.Err(); err != nil {
			return "", totalTokens, err
		}
		c.currentIteration = state.Iteration

		// ─── EXPLORATION LOOP: evaluate and rank questions ───
		unanswered := collectUnanswered(state)
		if len(unanswered) > 0 {
			setPhase(PhaseEvaluateQs)
			emit(PhaseEvaluateQs, fmt.Sprintf("Evaluating %d questions...", len(unanswered)), nil)

			evaluated, _, _ := c.doEvaluateQuestions(ctx, state, unanswered)

			// Apply scores back and filter
			applyEvaluatedScores(state, evaluated)
			kept := FilterAndRankQuestions(state.OpenQuestions, 0.6)
			state.OpenQuestions = kept
			save()
		}

		// ─── ASK ALL UNANSWERED QUESTIONS ───
		// Inner loop: ask every unanswered question before processing.
		// This ensures all generated questions are presented to the user
		// before critique/gap-finding can inject lower-quality follow-ups.
		var answeredThisRound []*Question
		for {
			if err := ctx.Err(); err != nil {
				return "", totalTokens, err
			}

			question := SelectHighestImpactQuestion(state)
			if question == nil {
				break
			}

			var answer string
			if answerCh != nil {
				setPhase(PhaseAwaitAnswer)
				emit(PhaseAwaitAnswer, "", question)
				save()

				select {
				case ans, ok := <-answerCh:
					if !ok {
						return "", totalTokens, fmt.Errorf("answer channel closed")
					}
					answer = ans
				case <-ctx.Done():
					return "", totalTokens, ctx.Err()
				}
			} else {
				setPhase(PhaseAwaitAnswer)
				emit(PhaseAwaitAnswer, "Reasoning about: "+question.Text, question)

				ans, tokens, err := c.doSimulateAnswer(ctx, state, question.Text)
				totalTokens += tokens
				if err != nil {
					return "", totalTokens, fmt.Errorf("simulate answer: %w", err)
				}
				answer = ans
			}

			// User requested early finish — skip to synthesize
			if answer == "[finish]" {
				log.Printf("refinement: user requested finish — skipping to synthesize")
				question.Answered = true
				question.Answer = ""
				goto synthesize
			}

			question.Answered = true
			question.Answer = answer
			answeredThisRound = append(answeredThisRound, question)
		}

		if len(answeredThisRound) == 0 {
			// No questions available — try regenerating before giving up
			log.Printf("refinement: no open questions but not converged (avg=%.2f, target=%.2f) — regenerating",
				AverageConfidence(state), c.minConfidence)
			rTokens, rErr := c.doGenerateQuestions(ctx, state)
			totalTokens += rTokens
			if rErr != nil || OpenQuestionCount(state) == 0 {
				log.Printf("refinement: regeneration produced no new questions — exiting loop")
				break
			}
			continue // re-enter the loop to ask the regenerated questions
		}

		log.Printf("refinement: asked %d questions this round", len(answeredThisRound))

		// ─── REFINEMENT: process all answers ───
		var allFacts []string
		for _, question := range answeredThisRound {
			// Skip dismissed questions — user found them irrelevant
			if question.Answer == "[dismissed]" {
				log.Printf("refinement: question dismissed: %s", question.Text)
				continue
			}

			normalizedAnswer, tokens, normErr := c.doNormalizeAnswer(ctx, state.IdeaSummary, question.Text, question.Answer)
			totalTokens += tokens
			if normErr != nil {
				log.Printf("refinement: normalize answer error (using raw): %v", normErr)
				normalizedAnswer = question.Answer
			}

			setPhase(PhaseExtractFacts)
			emit(PhaseExtractFacts, fmt.Sprintf("Extracting facts from answer to: %s", question.Text), nil)

			facts, tokens, err := c.doExtractFacts(ctx, state.IdeaSummary, normalizedAnswer)
			totalTokens += tokens
			if err != nil {
				log.Printf("refinement: extract facts error (skipping): %v", err)
				continue
			}
			state.KnownFacts = append(state.KnownFacts, facts...)
			allFacts = append(allFacts, facts...)
		}
		save()

		setPhase(PhaseUpdateSections)
		emit(PhaseUpdateSections, fmt.Sprintf("Integrating %d facts...", len(allFacts)), nil)

		for i, fact := range allFacts {
			if err := ctx.Err(); err != nil {
				return "", totalTokens, err
			}

			section, tokens, err := c.doClassifyFact(ctx, fact, i)
			totalTokens += tokens
			if err != nil {
				log.Printf("refinement: classify fact error (skipping): %v", err)
				continue
			}

			merged, tokens, err := c.doMergeFact(ctx, state.IdeaSummary, state.Sections[section], fact, i)
			totalTokens += tokens
			if err != nil {
				log.Printf("refinement: merge fact error (skipping): %v", err)
				continue
			}
			state.Sections[section] = merged
		}
		save()

		// ─── COHERENCE CHECK: verify new facts don't contradict state ───
		isCoherent := true
		if len(allFacts) > 0 {
			setPhase(PhaseCoherence)
			emit(PhaseCoherence, "Checking coherence...", nil)

			coherent, cohItems, cohTokens, _ := c.doCoherenceCheck(ctx, state, allFacts)
			totalTokens += cohTokens
			isCoherent = coherent
			if !coherent {
				emit(PhaseCoherence, "Inconsistencies detected — adding corrective questions", nil)
			} else {
				emit(PhaseCoherence, "Coherent", nil)
			}
			for i := range MergeReviewItems(state, cohItems) {
				emitReview(PhaseCoherence, &cohItems[i])
			}
			save()
		}

		// ─── TENSION CHECK: challenge the idea (only if coherent) ───
		if isCoherent {
			setPhase(PhaseTension)
			emit(PhaseTension, "Challenging assumptions...", nil)

			tenItems, tenTokens, _ := c.doTensionCheck(ctx, state)
			totalTokens += tenTokens
			for i := range MergeReviewItems(state, tenItems) {
				emitReview(PhaseTension, &tenItems[i])
			}
			save()
		}

		// ─── SCORING + VALIDATION ───
		setPhase(PhaseScoreConfidence)
		emit(PhaseScoreConfidence, "Evaluating completeness...", nil)

		scoreTokens, scoreErr := c.doScoreAllSections(ctx, state)
		totalTokens += scoreTokens
		if scoreErr != nil {
			return "", totalTokens, fmt.Errorf("score sections: %w", scoreErr)
		}
		emit(PhaseScoreConfidence, fmt.Sprintf("Average confidence: %.0f%%", AverageConfidence(state)*100), nil)
		save()

		// Critique
		setPhase(PhaseCritique)
		emit(PhaseCritique, "Running critique...", nil)

		criItems, criTokens, criErr := c.doCritique(ctx, state)
		totalTokens += criTokens
		if criErr != nil {
			log.Printf("refinement: critique error (continuing): %v", criErr)
		}
		for i := range MergeReviewItems(state, criItems) {
			emitReview(PhaseCritique, &criItems[i])
		}
		save()

		// Find new gaps
		setPhase(PhaseFindGaps)
		emit(PhaseFindGaps, "Looking for remaining gaps...", nil)

		gapItems, gapTokens, gapErr := c.doFindGaps(ctx, state)
		totalTokens += gapTokens
		if gapErr != nil {
			return "", totalTokens, fmt.Errorf("find gaps: %w", gapErr)
		}
		for i := range MergeReviewItems(state, gapItems) {
			emitReview(PhaseFindGaps, &gapItems[i])
		}

		// Prune resolved questions
		PruneResolved(state, c.minConfidence)

		// Safeguard: if pruning emptied the pool but we haven't converged, refill.
		if OpenQuestionCount(state) == 0 && !IsConverged(state, c.minConfidence) {
			log.Printf("refinement: post-prune question pool empty — regenerating")
			rTokens, _ := c.doGenerateQuestions(ctx, state)
			totalTokens += rTokens
		}

		state.Iteration++
		save()

		log.Printf("refinement: iteration %d complete — avg confidence %.2f, %d open questions",
			state.Iteration, AverageConfidence(state), OpenQuestionCount(state))
		emit(PhaseFindGaps, fmt.Sprintf("Iteration %d/%d complete", state.Iteration, state.MaxIterations), nil)
	}

	// Step 5: Synthesize final artifact
synthesize:
	setPhase(PhaseSynthesize)
	emit(PhaseSynthesize, "Generating vision document...", nil)

	artifact, tokens, err := c.doSynthesize(ctx, state)
	totalTokens += tokens
	if err != nil {
		return "", totalTokens, fmt.Errorf("synthesize: %w", err)
	}

	setPhase(PhaseComplete)
	emit(PhaseComplete, "Vision document complete", nil)
	save()

	// Flush trace to run log directory.
	if c.trace != nil {
		c.trace.Flush()
		log.Printf("refinement: trace — %s", c.trace.Summary())
	}

	return artifact, totalTokens, nil
}

// LoopEventToStreamEvent converts a LoopEvent to a model.StreamEvent
// suitable for emission through the SSE infrastructure.
func LoopEventToStreamEvent(event LoopEvent) model.StreamEvent {
	data, _ := json.Marshal(event)
	return model.StreamEvent{Type: "refinement", Content: string(data)}
}
