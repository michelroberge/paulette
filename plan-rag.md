# Plan: Paulette x R3 RAG Pipeline Integration

## Context

Paulette is an AI product development pipeline (Vision -> UX -> Architecture -> Build -> Complete) with a Go backend and React frontend. The R3 RAG Pipeline is a separate Python/FastAPI service that handles knowledge ingestion, embedding, semantic retrieval, and context construction using Qdrant + Ollama.

**Problem:** Paulette's stages currently operate in isolation — each project starts from zero knowledge. There's no mechanism to learn from past projects, reuse proven architectural patterns, or feed successful outputs back into a shared knowledge base.

**Goal:** Two-way integration:
1. **Paulette consumes RAG** — Stage conversations and bead execution are augmented with retrieved knowledge (past patterns, similar architectures, code examples)
2. **Paulette feeds RAG** — Approved artifacts and reviewed code flow back into the RAG store, building an ever-improving knowledge base

**Intended outcome:** Each new Paulette project benefits from the cumulative knowledge of all previous ones. High-quality outputs get promoted; poor patterns get demoted.

---

## Phase 1: Foundation (RAG Client, Config, Health)

### 1.1 Configuration

**Modify:** `backend/internal/config/config.go`
- Add `RAGConfig` struct with `Enabled bool` and `BaseURL string`
- Add `RAG RAGConfig` field to `Config` struct
- Load from env vars `RAG_ENABLED` and `RAG_BASE_URL` in `Load()` (following the existing env > settings > default pattern at lines 84-100)
- Default: `Enabled: false`, `BaseURL: "http://localhost:8000/api/v1"`

### 1.2 RAG HTTP Client

**Create:** `backend/internal/rag/client.go`
- `Client` struct wrapping `*http.Client` + `baseURL` + `enabled` flag
- `NewClient(cfg config.RAGConfig) *Client`
- `Healthy(ctx) bool` — calls `GET /health` with 3s timeout
- Methods for each RAG endpoint Paulette will use:
  - `Search`, `SearchCode`, `SearchDocs`
  - `BuildContext`, `CompressContext`
  - `Ingest`, `IngestStatus`
  - `Feedback`, `Promote`, `Enrich`
  - `Intent`
- Design: nil-safe — if `!enabled`, all methods return `(nil, nil)` so callers can treat "no RAG" as "no context" without error branching

**Create:** `backend/internal/rag/models.go`
- Go structs mirroring the RAG Pipeline's request/response schemas from `openapi.yaml`
- Key types: `SearchRequest`, `SearchResponse`, `SearchResult`, `ContextBuildRequest`, `ContextStrategy`, `ContextBuildResponse`, `IngestRequest`, `IngestOptions`, `FeedbackRequest`, `PromoteRequest`, `EnrichRequest`

### 1.3 Health Monitor

**Create:** `backend/internal/rag/health.go`
- `HealthMonitor` with `atomic.Bool` for lock-free status checks
- Polls `GET /health` every 30 seconds in a background goroutine
- `IsHealthy() bool` — instant check, no network call

### 1.4 Wire into Server

**Modify:** `backend/internal/server/server.go`
- Add `ragClient *rag.Client` field to `Server` struct (line 26)
- Accept `ragClient *rag.Client` in `New()` constructor (line 43)
- Start health monitor alongside orchestrator
- Pass `ragClient` to handlers: `ChatHandler`, `BeadHandler`, `PipelineHandler`

**Modify:** `backend/main.go`
- Construct `rag.NewClient(cfg.RAG)` and pass to `server.New()`

### 1.5 RAG Status Endpoint

**Create:** `backend/internal/handler/rag.go`
- `GET /api/rag/status` -> `{ enabled, healthy, baseUrl }`

**Modify:** `backend/internal/server/server.go`
- Mount `r.Get("/api/rag/status", ragH.Status)`

### 1.6 Frontend Status

**Modify:** `frontend/src/api/config.ts` — add `ragEnabled`, `ragHealthy` to app config
**Create:** `frontend/src/api/rag.ts` — `getRagStatus()` function

---

