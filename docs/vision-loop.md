# Vision Definition Pipeline

## Graph

1. User Idea (few sentences)
2. Summarize (2-sentence anchor)
3. Generate Initial Questions
4. **MAIN LOOP** (until convergence)

    ### Exploration Loop
    a. Evaluate questions (batch: relevance/novelty/impact → JSON scores)
    b. Rewrite vague high-impact questions (impact > 0.7, text < 40 chars)
    c. Filter (discard score < 0.6) + rank by composite score
    d. Deduplicate (exact text → normalized substring → word overlap with facts)
    e. Select best question

    ### Get Answer
    f. Ask user (interactive) or simulate (autonomous)
    g. Normalize answer (rewrite as explicit statements with summary + question context)

    ### Refinement Loop
    h. Extract facts (atomic units from normalized answer)
    i. Classify facts (route to section: problem/users/features/ux/metrics/constraints/scope)
    j. Merge facts into sections

    ### Coherence Check
    k. Check new facts against existing state for contradictions
       - If incoherent → add corrective questions, skip tension
       - If coherent → proceed

    ### Tension Check (gated on coherence)
    l. Challenge the idea: unrealistic assumptions, market risks, misaligned metrics
       - Up to 2 adversarial questions added to pool

    ### Validation Loop
    m. Score all sections (0–1 completeness)
    n. Critique (vague, missing, inconsistent, risky → corrective questions)
    o. Find gaps (new questions for lowest-confidence areas)
    p. Prune resolved questions

    ### Convergence Check
    q. Average confidence ≥ threshold AND no open questions → exit
    r. Max iterations reached → exit
    s. Otherwise → next iteration

5. **Final Synthesis** → vision.md

---

## Loop Types

### 1. Exploration Loop
Question quality control before asking:

```
generate_questions → evaluate_questions → filter + rank → select_best_question
```

Evaluation prompt returns JSON per question:
```json
{"relevance": 0.8, "novelty": 0.5, "impact": 0.9}
```

Composite score: `0.5 * impact + 0.3 * relevance + 0.2 * novelty`

Sub-steps:
- If impact high BUT specificity low → rewrite via LLM
- If score < 0.6 → discard
- Remove questions similar to already-asked or already-known facts

### 2. Refinement Loop
Normalize → Extract → Classify → Merge into structured state

### 3. Coherence Check
Verify newly added facts don't contradict existing sections.
Gates the tension check — no point challenging an internally contradictory idea.

### 4. Tension Check
Adversarial: challenge assumptions, not just clarify gaps.
Generates challenge questions (Source: "tension") that flow back through the normal pipeline.

### 5. Validation Loop
Critique → Score → Detect gaps → Feed back as questions

All five concerns are embedded in each iteration of the main loop.

---

## Control Signals

* **Confidence scores per section** → drives focus (exploration targets lowest-confidence areas)
* **Open questions list** → drives exploration
* **Coherence result** → gates tension check
* **Validation issues** → drives correction

## Final Output

```text
vision.md:

# Product Vision: [derived name]

## Problem Statement
## Target Users
## Core Features
## User Experience
## Success Metrics
## Constraints & Assumptions
## Out of Scope
```

---

## Mental Model

> **Input → Structured Memory → Iterative Refinement (5 concerns per iteration) → Converged State → Deterministic Synthesis**

Each iteration: Explore → Refine → Cohere → Challenge → Validate
