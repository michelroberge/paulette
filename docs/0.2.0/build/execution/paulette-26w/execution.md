Now I have a full picture of the codebase. Let me look at the agent package to understand `GetClaudeBin` and the model package for stage names:Let me quickly check the Ollama provider type name:Good. Now I have all the information I need to write the tests. Let me create the comprehensive fallback test file:Now let me also create a handler-level regression test that verifies the nil-registry path in `ChatHandler.resolveProvider`:Now let me write the handler-level test for the nil-registry path:Now let me verify the repository interfaces to make sure my nil implementations are correct:I need to fix my nil interface implementations - the interfaces don't match what I wrote. Let me check the stream.Manager too:Good, `stream.NewManager()` exists. Now let me fix the handler test file with correct interface implementations:Now let me check the model.BtwMessage and model.StageActivity types to make sure the nop interfaces are correct:Good. Now let me run the tests to verify they compile and pass:All 27 tests pass. Let me do a final full build check to make sure nothing is broken:All tests pass and the build is clean. Here's a summary of what was implemented:

## Zero Regression Test — Claude CLI Fallback (Milestone 6.1)

### What was done

Created two new test files that comprehensively verify the three-level fallback chain preserves v0.1.0 behavior when no `connections.json` or `config.json` exist:

---

**`backend/internal/provider/claude_cli_fallback_test.go`** (19 tests) — Tests at the provider layer:

| Test | What it verifies |
|---|---|
| `TestFallbackModel_AllStages` | Vision/UX/Complete → `claude-sonnet-4-6`; Architecture/Build → `claude-opus-4-6` |
| `TestFallbackModel_UnknownStage` | Unknown stage names get `claude-sonnet-4-6` (safe default) |
| `TestClaudeCLIProvider_ImplementsProviderInterface` | Compile-time assertion that `ClaudeCLIProvider` satisfies `Provider` |
| `TestClaudeCLIProvider_ListModels_ReturnsUnsupported` | Returns `ErrModelListUnsupported` (frontend shows free-text field) |
| `TestConnectionStore_MissingFile_StartsEmpty` | No `connections.json` → empty store, no panic, no file created as side-effect |
| `TestConnectionStore_Get_NotFound` | Empty store returns error gracefully on `Get` |
| `TestStageConfigStore_MissingGlobalConfig_EmptyDefaults` | No `config.json` → empty `GlobalConfig`, no error |
| `TestStageConfigStore_MissingProjectConfig_EmptyOverrides` | No `stage_config.json` → empty overrides, no error |
| `TestStageConfigStore_ResolveStage_MissingFiles` | All 5 stages return `nil` (triggering Claude CLI fallback) |
| `TestRegistry_ResolveForStage_NoConfigFiles` | **Core regression test**: all 5 stages return `*ClaudeCLIProvider` + correct model |
| `TestRegistry_ResolveForStage_NilStageConfig` | Nil `stageConfig` → Claude CLI fallback for all stages |
| `TestRegistry_ResolveForStage_EmptyAssignment_FallsBackToCLI` | Empty `connectionId` in config → treated as inherit, falls through to Claude CLI |
| `TestFallback_ProjectOverrideTakesPrecedenceOverGlobalAndCLI` | Level 1 > Level 2 > Level 3 priority verified |
| `TestFallback_GlobalDefaultTakesPrecedenceOverCLI` | Level 2 > Level 3 priority verified |
| `TestFallback_DeletedConnection_FallsBackToCLI` | After connection deletion + cascade clear, stage reverts to Claude CLI |

---

**`backend/internal/handler/chat_fallback_test.go`** (8 tests) — Tests at the HTTP handler layer:

| Test | What it verifies |
|---|---|
| `TestChatHandler_NilRegistry_ResolveProviderFallback` | Nil `providerRegistry` → all 5 stages fall back to `*ClaudeCLIProvider` + correct model |
| `TestChatHandler_WithRegistryNoConfig_ResolveProviderFallback` | Real registry with no config files → same Claude CLI fallback |