## Phase 2: Knowledge Consumption (RAG Augments Conversations)

### 2.1 Stage Context Builder

**Create:** `backend/internal/rag/context.go`
- `StageContextConfig` — per-stage defaults for collections, groups, max chunks, chunk types, boost
- Default mappings:
  - **Vision:** `docs+specs` collections, `["patterns","constraints"]` groups, 6 chunks
  - **UX:** `docs+specs`, `["examples","patterns"]`, 8 chunks
  - **Architecture:** `code+docs+specs`, `["examples","patterns","constraints"]`, 12 chunks, boost promoted
  - **Build:** `code+docs`, `["examples","patterns"]`, 8 chunks
- `BuildStageContext(ctx, client, stage, query, projectName) (string, []SourceRef, error)`
  1. Look up stage defaults
  2. Call `client.BuildContext()` with stage-appropriate strategy
  3. If result > ~2000 tokens, call `client.CompressContext()`
  4. Return formatted context block with source refs

Output format:
```
--- RAG KNOWLEDGE CONTEXT ---
[Examples]
... chunk content ...
[Patterns]
... chunk content ...
Sources: chunk-id (file.go), chunk-id (design.md)
--- END RAG KNOWLEDGE CONTEXT ---
```

### 2.2 System Prompt Augmentation

**Modify:** `backend/internal/agent/prompts.go`
- Add `GetSystemPromptWithRAG(stage, version, previousArtifacts, frameworkCfg, ragContext, enhancement)` variant
- Inserts RAG context block between the base prompt and enhancement context (before `applyEnhancementContext` at line 357)
- Existing `GetSystemPrompt()` remains unchanged for backward compatibility

### 2.3 Chat Handler Integration

**Modify:** `backend/internal/handler/chat.go`
- Add `ragClient *rag.Client` field to `ChatHandler` (line 35)
- Accept in `NewChatHandler()` constructor (line 50)
- In the chat flow (where system prompt is built before the LLM call):
  1. Check `h.ragClient != nil && h.ragClient.Healthy()`
  2. Call `rag.BuildStageContext()` with user's message as query
  3. Use `GetSystemPromptWithRAG()` instead of `GetSystemPrompt()`
  4. Emit `rag_sources` SSE event before first `chunk` event
  5. Cap RAG retrieval at 3s timeout — proceed without if slow

### 2.4 Bead Execution Augmentation

**Modify:** `backend/internal/handler/bead.go`
- Before building the code-writer system prompt in bead execution:
  1. Build query from `bead.Title + bead.Description + bead.TargetFiles`
  2. Call `client.SearchCode()` for code patterns
  3. Call `client.SearchDocs()` for architectural guidance
  4. Inject as `--- RELEVANT CODE PATTERNS ---` section in the code-writer prompt

**Modify:** `backend/internal/agent/beads.go`
- Add optional `ragContext string` parameter to code-writer prompt construction
- Add anti-pattern retrieval for Devil's Advocate reviewer

---

## Phase 3: Knowledge Production (Feed Artifacts & Code to RAG)

### 3.1 Ingestion Service

**Create:** `backend/internal/rag/ingest.go`
- `IngestArtifact(ctx, client, project, stage, content) error`
  - Maps stage to collection: vision/ux -> `specs`, architecture -> `specs`, build -> `docs`
  - Writes artifact to temp file, calls `PUT /ingest`
  - Metadata: `project`, `stage`, `version`, `type:artifact`, `paulette_project_id`
- `IngestProjectCode(ctx, client, project) error`
  - Calls `PUT /ingest` with `path: project.HostDir`, collection `code`
  - Excludes `.paulette/`, `.git/`, `node_modules/`
- `IngestBeadCode(ctx, client, project, bead) error`
  - Ingests bead target files with bead-specific metadata

### 3.2 Hook into Pipeline Approve

**Modify:** `backend/internal/handler/pipeline.go`
- After artifact is approved (around the git commit in `ApproveInternal()`):
  - Fire-and-forget goroutine: `go rag.IngestArtifact(...)`
