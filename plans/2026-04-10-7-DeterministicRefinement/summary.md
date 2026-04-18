# Summary: Deterministic Refinement Loop

## Problem
With small models (llama3.2:1b), most LLM calls in the refinement loop produced garbage: single-word facts, hallucinated content, creative writing. Each iteration made ~50-60 LLM calls, many of which were both slow and harmful.

## Changes (steps.go, controller.go)

8 of 15 LLM functions replaced with deterministic algorithms:

| Function | Was | Now |
|----------|-----|-----|
| doClassifyFact | LLM per fact (5-25/iter) | Keyword-to-section mapping |
| doExtractFacts | LLM + fallback | `deterministicExtractFacts` directly |
| doNormalizeAnswer | LLM + expansion guard | Pass-through (skip entirely) |
| doScoreSection | LLM per section (7/iter) | Bullet count + word count heuristic |
| doEvaluateQuestions | LLM batch | Impact/relevance/novelty heuristics |
| doRewriteQuestion | LLM conditional | Removed from loop |
| doCoherenceCheck | LLM | Structural negation + cross-section check |
| doCritique | LLM | Structural checks (metrics without numbers, features without constraints) |
| doFindGaps | LLM | Template questions for empty/thin sections |
| doMergeFact | (already deterministic) | Bullet-point append |

5 functions kept as LLM: doSummarize, doGenerateQuestions, doTensionCheck, doSimulateAnswer, doSynthesize.

## Results
- LLM calls per run: **84 → 18** (78% reduction)
- Hallucination surface: reduced from every call to only 5 functions
- All 21 tests pass
