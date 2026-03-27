Let me read the existing test files and key source files to understand what's already there:Now let me look at the key provider and handler files:Let me check the remaining important provider files:Now let me run the existing tests to see their status:Tests pass and the frontend builds cleanly. Now let me look at a few more files to understand what tests to add:Now I have a complete understanding. Let me write the comprehensive tests for Milestone 6. The existing tests cover the core fallback scenarios (M6.1), file permissions (M6.6), and stage config CRUD. I need to add tests for:

1. Ollama streaming parsing (M6.2 enablement)
2. OpenAI-compatible SSE parsing
3. ConnectionHandler HTTP CRUD (M6.5)
4. Error event payload structure (M6.4)
5. Mixed provider routing integration (M6.3)There's already an `ollama_e2e_test.go` file. Let me check it:Let me see the full existing e2e test file and then fix my new test file:The existing e2e test file is very comprehensive. Let me delete my redundant file and check what handler tests exist:Good. Now let me write the missing tests. I need tests for:
1. `ConnectionHandler` HTTP endpoints (CRUD, cascade deletion, test)
2. OpenAI-compatible SSE streaming parser
3. Provider error event format for the error bannerI need to fix compilation issues. Let me check what the ConnectionStore exposes:Handler tests pass. Now let me add the OpenAI SSE streaming tests and the provider error event tests:I need to add the `collectEvents` helper to my test file since it's defined in a deleted file:Now let me add the provider error event payload test (M6.4 - connection error banner):All handler tests pass. Now let me add the mixed provider routing integration test (M6.3) and the autonomous mode regression test (M6.7):Now let me add the autonomous mode regression test (M6.7) and also a build/compile regression test:All pass. Now let me also add provider validation tests that directly cover the `validateConnectionInput` function edge cases for M6.6-adjacent validation:All tests pass. Let me run with verbose output to see the full test count:All 100+ tests pass. Let me do a final comprehensive run with counts:145 tests pass, frontend builds, backend compiles. Let me do one final clean test run to confirm everything:All 145 tests pass with zero failures. Here's a summary of everything implemented for Milestone 6 (Integration Testing & Polish):

---

## Milestone 6: Integration Testing & Polish — Complete

### New Test Files Added

**`backend/internal/provider/openai_compat_streaming_test.go`** — SSE streaming validation (M6.3, M6.2 enabling)
- `TestOpenAICompatSSE_*` — 7 tests for the shared SSE parser: normal chunks, empty deltas, comments, non-data lines, malformed JSON, provider error payloads, fallback done sentinel
- `TestOpenAICompatChat_LMStudio_*` — HTTP round-trip test with mock server, verifies no auth headers sent
- `TestOpenAICompatChat_OpenAI_*` — Tests `Authorization: Bearer` header, org/project ID headers, 401 error handling
- `TestOpenAICompatListModels_*` — `/v1/models` parsing
- Interface compliance assertions for `LMStudioProvider` and `OpenAIProvider`

**`backend/internal/provider/mixed_provider_routing_test.go`** — Mixed provider routing (M6.3)
- Vision/UX → Ollama, Architecture/Build → OpenAI, Complete → Claude CLI fallback
- Per-project override overrides global for one stage without bleeding to others
- Dynamic reassignment takes effect immediately (no caching)
- Independent projects get independent routing
- Persistence round-trip (simulates server restart)

**`backend/internal/provider/validation_test.go`** — Connection validation
- All validation rules: empty name, whitespace name, unknown provider type, all 7 valid types, missing BaseURL for Ollama/LMStudio, OpenAI without URL, empty DefaultModel for all non-CLI providers, ClaudeCLI exempt from model requirement

**`backend/internal/handler/connection_handler_test.go`** — ConnectionHandler HTTP surface (M6.5)
- Full CRUD: List (empty + populated), Create (Ollama 201, OpenAI with HasCredentials, validation errors), Get, Update (model change, empty API key preserves existing key), Delete
- Credential redaction: API key never appears in any response, HasCredentials correctly set
- **Cascade deletion test**: deletes a connection assigned to vision+ux stages, verifies both stages cleared and affectedStages returned
- Test/TestNew: mock Ollama success (with model list), unreachable server (success=false not 5xx), unsupported GitHub Copilot (success=false not 4xx)
- ListModels/ListModelsNew: Ollama returns models, Claude CLI returns 422
- Claude CLI connection requires no URL/key fields

**`backend/internal/handler/connection_error_event_test.go`** — Error banner payload (M6.4)
- `isConnectionError: true` always set
- Content always valid JSON
- With stageConfig: `connectionId` and `connectionName` populated for "Go to Configure →" link
- Nil stageConfig/connStore: no panic, safe defaults
- All five pipeline stages work without error

**`backend/internal/handler/autonomous_mode_test.go`** — Autonomous mode regression (M6.7)
- No config: all stages fall back to ClaudeCLI with correct hardcoded models
- Ollama-only: all 5 stages resolve to OllamaProvider
- Mixed providers: cheap model for Vision/UX, powerful for Architecture/Build, fallback for Complete
- Determinism: repeat calls return the same provider (no shared mutable state)

### Milestone 6 Acceptance Criteria Coverage

| Criterion | Status | Tests |
|---|---|---|
| M6.1 — Claude CLI fallback (no config) | ✅ Pre-existing + reinforced | `TestChatHandler_*Fallback*`, `TestAutonomousMode_NoConfig_*` |
| M6.2 — Ollama end-to-end | ✅ Pre-existing comprehensive e2e tests | `ollama_e2e_test.go` (30+ tests) |
| M6.3 — Mixed provider routing | ✅ New tests | `mixed_provider_routing_test.go`, `autonomous_mode_test.go` |
| M6.4 — Connection error recovery | ✅ New tests | `connection_error_event_test.go` |
| M6.5 — Connection deletion cascade | ✅ New tests | `TestConnectionHandler_Delete_CascadesStageAssignments`, `TestFallback_DeletedConnection_*` |
| M6.6 — chmod 600 verification | ✅ Pre-existing exhaustive tests | `connection_store_test.go` (full lifecycle) |
| M6.7 — Autonomous mode with custom providers | ✅ New tests | `autonomous_mode_test.go` |
| Frontend builds (TypeScript) | ✅ | `npm run build` passes |
| Backend compiles | ✅ | `go build` passes |