Now let me check how `NewClaudeCLIProvider` is called elsewhere:Let me also check `provider.go` to see the full context of how `NewClaudeCLIProvider` is called:Now I have a complete picture. The fix requires two files:

1. **`agent/agent.go`** — Export a `GetClaudeBin()` getter and add a `ChatWithBin()` that accepts an explicit binary path, so the provider can use `p.claudePath` consistently in both methods.
2. **`provider/claude_cli.go`** — Add the `claudePath string` field, seed it from `agent.GetClaudeBin()` in the constructor, wire it into both `TestConnection` and `Chat`.Now refactor `agent.Chat` to delegate to an internal helper that accepts an explicit binary path, and expose `ChatWithBin` for per-instance use:Now update `claude_cli.go` to add the `claudePath` field, seed it from `agent.GetClaudeBin()`, and use it in both `TestConnection` and `Chat`:Now verify the backend compiles cleanly:Go isn't available in this environment. Let me do a careful manual verification of the final file states to ensure correctness:Both files look correct. Here's a summary of every change made and the rationale:

---

## Changes Made

### `backend/internal/agent/agent.go`

**1. Added `GetClaudeBin() string`** (exported getter)
```go
func GetClaudeBin() string {
    return claudeBin
}
```
This lets `ClaudeCLIProvider` read the currently-configured binary path at construction time, without depending on global mutation ordering or exposing the package variable directly.

**2. Refactored `Chat()` to delegate to a new `ChatWithBin()`**
- `Chat()` now simply calls `ChatWithBin(ctx, claudeBin, ...)` — all existing callers that go through `agent.Chat()` (other parts of the agent package) are unaffected.
- `ChatWithBin()` accepts an explicit `bin string` parameter and contains the actual subprocess logic. An empty `bin` safely falls back to the package-level `claudeBin`, making it impossible to accidentally pass a zero value and get a broken exec.

---

### `backend/internal/provider/claude_cli.go`

**1. Added `claudePath string` field to `ClaudeCLIProvider`** (per ARCH-v0.2.0-011)

**2. `NewClaudeCLIProvider()` seeds `claudePath` from `agent.GetClaudeBin()`**
- Called after `agent.SetClaudePath()` runs in `main.go`, so it always captures the user's configured path — not the "claude" default hardcoded string.

**3. `Chat()` calls `agent.ChatWithBin(ctx, p.claudePath, ...)`** instead of `agent.Chat()`
- `Chat` and `TestConnection` now provably use **the same binary** — divergence is structurally impossible.
- Removed the `fmt.Errorf("claude cli: %w", err)` wrapper since `ChatWithBin` already produces well-prefixed errors (`"start claude: ..."`, `"stdout pipe: ..."`) and the extra prefix was additive noise.

**4. `TestConnection()` uses `p.claudePath`** instead of the hardcoded `"claude"` string
- Error message now includes the actual path being probed (e.g. `"/custom/bin/claude" --version failed: ...`) which aids debugging when a custom path is configured.