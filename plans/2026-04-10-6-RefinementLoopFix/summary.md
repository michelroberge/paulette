# Summary: Refinement Loop Fix

## Problem
Two issues in guided vision refinement:
1. Q&A loop exited immediately without asking any questions (run `575fe871`: evaluate_questions -> synthesize, skipping all Q&A)
2. Loop asked only 1 question per iteration instead of all generated questions (run `459684de`: 5 questions generated, only Q1 asked, then garbage follow-ups from critique/gaps displaced remaining original questions)

## Root Causes
1. **Score mismatch**: Soft-failure default Score=0.5 fell below FilterAndRankQuestions threshold of 0.6, removing ALL questions
1b. **Parser failure**: `parseQuestionScores` used `LastIndex("]")` which matched garbage appended by small models
2. **Aggressive pruning**: `PruneResolved` dropped unanswered questions when section confidence exceeded threshold
3. **Hard break**: `SelectHighestImpactQuestion` returning nil caused immediate loop exit with no recovery
4. **No safeguard**: Post-prune empty pool was never refilled
5. **Single-question-per-iteration design**: Only 1 question asked per iteration; critique/gap-finding then injected low-quality follow-ups that displaced remaining original questions

## Changes

### Backend
- `steps.go`: Changed soft-failure default score from 0.5 to 0 (unscored sentinel). Rewrote `parseQuestionScores` to strip code fences and use bracket-depth matching for first complete JSON array.
- `state.go`: `PruneResolved` now only removes answered questions, not unanswered ones in high-confidence sections.
- `controller.go`: Restructured SELECT+ANSWER into inner loop that asks ALL unanswered questions before processing. Added question regeneration when pool is empty but confidence unmet. Skips processing for `[dismissed]` answers.
- `controller_test.go`: Added 3 new tests covering the specific failures.

### Frontend
- `useRefinementLoop.ts`: Race-safe input hiding via `questionVersionRef` — nulls `currentQuestion` only when no new question arrived during POST. Added `dismiss` function that sends `[dismissed]` as answer.
- `RefinementPanel.tsx`: Shows question score/impact/source badges. Added "Dismiss" button next to "Answer" for skipping irrelevant questions. Active log step shows pulsing dot so user sees progress during long operations.
- `types/refinement.ts`: Added `score`, `relevance`, `novelty`, `source` fields to `RefinementQuestion`.

### Hallucination prevention (steps.go)
- `doNormalizeAnswer`: Added word-count expansion guard (max 3x). Small models hallucinate during normalization (e.g. "any casual gamer" → 6 invented demographics). Falls back to raw answer on expansion.
- `doMergeFact`: Replaced LLM call with deterministic bullet-point append. Eliminates hallucination snowball where merge generates creative writing ("scientists baffled by bizarre phenomenon"). Synthesize step produces the final narrative.
- `filterAndCapFacts`: Hard cap of 5 facts per answer, min 3 words per fact.
- Net effect: fewer LLM calls (77 vs 84 in tests), no hallucination in intermediate state.

## Test Results
All 21 backend tests pass. Frontend compiles clean. `TestRunLoopAutonomous` now asks all questions per round (3, then 2, then 2) instead of 1 at a time.
