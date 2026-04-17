# Fix: Guided Vision Q&A Loop Exits Immediately — Zero Questions Asked

## Context

The guided refinement loop for vision definition exits without asking any questions, jumping straight from evaluate_questions to synthesize. The user tests with Ollama (llama3.2:1b), which amplifies the issue.

**Confirmed by run trace** `575fe871`: summarize (OK) -> generate_questions (5 good questions) -> evaluate_questions (LLM returned valid scores wrapped in code fences + extra JSON garbage -> parse failed -> all Score=0.5) -> FilterAndRankQuestions removed all 5 (0.5 < 0.6 threshold) -> SelectHighestImpactQuestion nil -> break -> synthesize. No Q&A at all.

## Root Causes

### Bug 1 (PRIMARY) — Soft-failure score (0.5) falls below filter threshold (0.6)
- steps.go:203,210,221: When `doEvaluateQuestions` fails to parse LLM output, all questions get `Score = 0.5`
- state.go:247: `FilterAndRankQuestions` keeps `Score >= 0.6 || Score <= 0` — Score 0.5 fails both conditions
- Result: ALL questions removed from the pool

### Bug 1b — `parseQuestionScores` fails on code-fenced responses
- steps.go:425-444: Uses `strings.Index("[")` / `strings.LastIndex("]")` to find JSON array
- LLM wraps scores in code fences and appends extra JSON — the first `[` and last `]` span multiple arrays + fence markers = invalid JSON
- The scores in the first code block were actually valid but never extracted

### Bug 2 — `PruneResolved` drops unanswered questions
- state.go:293-296: Removes unanswered questions when their section's confidence >= threshold
- With small models, `doScoreAllSections` can inflate section scores after minimal content
- Result: unanswered questions pruned, pool drained

### Bug 3 — `break` on nil question with no recovery
- controller.go:180-181: When `SelectHighestImpactQuestion` returns nil, the loop does `break` — no attempt to regenerate questions

### Bug 4 — No post-prune question regeneration
- controller.go:331: After `PruneResolved` empties the pool, nothing refills it before the next iteration checks for questions

## Fixes

### Fix 1a: Change soft-failure default score from 0.5 to 0 (`steps.go`)
Lines 203, 210, 221 — change `questions[i].Score = 0.5` to `questions[i].Score = 0`

### Fix 1b: Make `parseQuestionScores` strip code fences (`steps.go`)
Lines 425-444 — strip code fence lines, use bracket-depth matching for first complete array

### Fix 2: `PruneResolved` only drops answered questions (`state.go`)
Lines 288-300 — remove the section-confidence check for unanswered questions

### Fix 3: Replace `break` with question regeneration (`controller.go`)
Lines 179-181 — when question is nil, try `doGenerateQuestions` before giving up

### Fix 4: Post-prune regeneration safeguard (`controller.go`)
After line 331 — regenerate if pruning emptied the pool before convergence

### New tests (`controller_test.go`)
1. `TestFilterAndRankKeepsUnscoredQuestions`
2. `TestPruneResolvedKeepsUnanswered`

## Execution Order

| # | Fix | File | Lines |
|---|-----|------|-------|
| 1a | Default score 0.5 -> 0 | steps.go | 203, 210, 221 |
| 1b | Strip code fences in parser | steps.go | 425-444 |
| 2 | Prune only answered | state.go | 288-300 |
| 3 | Break -> regenerate | controller.go | 179-181 |
| 4 | Post-prune safeguard | controller.go | after 331 |
| 5 | New tests | controller_test.go | append |
