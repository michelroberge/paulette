Now let me read the chat handler to understand the pattern used there, and also the provider package:Let me also read the agent.go and provider claude_cli.go files:Let me check the server and main to understand MockHandler construction:Now I have a full picture. Here's my implementation plan:

1. Export mock-specific helpers from `agent/mock.go` (so the handler can use them without re-implementing)
2. Update `handler/mock.go` to add provider fields and refactor `StartMockRun` to route through the provider layer
3. Update the `NewMockHandler` call in `server/server.go` to pass the provider dependencies

Let's implement:Now update `handler/mock.go` to add the provider layer:Now replace `StartMockRun` to use the provider layer with the retry loop:Now update `server.go` to pass the provider dependencies to `NewMockHandler`:Now let's verify everything compiles:Go binary isn't available in the environment, so let me do a thorough manual review of all modified files to ensure correctness:Let me verify the `StageConfigStore.ResolveStage` method exists (used in `buildProviderErrorEvent`):Good. Let me also verify the `connectionErrorPayload` type (shared from `chat.go`) is accessible within the `handler` package:`connectionErrorPayload` is defined in `chat.go` within the same `handler` package — `mock.go` can use it directly since they're in the same package. ✓

Let me do a final cross-check on all the key aspects:Everything looks correct. Let me provide a summary of all the changes made:

---

## Summary of Changes

### `backend/internal/agent/mock.go` (modified)
Added four exported symbols to allow the mock handler to implement the retry loop using the provider layer:
- **`MaxMockRetries`** — exported constant aliasing `maxMockRetries = 2`
- **`HasHTMLBlock(s string) bool`** — exported wrapper over `hasHTMLBlock`
- **`BuildMockSystemPrompt(cfg *model.FrameworkConfig) string`** — exported wrapper over `buildMockSystemPrompt`
- **`MockRetryPrompt`** — exported string constant aliasing `mockRetryPrompt`

The original `GenerateMock` function and all Claude CLI–specific internals are untouched.

---

### `backend/internal/handler/mock.go` (modified)
**Struct + constructor:**
- Added `providerRegistry *provider.Registry`, `stageConfig *provider.StageConfigStore`, `connStore *provider.ConnectionStore` fields to `MockHandler`
- Updated `NewMockHandler` signature to accept these three; all may be `nil` for backward compatibility

**New methods:**
- `resolveProvider(projectID, hostDir string, stage model.StageName)` — delegates to `registry.ResolveForStage` when available, falls back to `NewClaudeCLIProvider()` (identical pattern to `ChatHandler`)
- `buildProviderErrorEvent(hostDir string, stage model.StageName, err error)` — builds a structured `StreamEvent{Type: "error"}` with connection metadata (reuses the `connectionErrorPayload` type defined in `chat.go`)

**`StartMockRun` refactored:**
- Removed direct `agent.GenerateMock()` call
- Now calls `h.resolveProvider()` before starting the goroutine; emits a structured provider error if resolution fails
- The background goroutine implements the HTML-envelope retry loop (up to `agent.MaxMockRetries` attempts) using `prov.Chat()` directly:
  - `chunk`/other events forwarded to `run` for real-time streaming
  - `tokens` events accumulate into `stageTokensAccum`
  - `error` events forwarded and short-circuit the loop
  - `done` event Content (full response) checked with `agent.HasHTMLBlock()`; if valid, HTML extracted, saved to disk, and `done` emitted; if invalid, retry prompt substituted

---

### `backend/internal/server/server.go` (modified)
Updated the `NewMockHandler` call at line 103 to pass `s.providerRegistry`, `s.stageConfig`, and `s.connStore` — the same fields already used by `NewChatHandler`.