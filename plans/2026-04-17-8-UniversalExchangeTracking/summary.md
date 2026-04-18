# Universal AI Exchange Tracking + Branch Consolidation

## What was done

### 1. Branch Consolidation
Merged `feat/rag-integration` into `feat/opt-flow`, resolving 3 merge conflicts (chat.go, App.tsx, useChat.ts) and fixing 10+ test files with missing `regPath`/`logBase` constructor arguments.

### 2. Features Recovered from feat/rag-integration
- **Visual Workflow Graph**: Interactive React Flow pipeline visualization (`WorkflowGraph.tsx`, `WorkflowNodeTypes.tsx`, `WorkflowPage.tsx`) with dagre layout, decision nodes, loopback edges, and current-step highlighting
- **Editable Prompts**: Monaco-based `WorkflowPromptEditor.tsx` with per-project and per-connection prompt overrides
- **Prompt Externalization**: `PromptStore`/`PromptChain` system in `promptfiles/` package with template rendering (Go `text/template`)
- **PromptSnapshot Model**: Full LLM round-trip capture (system prompt, history, RAG context, user message, raw response)
- **StepTrace**: Structured JSON trace for refinement loop exchanges
- **RAG Integration**: Context injection, source display, health monitoring
- **Refinement Loop**: Guided vision path with Q&A scoring and synthesis

### 3. Universal Exchange Tracking Added
Extended PromptSnapshot/runTrace logging to ALL LLM operations:

| Operation | File | Method |
|-----------|------|--------|
| Vision Chat | chat.go | PromptSnapshot ✅ (existed) |
| Vision Turn | vision_turn.go | logVisionStep ✅ (existed) |
| UX/Arch/Build Chat | chat.go | PromptSnapshot ✅ (existed) |
| **Summary/Complete** | pipeline.go | **PromptSnapshot (NEW)** |
| **Refine** | refine.go | **PromptSnapshot (NEW)** |
| **Skill Analysis** | skills.go | **PromptSnapshot + runLog (NEW)** |
| **Instruct Planning** | instruct.go | **PromptSnapshot + runLog (NEW)** |
| **Bead Parse** | bead.go | **runTrace step (NEW)** |
| **Bead Execute** | bead.go | **runTrace step (NEW)** |
| **Bead Review** | bead.go | **runTrace step (NEW)** |
| **Mock Specs Synthesis** | mock.go | **runTrace step (NEW)** |
| **Arch/Build Synthesis** | synthesis.go | **runTrace step (NEW, via param)** |

### 4. Infrastructure
- `writePromptSnapshot()` helper in `runlog_helpers.go` for consistent snapshot writes
- `SkillHandler` and `InstructHandler` now receive `logBase` for run logging
- `synthesizeSpecs()` and `applySynthesis()` accept optional `*runTrace` parameter

## Build Status
- Backend: ✅ compiles clean
- Frontend: ✅ TypeScript type-checks clean
- Tests: pre-existing failures only (Windows file mode tests, connection error recovery tests)
