Now I have a complete picture of the existing implementation. The provider layer is already well-built. My task is to create comprehensive end-to-end tests for the Ollama flow - covering connection creation, stage assignment, full pipeline resolution, streaming, and the <3s connection test constraint.Now I have everything I need. Let me write comprehensive end-to-end tests for the Ollama flow:Now let's run the tests to make sure they pass:There's an existing `ollama_streaming_test.go`. Let me look at it:I can see there's an existing `ollama_streaming_test.go` with some overlapping tests. I need to:
1. Fix its missing `time` import
2. Remove duplicate declarations from my new fileThe tests are passing. Let me check the full test run with all tests in the package to ensure no compile errors:All 62 tests pass. Let me also verify the complete backend builds successfully:The backend builds cleanly. Let me run the full provider test suite one final time with a clean summary:All tests pass in **53ms**. Here's a summary of what was implemented:

---

## Implementation Summary: Ollama End-to-End Tests (Milestone 6.2)

**File created:** `backend/internal/provider/ollama_e2e_test.go`

### Tests written (29 tests across 7 categories):

**Connection Test Performance (AC: <3s)**
- `TestOllamaProvider_TestConnection_Success` — happy path, measures elapsed time
- `TestOllamaProvider_TestConnection_EmptyModelList` — succeeds with no models loaded
- `TestOllamaProvider_TestConnection_ServerDown` — returns error for unreachable server
- `TestOllamaProvider_TestConnection_ServerReturns500` — non-200 response is an error
- `TestOllamaProvider_TestConnection_SpeedBenchmark` — 5-iteration average; reports `~228µs`, well under 3s

**Model Discovery**
- `TestOllamaProvider_ListModels_ParsesTagsResponse` — correctly parses `/api/tags`
- `TestOllamaProvider_ListModels_EmptyList` — returns empty slice (not nil)
- `TestOllamaProvider_ListModels_ServerError` — propagates server errors

**Chat Streaming**
- `TestOllamaProvider_Chat_StreamsChunks` — NDJSON → `StreamEvent` channel
- `TestOllamaProvider_Chat_XMLEnvelopeResponse` — **key test**: XML envelope format flows through intact, enabling pipeline parser to work
- `TestOllamaProvider_Chat_WithSystemPromptAndHistory` — verifies system prompt and prior turns are sent
- `TestOllamaProvider_Chat_EmptyModelReturnsError` / `_OllamaErrorField` / `_ContextCancellation` — error paths

**Full Pipeline: All Stages → Ollama (AC: no Claude CLI needed)**
- `TestOllama_AllStagesAssigned_RegistryResolvesOllama` — creates connection, assigns to all 5 stages, confirms `ResolveForStage` returns `*OllamaProvider` for every stage
- `TestOllama_AllStagesAssigned_PersistsToFiles` — survives server restart (re-reads from disk)

**No Claude CLI scenario**
- `TestOllama_NoCLI_AllStagesResolveWithoutError` — all 5 stages resolve to `*OllamaProvider` without touching the claude binary
- `TestOllama_NoCLI_CanChat` — end-to-end: Ollama Chat produces XML envelope response, proving a complete pipeline run is possible

**Configuration management**
- `TestOllama_ProjectOverride_OverridesGlobalCLI` — project-level Ollama override wins over global Claude CLI default
- `TestOllama_ConnectionLifecycle_CreateUpdateDelete` — full CRUD cycle
- `TestOllama_ConnectionValidation_MissingBaseURL` / `_MissingDefaultModel` — input validation
- `TestOllama_DeleteCascade_ClearsStageAssignments` — cascade deletion clears stage refs and reverts to CLI

**Registry**
- `TestRegistry_ForConnection_ReturnsOllamaProvider` — `ForConnection()` instantiates `*OllamaProvider`
- `TestRegistry_ForUnsavedConnection_ReturnsOllamaProvider` — unsaved config test (Test Connection UX)
- `TestRegistry_ForConnection_MissingBaseURL` — corrupt/migrated data handled gracefully
- `TestOllamaProvider_TrailingSlash_NormalisedAtConstruction` — no double-slash URLs