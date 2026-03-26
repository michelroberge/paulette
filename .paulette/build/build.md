# Build Plan: Paulette v0.2.0 — Pluggable LLM Provider Layer

## Pre-existing Code Context

The following code exists from v0.1.0 and will be **modified or extended** (not rebuilt):

| File | Role | Modification Type |
|---|---|---|
| `backend/main.go` | Server entry point | Extended — init new stores, pass to server |
| `backend/internal/server/server.go` | Chi router | Extended — mount new route groups |
| `backend/internal/agent/agent.go` | Claude CLI subprocess wrapper | Retained as-is; wrapped by new adapter |
| `backend/internal/handler/chat.go` | Chat SSE handler | Modified — resolve provider instead of direct `agent.Chat()` |
| `backend/internal/handler/mock.go` | Mock generation handler | Modified — resolve provider instead of direct `agent.Chat()` |
| `backend/internal/autopilot/orchestrator.go` | Autonomous mode | No direct changes (uses handlers which resolve providers) |
| `backend/internal/model/project.go` | Project model | Unchanged |
| `backend/internal/stream/manager.go` | SSE stream manager | Unchanged |
| `frontend/src/App.tsx` | Top-level component | Modified — add React Router, "Configure Paulette" button |
| `frontend/src/types/index.ts` | TypeScript types | Extended — add provider types |
| `frontend/src/api/client.ts` | API fetch helper | Unchanged (reused) |

---

## Milestones

### Milestone 1: Provider Interface & Core Infrastructure
**Goal:** Establish the `Provider` interface, connection store, and stage config store in the backend. This is the foundation that all subsequent work depends on.

**Tasks:**

- [ ] **1.1** Create `provider/provider.go` — Define `Provider` interface (`Chat`, `TestConnection`, `ListModels`), `ChatRequest`, `ModelInfo`, `StreamEvent` re-export, `ErrModelListUnsupported`, and `Registry` struct with `ForConnection()` and `ResolveForStage()` methods
  - **Creates:** `backend/internal/provider/provider.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-005, JRN-v0.2.0-008
  - Implements: ARCH-v0.2.0-010
  - AC: Interface compiles; `ResolveForStage` implements the 3-level fallback (project override → global default → Claude CLI hardcoded)

- [ ] **1.2** Create `provider/connection.go` — Define `Connection`, `ConnectionResponse`, `ProviderType` constants
  - **Creates:** `backend/internal/provider/connection.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-002
  - Implements: ARCH-v0.2.0-018 (data model portion)
  - AC: All 7 provider types defined; `ConnectionResponse` redacts `APIKey` into `HasCredentials` bool

- [ ] **1.3** Create `provider/connection_store.go` — CRUD for connections with `~/.paulette/connections.json` persistence, atomic writes, `chmod 600`
  - **Creates:** `backend/internal/provider/connection_store.go`
  - Serves journeys: JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004
  - Implements: ARCH-v0.2.0-018
  - AC: `List`, `Get`, `Create`, `Update`, `Delete` all work; file written atomically with 0600 permissions; UUID assigned on create; `Delete` returns affected stage assignments

- [ ] **1.4** Create `provider/stage_config.go` — Global defaults (`~/.paulette/config.json`) and per-project overrides (`<hostDir>/.paulette/stage_config.json`)
  - **Creates:** `backend/internal/provider/stage_config.go`
  - Serves journeys: JRN-v0.2.0-006, JRN-v0.2.0-007
  - Implements: ARCH-v0.2.0-019
  - AC: `GetGlobalDefaults`, `SetGlobalStageDefault`, `GetProjectOverrides`, `SetProjectStageOverride`, `ResetProjectOverrides`, `ClearConnectionReferences` all work; files written atomically

- [ ] **1.5** Create `provider/claude_cli.go` — Wrap existing `agent.Chat()` behind the `Provider` interface
  - **Creates:** `backend/internal/provider/claude_cli.go`
  - **References:** `backend/internal/agent/agent.go` (calls `agent.Chat()` directly)
  - Serves journeys: JRN-v0.1.0-002, JRN-v0.2.0-001
  - Implements: ARCH-v0.2.0-011
  - AC: `Chat` delegates to `agent.Chat()`; `TestConnection` runs `claude --version`; `ListModels` returns `ErrModelListUnsupported`

