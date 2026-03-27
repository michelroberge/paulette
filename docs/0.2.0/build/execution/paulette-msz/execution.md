Now let me check how `ChatHandler` is constructed in the server, and what the `StageConfigStore` signature looks like:Now let me check the agent's StreamEvent type and how the frontend handles error events:I have a complete picture. Now I'll implement all the changes needed:

1. **`chat.go`** - the main target: add provider layer, remove direct `agent.Chat()` calls
2. **`server.go`** - wire new dependencies to `ChatHandler`  
3. **`main.go`** - initialize provider stores at startupNow let me check if `NewClaudeCLIProvider` and `FallbackModel` exist in the provider package:I need to add a `FallbackModel` exported function to the provider package since `stageModels` is unexported. Let me add it:Now modify the two `agent.Chat()` call sites in `chat.go` — first `resumeChatRun`:Now modify `StartChatRun`'s `agent.Chat()` call:Now update `server.go` to add the provider fields and pass them to `NewChatHandler`:Now update the `NewChatHandler` call in `server.go`'s `Router()` method:Now update `main.go` to initialize the provider stores and pass them to `server.New()`:Now let's verify the build compiles correctly:Go isn't installed in this environment. Let me do a thorough manual review to verify correctness:The code looks correct. Now let me also check if there are any other callers of `NewChatHandler` that need to be updated:Good, only one call site. Let me also check if `sseWrite` is still used (it was in the original file):Let me verify the `ConnectionStore.Get` method signature to ensure correct usage:`ConnectionStore.Get(id string) (*Connection, error)` — matches exactly how it's used in `buildProviderErrorEvent`. 

Let me verify the final state of `main.go` and `server.go`:Let me also check if `mock.go` or any other handlers call `agent.Chat()` directly (the task only requires chat.go, but let's verify scope is correct):The `mock.go` direct `agent.Chat()` call appears to have already been updated in a previous iteration. The remaining direct calls in `bead.go` and `instruct.go` are out of scope for this task (task 3.5 covers only `chat.go`; task 3.6 covers `mock.go` separately).

Let me do a final check to make sure there are no unused imports in `chat.go` — specifically verify `pipeline` and other imports are still needed:All imports are still in use. The implementation is complete. Here's a summary of all the changes made:

---

## Summary of Changes

### `backend/internal/handler/chat.go` (primary target)

**New types:**
- `connectionErrorPayload` struct — JSON-serializable structured error payload matching the frontend's `ConnectionError` TypeScript interface (`isConnectionError`, `connectionId`, `connectionName`, `reason`)

**`ChatHandler` struct — added 3 new fields:**
- `providerRegistry *provider.Registry` — resolves provider+model per stage
- `stageConfig *provider.StageConfigStore` — reads stage assignments (global + per-project)
- `connStore *provider.ConnectionStore` — used for connection name lookup in error events

**`NewChatHandler` — updated signature** to accept the 3 new fields. All are nil-safe (nil = pure v0.1.0 Claude CLI fallback behaviour).

**New methods:**
- `buildProviderErrorEvent(hostDir, stage, err)` — builds a `StreamEvent{Type: "error"}` with a JSON-encoded `connectionErrorPayload`. Best-effort looks up the connection name from the stage config + conn store.
- `resolveProvider(projectID, hostDir, stage)` — delegates to `registry.ResolveForStage()` when registry is non-nil; falls back to `NewClaudeCLIProvider()` + `FallbackModel(stage)` otherwise.

**`StartChatRun` and `resumeChatRun` — both updated** to:
1. Call `h.resolveProvider()` instead of hardcoded `agent.Chat(stageModels[stage], ...)`
2. If provider resolution fails → emit `connectionErrorPayload` error event, finish run, return `(run, nil)` — the SSE client receives the error via the stream (never an HTTP 500)
3. If `prov.Chat()` fails → same error-via-stream pattern
4. On success → pass a `provider.ChatRequest` struct to `prov.Chat()` and stream events exactly as before

**Removed:** Local `stageModels` var (this is now the `provider.stageModels` fallback inside `ResolveForStage`).

### `backend/internal/server/server.go`
- Added `connStore`, `providerRegistry`, `stageConfig` fields to `Server` struct
- Updated `New()` signature to accept these three new parameters
- Passes them through to `handler.NewChatHandler()`

### `backend/main.go`
- Added `provider` package import
- Initialises `ConnectionStore`, `Registry`, and `StageConfigStore` after loading config
- Passes them to `server.New()`

### `backend/internal/provider/provider.go`
- Added exported `FallbackModel(stage) string` function so `chat.go`'s nil-registry path can return the correct Claude model without accessing the unexported `stageModels` map.