- When project reaches `StageComplete`:
  - Fire-and-forget: `go rag.IngestProjectCode(...)`
- Never blocks the approve flow — 30s timeout on ingestion goroutines

### 3.3 Hook into Bead Closure

**Modify:** `backend/internal/handler/bead.go`
- After bead transitions to `BeadStatusClosed`:
  - Fire-and-forget: `go rag.IngestBeadCode(...)`

---

## Phase 4: Feedback Loop (Promote Good, Penalize Bad)

### 4.1 Feedback Service

**Create:** `backend/internal/rag/feedback.go`
- `RecordPositiveOutcome(ctx, client, project, bead) error`
  - Sends `useful` + `high_quality` signals on chunks that informed this bead
- `RecordNegativeOutcome(ctx, client, project, bead, findings) error`
  - Sends `not_useful` on pre-fix code chunks
- `PromoteGoldenPattern(ctx, client, chunkID) error`
  - Calls `POST /promote` with target `golden_patterns`
- `PromoteProjectArtifacts(ctx, client, project) error`
  - Bulk promotes all artifact chunks for completed projects

### 4.2 Hook into Review Outcomes

**Modify:** `backend/internal/handler/bead.go`
- After Devil's Advocate LGTM on first attempt -> `RecordPositiveOutcome()`
- After review rejection -> `RecordNegativeOutcome()`

### 4.3 Hook into Project Completion

**Modify:** `backend/internal/handler/pipeline.go`
- When reaching `StageComplete` -> `PromoteProjectArtifacts()`

---

## Phase 5: UI Integration

### 5.1 RAG Status Badge

**Create:** `frontend/src/components/layout/RAGStatusBadge.tsx`
- Small badge in header showing RAG connected/disconnected
- Polls `GET /api/rag/status` every 30s

### 5.2 RAG Sources in Chat

**Modify:** `frontend/src/types/index.ts` — extend `StreamEvent` with `rag_sources` type
**Modify:** `frontend/src/hooks/useChat.ts` — handle `rag_sources` events
**Modify:** `frontend/src/components/chat/ChatPanel.tsx` — collapsible "Knowledge Sources" section showing chunk file paths, collections, relevance scores, content previews

### 5.3 RAG Context in Bead Detail

**Modify:** `frontend/src/components/build/BeadDetailPanel.tsx`
- "Knowledge Context" tab showing retrieved patterns, promotion status, feedback signals

### 5.4 RAG Settings in Configure Page

**Modify:** `frontend/src/components/configure/ConfigurePage.tsx`
- "RAG Integration" section: base URL, enable/disable, health status, per-stage settings, manual re-ingest trigger

---

## Phase Dependencies

```
Phase 1 (Foundation)
  |
  +---> Phase 2 (Consumption)     -- can start after Phase 1
  |       |
  |       +---> Phase 5.2, 5.3    -- UI for consumed context
  |
  +---> Phase 3 (Production)      -- can start after Phase 1, parallel with Phase 2
  |       |
  |       +---> Phase 4 (Feedback) -- needs Phase 3 (must have content to give feedback on)
  |               |
  |               +---> Phase 5.3  -- UI for feedback signals
  |
  +---> Phase 5.1 (Status Badge)  -- can start in parallel with Phase 2
  +---> Phase 5.4 (Settings UI)   -- can start after Phase 1
```

---

## Verification Plan

1. **Phase 1:** Start RAG pipeline (`docker-compose up` in r3-rag-pipeline), start Paulette, verify `GET /api/rag/status` returns `{ enabled: true, healthy: true }`
2. **Phase 2:** Ingest sample project code into RAG, then start a new Paulette project — verify RAG context appears in SSE events during chat, verify system prompt includes `--- RAG KNOWLEDGE CONTEXT ---` block
3. **Phase 3:** Complete a Paulette project through approve — verify artifacts appear in RAG collections via `POST /search` on the RAG pipeline
4. **Phase 4:** Execute beads with code review — verify feedback signals on RAG chunks via the RAG viewer UI
5. **Phase 5:** Verify UI components render correctly — status badge, knowledge sources in chat, bead detail context tab

