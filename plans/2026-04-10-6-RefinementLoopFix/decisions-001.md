## Decision: Use Score=0 as soft-failure default instead of 0.5
ID: DEC-001

- Date: 2026-04-10
- Status: Accepted

- Context:
  When `doEvaluateQuestions` fails to parse LLM output, it assigned Score=0.5 to all questions. `FilterAndRankQuestions` keeps questions with `Score >= 0.6 || Score <= 0`. Score 0.5 fails both conditions, causing all questions to be removed from the pool.

- Options Considered:
  - Option A: Change default score to 0 (means "unscored")
  - Option B: Lower filter threshold from 0.6 to 0.4
  - Option C: Change default score to 0.65 (above threshold)

- Decision:
  Option A — Score=0 as "unscored" sentinel

- Rationale:
  Score=0 already has correct handling everywhere: `FilterAndRankQuestions` keeps `Score <= 0`, and `SelectHighestImpactQuestion` uses fallback scoring (`Impact + (1 - sectionConf)`) for `Score <= 0`. No other code needs to change.

- Consequences:
  - Positive:
    - Unscored questions survive filtering and get fallback ranking
    - Zero additional changes needed in other functions
  - Negative:
    - None identified

- Impact:
  - Affects: steps.go (doEvaluateQuestions)
  - Related Plans: 2026-04-10-6-RefinementLoopFix

---

## Decision: PruneResolved should only remove answered questions
ID: DEC-002

- Date: 2026-04-10
- Status: Accepted

- Context:
  `PruneResolved` was removing unanswered questions when their section's confidence exceeded the threshold. With small models (Ollama), section scoring can inflate after minimal content, causing all unanswered questions for those sections to be pruned.

- Options Considered:
  - Option A: Only prune answered questions
  - Option B: Only prune unanswered when ALL sections exceed threshold

- Decision:
  Option A — only prune answered questions

- Rationale:
  The evaluate+filter+rank step is the right place to remove low-value unanswered questions. Pruning should only clean up questions that have been answered.

- Consequences:
  - Positive:
    - Question pool won't be unexpectedly drained by inflated section scores
  - Negative:
    - More questions may remain in pool (minor, bounded by MaxIterations)

- Impact:
  - Affects: state.go (PruneResolved)
  - Related Plans: 2026-04-10-6-RefinementLoopFix

---

## Decision: Regenerate questions on empty pool instead of breaking
ID: DEC-003

- Date: 2026-04-10
- Status: Accepted

- Context:
  When `SelectHighestImpactQuestion` returned nil (empty pool), the loop did `break` with no recovery. This meant any bug that drained the question pool would immediately exit the loop regardless of confidence level.

- Options Considered:
  - Option A: Call `doGenerateQuestions` before breaking
  - Option B: Call `doFindGaps` before breaking

- Decision:
  Option A — regenerate via `doGenerateQuestions`, which uses `MergeQuestions` internally to avoid duplicates

- Rationale:
  `doGenerateQuestions` is designed to produce questions for sections with lowest confidence and deduplicates against existing questions and known facts. If it produces nothing, the break is genuine.

- Consequences:
  - Positive:
    - Loop recovers from empty pool when confidence isn't met
  - Negative:
    - One extra LLM call on the recovery path (acceptable, rare after other fixes)

- Impact:
  - Affects: controller.go (RunLoop, two locations)
  - Related Plans: 2026-04-10-6-RefinementLoopFix

---

## Decision: Strip code fences in parseQuestionScores using bracket-depth matching
ID: DEC-004

- Date: 2026-04-10
- Status: Accepted

- Context:
  Small models (llama3.2:1b) wrap JSON in code fences and append extra JSON blocks. The parser used `strings.LastIndex("]")` which picked up the last `]` in garbage, creating an invalid range.

- Options Considered:
  - Option A: Strip code fences + bracket-depth matching for first complete array
  - Option B: Regex to extract first code block content

- Decision:
  Option A — strip fences then find first complete `[...]` by counting bracket depth

- Rationale:
  Bracket-depth matching is more robust than regex for nested JSON and handles cases with or without code fences.

- Consequences:
  - Positive:
    - Valid scores extracted even when LLM appends extra content
  - Negative:
    - None identified

- Impact:
  - Affects: steps.go (parseQuestionScores, new helpers stripCodeFences + extractFirstJSONArray)
  - Related Plans: 2026-04-10-6-RefinementLoopFix