**Depends on:** Nothing (greenfield `provider/` package)

---

### Milestone 2: LLM Provider Implementations
**Goal:** Implement all P1 and P2 provider backends. Each provider converts its native streaming format into the shared `StreamEvent` channel.

**Tasks:**

- [ ] **2.1** Create `provider/openai_compat.go` — Shared OpenAI-compatible implementation: `openAICompatChat()`, `openAICompatListModels()`, `openAICompatTestConnection()`, SSE line parsing (`data: {...}` → `StreamEvent`), message format conversion
  - **Creates:** `backend/internal/provider/openai_compat.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-017
  - AC: Handles `data: [DONE]` sentinel; parses `choices[0].delta.content`; supports optional auth header and base URL; 30s test timeout, 10min chat timeout

- [ ] **2.2** Create `provider/ollama.go` — Ollama provider using `POST /api/chat` (NDJSON streaming) and `GET /api/tags` (model list)
  - **Creates:** `backend/internal/provider/ollama.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-012
  - AC: Streams NDJSON; converts `{"message":{"content":"..."}, "done":false}` to `StreamEvent`; `ListModels` parses `/api/tags` response; no auth headers sent

- [ ] **2.3** Create `provider/lmstudio.go` — LM Studio provider delegating to `openai_compat` functions with no auth
  - **Creates:** `backend/internal/provider/lmstudio.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-013
  - AC: All three interface methods delegate to `openAICompat*` functions; base URL required; no auth header

- [ ] **2.4** Create `provider/openai.go` — OpenAI provider delegating to `openai_compat` functions with `Authorization: Bearer {key}` header, optional org/project headers
  - **Creates:** `backend/internal/provider/openai.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-014
  - AC: Sets `Authorization`, optional `OpenAI-Organization` and `OpenAI-Project` headers; default base URL `https://api.openai.com`

- [ ] **2.5** Create `provider/anthropic.go` — Anthropic API provider using `POST /v1/messages` with typed SSE events (`content_block_delta`, `message_stop`)
  - **Creates:** `backend/internal/provider/anthropic.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-015
  - AC: Sets `x-api-key` and `anthropic-version` headers; parses `content_block_delta` events; `ListModels` returns hardcoded list of known Anthropic models; `TestConnection` sends minimal 1-token request

- [ ] **2.6** Create `provider/gemini.go` — Gemini provider using `streamGenerateContent` endpoint with API key as query param
  - **Creates:** `backend/internal/provider/gemini.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-016
  - AC: API key passed as `?key=` query param; parses `candidates[0].content.parts[0].text` from NDJSON array; `ListModels` uses `GET /v1beta/models`; default base URL `https://generativelanguage.googleapis.com`

- [ ] **2.7** Wire `Registry.ForConnection()` — Implement the factory method that instantiates the correct provider type based on `Connection.ProviderType`
  - **Modifies:** `backend/internal/provider/provider.go`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-008
  - Implements: ARCH-v0.2.0-010
  - AC: All 6 active provider types are instantiated correctly; `github_copilot` returns an error ("not yet supported"); unknown types return an error

**Depends on:** Milestone 1

---

### Milestone 3: Backend API Handlers & Integration
**Goal:** Expose connection CRUD, test, model list, and stage config via HTTP API. Modify existing chat/mock handlers to route through the provider layer.

**Tasks:**

- [ ] **3.1** Create `handler/connection.go` — Connection CRUD handlers (`List`, `Create`, `Get`, `Update`, `Delete`), `Test`/`TestNew`, `ListModels`/`ListModelsNew`
  - **Creates:** `backend/internal/handler/connection.go`
  - Serves journeys: JRN-v0.2.0-001 through JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-020
  - AC: `List`/`Get` redact credentials (return `ConnectionResponse`); `Create` assigns UUID and timestamps; `Delete` calls `ClearConnectionReferences` and returns affected stages; `Test`/`TestNew` call `provider.TestConnection()` and optionally `ListModels()` in parallel; all errors return structured JSON