---

## Key Files Summary

| File | Phase | Change |
|------|-------|--------|
| `backend/internal/config/config.go` | 1.1 | Add RAGConfig |
| `backend/internal/rag/client.go` | 1.2 | New: HTTP client |
| `backend/internal/rag/models.go` | 1.2 | New: Request/response types |
| `backend/internal/rag/health.go` | 1.3 | New: Health monitor |
| `backend/internal/rag/context.go` | 2.1 | New: Stage context builder |
| `backend/internal/rag/ingest.go` | 3.1 | New: Ingestion service |
| `backend/internal/rag/feedback.go` | 4.1 | New: Feedback service |
| `backend/internal/handler/rag.go` | 1.5 | New: Status endpoint |
| `backend/internal/handler/chat.go` | 2.3 | Add RAG context injection |
| `backend/internal/handler/bead.go` | 2.4, 3.3, 4.2 | RAG augment + ingest + feedback |
| `backend/internal/handler/pipeline.go` | 3.2, 4.3 | Ingest on approve + promote on complete |
| `backend/internal/agent/prompts.go` | 2.2 | Add `GetSystemPromptWithRAG()` |
| `backend/internal/agent/beads.go` | 2.4 | RAG context in code-writer prompt |
| `backend/internal/server/server.go` | 1.4 | Wire RAG client into server + handlers |
| `backend/main.go` | 1.4 | Construct RAG client |
| `frontend/src/api/rag.ts` | 1.6 | New: RAG API client |
| `frontend/src/components/layout/RAGStatusBadge.tsx` | 5.1 | New: Status indicator |
| `frontend/src/components/chat/ChatPanel.tsx` | 5.2 | Show knowledge sources |
| `frontend/src/components/build/BeadDetailPanel.tsx` | 5.3 | Knowledge context tab |
| `frontend/src/components/configure/ConfigurePage.tsx` | 5.4 | RAG settings section |





Here's what was implemented across all 5 phases:

Phase 1 — Foundation (complete)

config.go — Added RAGConfig struct, env var loading (RAG_ENABLED, RAG_BASE_URL)
client.go — New: Nil-safe HTTP client wrapping all RAG endpoints
models.go — New: Go structs mirroring RAG Pipeline OpenAPI schema
health.go — New: Background health monitor with atomic.Bool
rag.go — New: GET /api/rag/status endpoint
server.go — Wired ragClient into Server, handlers, and routes
main.go — Construct RAG client, start health monitor, cancel on shutdown
rag.ts — New: Frontend API for RAG status
Phase 2 — Knowledge Consumption (complete)

context.go — New: Stage-aware context builder (BuildStageContext, BuildBeadContext)
prompts.go — Added GetSystemPromptWithRAG() with RAG context injection
chat.go — RAG context fetched (3s timeout) and injected into system prompt; rag_sources SSE event emitted
Phase 3 — Knowledge Production (complete)

ingest.go — New: IngestArtifact, IngestProjectCode, IngestBeadCode
pipeline.go — Fire-and-forget ingestion on approve + bulk ingest on completion
bead.go — Fire-and-forget ingestion on bead closure
Phase 4 — Feedback Loop (complete)

feedback.go — New: RecordPositiveOutcome, RecordNegativeOutcome, PromoteProjectArtifacts
bead.go — ragOnReviewOutcome() fires positive/negative signals based on Devil's Advocate results
pipeline.go — Promotes all artifacts on project completion
Phase 5 — UI (complete)

index.ts — Extended StreamEvent with rag_sources, added RAGSource type
useChat.ts — Handles rag_sources events, exposes ragSources state
RAGStatusBadge.tsx — New: Green/red badge polling /api/rag/status
RAGSourcesPanel.tsx — New: Collapsible panel showing knowledge sources
ProjectHeader.tsx — Added RAGStatusBadge
ChatPanel.tsx — Added RAGSourcesPanel with ragSources prop
App.tsx — Destructures ragSources from useChat, passes to ChatPanel
All test files updated to match new NewChatHandler signature (added nil ragClient arg).