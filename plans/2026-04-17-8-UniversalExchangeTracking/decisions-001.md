## Decision: Merge feat/rag-integration into feat/opt-flow
ID: DEC-001

- Date: 2026-04-17
- Status: Accepted

- Context:
  Two feature branches diverged from develop with complementary features:
  - feat/opt-flow: vision turn runner with exchange tracing
  - feat/rag-integration: visual workflow graph, editable prompts, RAG, prompt externalization

- Options Considered:
  - Option A: Cherry-pick specific commits from rag-integration
  - Option B: Full merge of rag-integration into opt-flow

- Decision:
  Full merge (Option B)

- Rationale:
  The branches share the same base commit. A full merge brings all features cleanly with only 3 manageable conflicts. Cherry-picking would risk inconsistent state and missing interdependencies.

- Consequences:
  - Positive:
    - All features from both branches available on one branch
    - Visual workflow, editable prompts, RAG integration all recovered
  - Negative:
    - 153 files changed — larger diff to review

- Impact:
  - Affects: all handler files, frontend pipeline components, prompt system
  - Related Plans: PromptExternalization, WorkflowGraphRevamp

---

## Decision: Use PromptSnapshot for simple chat-like ops, runTrace for multi-step ops
ID: DEC-002

- Date: 2026-04-17
- Status: Accepted

- Context:
  Need to track ALL LLM exchanges. Some operations are single-call (summary, refine), others are multi-step (bead execution with review loop, mock orchestration).

- Options Considered:
  - Option A: Use PromptSnapshot everywhere
  - Option B: Use PromptSnapshot for single-call ops, runTrace steps for multi-step ops

- Decision:
  Option B — hybrid approach matching existing patterns

- Rationale:
  PromptSnapshot captures one full round-trip with all context. runTrace captures ordered steps with individual timing. Multi-step operations (bead execute → review → correction loop) need per-step visibility that PromptSnapshot can't provide.

- Consequences:
  - Positive:
    - Consistent with existing vision_turn.go and refinement/trace.go patterns
    - Each bead execution and review iteration gets its own step log
  - Negative:
    - Two logging patterns to maintain

- Impact:
  - Affects: runlog_helpers.go, bead.go, mock.go, synthesis.go
  - Related Plans: UniversalExchangeTracking