- [ ] **3.2** Create `handler/config_handler.go` — Stage config handlers (`GetGlobalDefaults`, `SetGlobalStageDefault`, `GetProjectOverrides`, `SetProjectStageOverride`, `ResetProjectOverrides`)
  - **Creates:** `backend/internal/handler/config_handler.go`
  - Serves journeys: JRN-v0.2.0-006, JRN-v0.2.0-007
  - Implements: ARCH-v0.2.0-021
  - AC: `PUT .../stages/:stage` validates stage name; `PUT` with null body clears override; `POST .../reset` clears all project overrides; all endpoints read/write via `StageConfigStore`

- [ ] **3.3** Mount new routes in `server/server.go` — Add `/api/connections/*` and `/api/config/*` route groups
  - **Modifies:** `backend/internal/server/server.go`
  - Serves journeys: JRN-v0.2.0-001 through JRN-v0.2.0-007
  - Implements: ARCH-v0.2.0-002
  - AC: All new endpoints reachable; existing routes unchanged; server struct accepts `connStore`, `providerRegistry`, `stageConfig`

- [ ] **3.4** Modify `main.go` — Initialize `ConnectionStore`, `Registry`, `StageConfigStore` on startup; pass to server constructor
  - **Modifies:** `backend/main.go`
  - Implements: ARCH-v0.2.0-001
  - AC: `connections.json` and `config.json` loaded at boot; missing files handled gracefully (empty defaults)

- [ ] **3.5** Modify `handler/chat.go` — Replace direct `agent.Chat()` call with `registry.ResolveForStage()` → `provider.Chat()`. Emit structured error `StreamEvent` on provider failure
  - **Modifies:** `backend/internal/handler/chat.go`
  - Serves journeys: JRN-v0.1.0-002, JRN-v0.2.0-008
  - Implements: ARCH-v0.2.0-006
  - AC: `stageModels` map remains as fallback; `StartChatRun` resolves provider + model via registry; connection errors produce `StreamEvent{Type: "error"}` with connection name and reason; existing Claude CLI path still works when no config exists

- [ ] **3.6** Modify `handler/mock.go` — Same provider-resolution refactor as chat handler
  - **Modifies:** `backend/internal/handler/mock.go`
  - Serves journeys: JRN-v0.1.0-005, JRN-v0.2.0-008
  - Implements: ARCH-v0.2.0-007
  - AC: Mock generation uses resolved provider; error handling matches chat handler pattern

**Depends on:** Milestones 1, 2

---

### Milestone 4: Frontend Foundation & Routing
**Goal:** Add React Router, TypeScript types, API client functions, and the Configure Paulette page shell.

**Tasks:**

- [ ] **4.1** Install `react-router-dom` and set up client-side routing in `App.tsx` — Routes: `/` (home), `/projects/:id` (project detail), `/configure` (configure page)
  - **Modifies:** `frontend/src/App.tsx`, `frontend/package.json`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-008
  - Implements: ARCH-v0.2.0-032
  - AC: All three routes render correct views; existing project list and project detail views function identically; URL reflects current view; back/forward navigation works; `?tab=` and `?highlight=` query params parsed on `/configure`

- [ ] **4.2** Create `types/provider.ts` — TypeScript types for `ProviderType`, `Connection`, `ConnectionInput`, `ModelInfo`, `TestResult`, `DeleteResult`, `StageAssignment`, `GlobalStageConfig`, `ProjectStageConfig`
  - **Creates:** `frontend/src/types/provider.ts`
  - Serves journeys: JRN-v0.2.0-001 through JRN-v0.2.0-007
  - Implements: ARCH-v0.2.0-031
  - AC: All types match backend API contract; `Connection` has `hasCredentials` (not `apiKey`); `ConnectionInput` has `apiKey` for create/update only

- [ ] **4.3** Create `api/connections.ts` — API client functions for connection CRUD, test, and model discovery
  - **Creates:** `frontend/src/api/connections.ts`
  - Serves journeys: JRN-v0.2.0-001 through JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-029
  - AC: All 8 functions (`listConnections`, `createConnection`, `updateConnection`, `deleteConnection`, `testConnection`, `testNewConnection`, `listModels`, `listModelsForNew`) implemented using existing `apiFetch` helper

- [ ] **4.4** Create `api/stageConfig.ts` — API client functions for global defaults and project overrides
  - **Creates:** `frontend/src/api/stageConfig.ts`
  - Serves journeys: JRN-v0.2.0-006, JRN-v0.2.0-007
  - Implements: ARCH-v0.2.0-030
  - AC: All 5 functions implemented; `setProjectStageOverride` sends `null` to clear override

