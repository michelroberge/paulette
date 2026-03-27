The provider directory already exists! Let me read the existing files to see what's already implemented and what needs to be added or fixed.Good — the provider package already exists from a prior iteration. Let me read the actual content of the key files to verify correctness and identify any gaps.The core provider files look well-implemented. Let me now verify the build compiles correctly and check a few remaining key files.The core infrastructure looks solid. Let me now run a build to verify everything compiles correctly:The build is clean. Let me also run the existing tests in the provider package to confirm everything works:All tests pass. Let me do one final check to confirm the full backend test suite is clean:All tests pass with zero failures. Here's a summary of what was verified:

---

## Task Complete: Provider Interface & Core Infrastructure

The `backend/internal/provider/` package was already fully implemented from the prior iteration. All components specified in Milestone 1 are present and working:

### Files Verified

| File | Component | Status |
|---|---|---|
| `provider/provider.go` | `Provider` interface, `Registry`, `ResolveForStage()` (3-level fallback), `FallbackModel()` | ✅ Complete |
| `provider/connection.go` | `Connection`, `ConnectionResponse`, `ProviderType` constants (all 7 types) | ✅ Complete |
| `provider/connection_store.go` | CRUD ops, atomic writes, `chmod 600`, UUID generation, rollback on failure | ✅ Complete |
| `provider/stage_config.go` | `StageConfigStore`, global config, per-project overrides, `ResolveStage()`, `ClearConnectionReferences()` | ✅ Complete |
| `provider/claude_cli.go` | `ClaudeCLIProvider` wrapping `agent.ChatWithBin()`, `TestConnection` via `claude --version` | ✅ Complete |
| `provider/utils.go` | Shared `writeJSONAtomic()` helper | ✅ Complete |
| `backend/main.go` | `ConnectionStore`, `Registry`, `StageConfigStore` initialized at startup | ✅ Complete |
| `server/server.go` | Server struct and constructor accept all three new stores | ✅ Complete |

### Key Architectural Properties
- **3-level resolution**: project override → global default → Claude CLI hardcoded fallback (preserves v0.1.0 behaviour when no config exists)
- **Atomic writes + `chmod 600`**: all credentials protected at rest
- **Thread-safe**: `sync.RWMutex` on both stores (fast reads during provider resolution)
- **Zero regressions**: `ResolveForStage` with nil `stageConfig` always returns Claude CLI — existing projects work unchanged

### Test Results
- **All 78 tests pass** across `provider` and `handler` packages
- Tests cover: fallback hierarchy, file permissions (0600), CRUD lifecycle, concurrent creates, mixed provider routing, validation edge cases