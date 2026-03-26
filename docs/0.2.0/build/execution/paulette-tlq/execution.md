The `main.go` is already fully implemented with the provider initialization. Let me verify the `ConnectionStore` and `StageConfigStore` constructors align with how they're called in `main.go`, and confirm the build passes.The `main.go` already has the complete ARCH-v0.2.0-001 implementation from the previous iteration. Let me verify the implementation is fully correct by cross-referencing it against the spec:The implementation is already complete and correct. Here's what `main.go` already does for ARCH-v0.2.0-001:

**Lines 48–50** — Provider stores initialized at boot:
- `provider.NewConnectionStore(cfg.RegistryPath)` — loads `~/.paulette/connections.json` eagerly; a missing file is silently treated as an empty store (graceful default), any other error is logged but doesn't crash
- `provider.NewRegistry(connStore)` — wires the registry to the connection store
- `provider.NewStageConfigStore(cfg.RegistryPath)` — stores the path to `~/.paulette/config.json`; reads lazily on demand and returns an empty `GlobalConfig{}` if the file doesn't exist

**Line 58** — All three are passed to `server.New()` which accepts them as named parameters and threads them into handlers (`ChatHandler`, `MockHandler`, `ConnectionHandler`, `StageConfigHandler`) and the provider registry's `ResolveForStage()` for the three-level fallback.

**Graceful missing-file handling** is implemented in both stores:
- `ConnectionStore.loadFromDisk()` — silently ignores `os.IsNotExist` errors, logs a warning for any other errors, and starts with an empty `[]Connection{}`
- `StageConfigStore.GetGlobalDefaults()` / `readGlobal()` — returns empty `GlobalConfig{StageDefaults: map[...]{}}` when the file is missing

The `main.go` for task 3.4 / ARCH-v0.2.0-001 is fully implemented. No modifications were needed — the previous iteration already delivered this task completely.