- [ ] **4.5** Modify home page — Add "Configure Paulette" button alongside "New Project" and "Import Project"
  - **Modifies:** `frontend/src/App.tsx` (or extracted `ProjectList` component)
  - Serves journeys: JRN-v0.2.0-001
  - Implements: ARCH-v0.2.0-033, SCR-001
  - AC: Button styled as secondary (`bg-white border border-gray-300`); navigates to `/configure`; appears alongside existing action buttons

**Depends on:** Nothing frontend-side (can run in parallel with Milestones 1–3 for types/API stubs)

---

### Milestone 5: Frontend Configure Page & Components
**Goal:** Build the full Configure Paulette UI — Connections tab, Connection form, Stage Defaults tab, Project Stage Settings panel, and error/toast components.

**Tasks:**

- [ ] **5.1** Create `components/configure/ConfigurePage.tsx` — Two-tab layout (Connections, Stage Defaults) with breadcrumb, query param support for `?tab=` and `?highlight=`
  - **Creates:** `frontend/src/components/configure/ConfigurePage.tsx`
  - Serves journeys: JRN-v0.2.0-001, JRN-v0.2.0-006
  - Implements: ARCH-v0.2.0-022, SCR-007
  - AC: Tabs switch content; `?tab=connections` or `?tab=defaults` selects correct tab; `← Home` breadcrumb navigates back; `max-w-4xl` centered layout

- [ ] **5.2** Create `components/configure/ConnectionsTab.tsx` — Connection list with provider badges, Add/Edit/Delete/Test actions, empty state
  - **Creates:** `frontend/src/components/configure/ConnectionsTab.tsx`
  - Serves journeys: JRN-v0.2.0-001 through JRN-v0.2.0-004
  - Implements: ARCH-v0.2.0-023, SCR-008
  - AC: Connections fetched on mount; provider badges colour-coded per spec; "Add Connection" button opens form; Edit pre-fills form; Delete shows confirmation with affected stages; `?highlight=` causes matching card to pulse with `ring-2 ring-red-400`; empty state with plug icon

- [ ] **5.3** Create `components/configure/ConnectionForm.tsx` — Slide-over form with provider-type-driven fields, Test Connection, model discovery, credential warning
  - **Creates:** `frontend/src/components/configure/ConnectionForm.tsx`
  - Serves journeys: JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-005
  - Implements: ARCH-v0.2.0-024, SCR-009, IACT-007, IACT-008
  - AC: Provider type selection dynamically shows/hides fields (no loading state); GitHub Copilot disabled with "Coming Soon" badge; Test Connection shows spinner → ✓/✗ inline; model dropdown populated on successful discovery, free-text fallback otherwise; credential warning banner shown when API key non-empty; Save/Cancel footer

- [ ] **5.4** Create `components/configure/StageDefaultsTab.tsx` — Five-stage table with connection picker and model field, auto-save per row
  - **Creates:** `frontend/src/components/configure/StageDefaultsTab.tsx`
  - Serves journeys: JRN-v0.2.0-006
  - Implements: ARCH-v0.2.0-025, SCR-010, IACT-010
  - AC: Each row shows stage pip + label, connection dropdown (all connections + "Claude CLI (default)"), model field (dropdown or free-text); changes auto-save on blur/change; "Saved ✓" fades in/out per row; note about global vs project scope at bottom

- [ ] **5.5** Create `components/configure/ProjectStageSettings.tsx` — Slide-over panel with Inherit toggles, per-stage connection/model override, "Reset all" action
  - **Creates:** `frontend/src/components/configure/ProjectStageSettings.tsx`
  - Serves journeys: JRN-v0.2.0-007
  - Implements: ARCH-v0.2.0-026, SCR-011, IACT-012
  - AC: Inherit toggle ON → read-only muted values; OFF → editable pickers pre-populated from global; override rows have `bg-yellow-50` tint; "Reset all" with confirmation dialog; auto-save per row

