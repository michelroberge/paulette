Let me read the existing test files in detail:Now I have a complete picture. The existing `mixed_provider_routing_test.go` covers generic Ollama+OpenAI routing. The task asks for a concrete scenario with Ollama at `http://10.0.0.57:11434` using `gemma3:4b` for Vision/UX and `codellama:7b` for Architecture/Build. I need to create a new targeted test file for this specific configuration, including both mock-based unit tests and an optional live-integration test.Now I have all the context I need. Let me write the test file for the mixed provider scenario using Ollama with dual-model routing:Now let's run the unit tests to verify they compile and pass:The `//go:build` directives can't be placed inline inside functions. Let me fix this by splitting integration tests into a separate file:Now remove unused imports from the unit test file and create a separate integration test file:Actually, let me just check the current state and verify:All 11 tests pass. Now let me create the integration test file (separate file with build tag) and also clean up the dummy constant I added:Now create the integration test file with the proper build tag at the file level:All 11 unit tests pass. Now let's try the integration tests against the live Ollama at `10.0.0.57:11434`:All integration tests pass against the live Ollama instance. Let's also confirm the full provider test suite still passes cleanly:All tests pass. Here's a summary of what was implemented:

## Implementation Summary

### Task: Test Mixed Provider Configuration (Milestone 6.3)

Two new test files were created in `backend/internal/provider/`:

---

### `ollama_dual_model_routing_test.go` (unit tests — no live Ollama required)

Uses `httptest` mock servers to verify stage-specific routing logic with 11 tests covering:

| Test | What it verifies |
|------|-----------------|
| `VisionAndUX_ResolveToGemma3` | Vision + UX resolve to `*OllamaProvider` with `gemma3:4b` |
| `ArchitectureAndBuild_ResolveToCodellama` | Architecture + Build resolve to `*OllamaProvider` with `codellama:7b` |
| `Complete_FallsBackToCLI` | Complete falls back to `*ClaudeCLIProvider` (unconfigured → inherit Claude CLI) |
| `AllFiveStages` | Full routing table verified in one pass — both provider type and model ID |
| `Chat_ReceivesCorrectModel` | The model name sent in the HTTP request body matches the resolved model |
| `ModelDiscovery` | `ListModels` returns both `gemma3:4b` and `codellama:7b` |
| `ProjectOverride_SwitchesArchitectureModel` | Project-level override for Architecture doesn't bleed into Build |
| `InheritRestoredAfterClear` | Setting override to `nil` correctly reverts to global default |
| `Persistence` | Stage assignments survive a JSON round-trip (simulated server restart) |
| `DynamicReassignment` | Reassigning a stage takes effect on the next call — no caching issue |
| `IndependentProjects` | Two projects can use the same connection with different per-project models |

---

### `ollama_dual_model_integration_test.go` (live integration, `//go:build integration`)

Confirmed working against **http://10.0.0.57:11434** — both `gemma3:4b` and `codellama:7b` were found on the server. Results:

- **Vision** `gemma3:4b` → responded in **1.98s**  
- **UX** `gemma3:4b` → responded in **0.23s** (already warm)  
- **Architecture** `codellama:7b` → responded in **3.64s**  
- **Build** `codellama:7b` → responded in **0.09s** (already warm)  

Run live integration tests with:
```bash
cd backend && go test -tags integration -run TestOllamaDualModelIntegration ./internal/provider/... -v
```