## Decision: Replace 8 LLM functions with deterministic algorithms
ID: DEC-005

- Date: 2026-04-10
- Status: Accepted

- Context:
  Small models (llama3.2:1b) hallucinate on most refinement LLM calls. The loop made ~50-60 LLM calls per iteration, most producing garbage that snowballed through the pipeline.

- Options Considered:
  - Option A: Replace all LLM calls with deterministic algorithms
  - Option B: Replace high-volume/low-quality calls, keep high-value ones as LLM

- Decision:
  Option B — replace 8 functions, keep 5 as LLM (summarize, generateQuestions, tensionCheck, simulateAnswer, synthesize)

- Rationale:
  The 5 kept functions genuinely require semantic reasoning (summarizing, question generation, adversarial thinking, autonomous answering, final synthesis). The 8 replaced functions were either: (a) producing garbage with small models, (b) had reliable deterministic alternatives, or (c) were redundant with other steps.

- Consequences:
  - Positive:
    - 78% fewer LLM calls (84 → 18)
    - Eliminates hallucination in intermediate state
    - Much faster iterations on Ollama
    - Deterministic behavior is predictable and debuggable
  - Negative:
    - Keyword classifier may misclassify nuanced facts (acceptable — wrong section is low-stakes)
    - Heuristic scoring is less nuanced than LLM scoring (acceptable — convergence still works)
    - Lost some novel critique/gap insights (mitigated by keeping tensionCheck as LLM)

- Impact:
  - Affects: steps.go (8 functions), controller.go (removed rewrite block)
  - Related Plans: 2026-04-10-6-RefinementLoopFix, 2026-04-10-7-DeterministicRefinement