- [ ] **5.6** Modify project header — Add ⚙ gear icon button; show `ring-2 ring-yellow-400` accent on pips with overrides
  - **Modifies:** `frontend/src/App.tsx` or extracted project header component
  - Serves journeys: JRN-v0.2.0-007
  - Implements: ARCH-v0.2.0-034, SCR-002
  - AC: Gear icon opens ProjectStageSettings panel; tooltip "Stage connection settings"; pips with project-level overrides show yellow ring accent

- [ ] **5.7** Create `components/chat/ConnectionErrorBanner.tsx` — Inline error banner with "Go to Configure →" deep-link and "Change stage connection" shortcut
  - **Creates:** `frontend/src/components/chat/ConnectionErrorBanner.tsx`
  - Serves journeys: JRN-v0.2.0-008
  - Implements: ARCH-v0.2.0-027, SCR-012, IACT-011
  - AC: Renders on `StreamEvent{type: "error"}` with connection info; "Go to Configure →" links to `/configure?tab=connections&highlight={id}`; "Change stage connection" opens ProjectStageSettings; banner persists until user navigates or retries

- [ ] **5.8** Create `components/configure/CredentialWarningToast.tsx` — Toast notification for credential saves, auto-dismiss after 6s
  - **Creates:** `frontend/src/components/configure/CredentialWarningToast.tsx`
  - Serves journeys: JRN-v0.2.0-002, JRN-v0.2.0-003
  - Implements: ARCH-v0.2.0-028, IACT-009
  - AC: Fixed bottom-right position; yellow warning styling; auto-dismisses after 6s with fade-out; only shown for providers that have credentials (not Ollama/LM Studio/Claude CLI)

- [ ] **5.9** Integrate ConnectionErrorBanner into chat panel — Detect connection error events and render banner in ChatPanel
  - **Modifies:** `frontend/src/components/chat/ChatPanel.tsx` and/or `frontend/src/hooks/useChat.ts`
  - Serves journeys: JRN-v0.2.0-008
  - Implements: ARCH-v0.2.0-027
  - AC: When SSE stream emits an error event with connection failure info, the banner replaces the streaming area; chat input remains available for retry after fix

**Depends on:** Milestone 4 (routing, types, API clients)

---

### Milestone 6: Integration Testing & Polish
**Goal:** End-to-end validation, edge case handling, and regression testing.

**Tasks:**

- [ ] **6.1** Verify Claude CLI fallback — Confirm that with no `connections.json` or `config.json`, the pipeline works identically to v0.1.0
  - Serves journeys: All JRN-v0.1.0-*
  - AC: New project → Vision stage fires with Claude Sonnet via CLI; Architecture fires with Claude Opus; no configuration required; zero regressions

- [ ] **6.2** Test Ollama end-to-end — Create Ollama connection, assign to all stages, run full pipeline
  - Serves journeys: JRN-v0.2.0-001
  - AC: User with no Claude CLI can complete a full pipeline using Ollama alone; connection test passes in <3s for local Ollama

- [ ] **6.3** Test mixed provider configuration — Assign different connections to different stages, verify stage-specific resolution
  - Serves journeys: JRN-v0.2.0-006, JRN-v0.2.0-007
  - AC: Vision/UX use Ollama, Architecture/Build use OpenAI; each stage correctly routes to its assigned provider and model

- [ ] **6.4** Test connection error recovery flow — Simulate unreachable provider, verify error banner, fix, retry
  - Serves journeys: JRN-v0.2.0-008
  - AC: Error banner appears immediately (no silent hang); "Go to Configure" deep-links correctly with highlight; after fixing connection, retry works without data loss

- [ ] **6.5** Test connection deletion cascade — Delete a connection used in stage assignments, verify fallback
  - Serves journeys: JRN-v0.2.0-004
  - AC: Affected stages reported in response; stage assignments cleared; stages fall back to Claude CLI default

- [ ] **6.6** Verify `chmod 600` on `connections.json` — Confirm file permissions on every save operation
  - AC: `stat ~/.paulette/connections.json` shows `0600` permissions after create, update, and delete operations

- [ ] **6.7** Verify autonomous mode with custom providers — Run full autonomous pipeline with non-Claude provider
  - Serves journeys: JRN-v0.1.0-004, JRN-v0.2.0-008
  - AC: Autonomous mode drives through all stages using configured provider; error mid-pipeline surfaces correctly

---

## Dependencies

```
Milestone 1 (Provider Interface & Stores)
    │
    ▼
Milestone 2 (Provider Implementations)
    │
    ▼
Milestone 3 (Backend API & Handler Integration)
    │
    ├── 3.5, 3.6 depend on 2.7 (registry wiring)
    │
    ▼
Milestone 6 (Integration Testing)

Milestone 4 (Frontend Foundation & Routing) ← Can start in parallel with M1-M3
    │
    ▼
Milestone 5 (Frontend Configure UI)
    │
    ▼
Milestone 6 (Integration Testing)
```

**Parallelism opportunities:**
- M1 + M4 can start simultaneously (backend provider package + frontend types/routing)
- M2 tasks (2.1–2.6) can be developed in parallel (independent provider implementations)
- M5 tasks (5.1–5.8) can be partially parallelized (page shell → tabs/form → panel/toast)

**Critical path:** M1 → M2 → M3.5 (chat handler modification) → M6.1 (regression test)

---

## Risks & Unknowns

| Risk | Impact | Mitigation |
|---|---|---|
| **XML envelope compliance** — Smaller models (Ollama, Gemini) may not reliably follow the `<response><artifact>...</artifact></response>` format that `agent/parse.go` depends on | Pipeline may fail to extract artifacts from non-Claude providers | Document as user responsibility per vision; consider adding a lenient parser fallback in a future iteration |
| **Streaming format variance** — Each provider has a different streaming protocol (NDJSON, SSE, typed SSE, JSON arrays) | Bugs in stream parsing could cause hangs or data loss | Each provider gets dedicated streaming tests; Ollama (NDJSON) and OpenAI (SSE) are the two distinct patterns to validate first |
| **Anthropic API rate limits** — Direct API calls may hit rate limits that the CLI abstracted away | User sees errors during heavy usage | Surface clear error messages; rate limiting/retry is explicitly out of scope for v0.2.0 |
| **React Router migration** — Adding routing to an app that currently uses state-only navigation is a significant refactor | Could break existing navigation patterns, especially mobile sidebar and tab state | Carefully preserve all existing state management; routes map 1:1 to existing views; test mobile layout |
| **Connection store concurrency** — Multiple goroutines (autopilot, SSE handlers) may access connections simultaneously | Race conditions on read during provider resolution | `sync.RWMutex` on `ConnectionStore` and `StageConfigStore` (already in arch spec) |
| **Gemini streaming format** — Gemini returns a JSON array streamed incrementally, which is unusual | Parsing complexity; potential for incomplete JSON chunks | Implement line-by-line parsing with buffering; test with real Gemini API |

---

## Definition of Done

1. **All 6 provider implementations** (Claude CLI, Ollama, LM Studio, OpenAI, Anthropic, Gemini) pass `TestConnection` and `Chat` against their respective APIs
2. **Connection CRUD** works end-to-end: create, edit, delete connections via the Configure Paulette UI; `connections.json` persisted with `chmod 600`
3. **Stage configuration** works at both levels: global defaults in `config.json`, per-project overrides in `stage_config.json`; inheritance toggle functions correctly
4. **Zero pipeline regressions**: existing Claude CLI–backed projects work without reconfiguration after upgrade (M6.1)
5. **Ollama-only pipeline**: a user with no Claude CLI installed can run the full pipeline using Ollama (M6.2)
6. **Mixed provider routing**: different connections assigned to different stages all resolve correctly (M6.3)
7. **Error recovery**: connection failures surface immediately with actionable UI (M6.4)
8. **Frontend compiles and builds**: `npm run build` succeeds with no TypeScript errors
9. **Backend compiles and builds**: `go build` succeeds with no errors
10. **GitHub Copilot** shows as "Coming Soon" in provider dropdown — not selectable

---
*Added by Further Instructions — 2026-03-26*

---

## Testing Environment

### Ollama Instance
| Setting | Value |
|---|---|
| Base URL | `http://10.0.0.57:11434` |
| Available Models | `gemma3:4b`, `codellama:7b` |

**Suggested stage assignments for testing:**
- Vision / UX / Complete stages → `gemma3:4b` (general-purpose, good for natural language)
- Architecture / Build stages → `codellama:7b` (code-oriented)