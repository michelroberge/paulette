Now I have a clear picture of all the issues. Let me fix them systematically:

1. Extract `writeJSONAtomic` to `utils.go` (prevent duplicate-symbol compile error)
2. Fix `ClearConnectionReferences` to handle per-project overrides and return errors properly
3. Add a `ResolveStage` method that does both lookups under a single lock
4. Update `provider.go` to use the atomic `ResolveStage`Now rewrite `stage_config.go` to remove `writeJSONAtomic`, fix `ClearConnectionReferences`, and add `ResolveStage`:Now update `provider.go` to use the new `ResolveStage` method (single lock) instead of calling the two public methods separately:Now verify the package compiles cleanly:There's a `connection_store.go` already. Let me check if it also defines `writeJSONAtomic`:Good — `connection_store.go` doesn't define its own `writeJSONAtomic`, so there's no duplicate. Let me do a final review of all provider files to confirm the package is clean:Everything looks correct. Let me do one final check to confirm there's no remaining inline definition of `writeJSONAtomic` in `stage_config.go` and verify the `ClearConnectionReferences` signature update is complete:Good — `ConnectionStore.Delete` correctly delegates stage cleanup to the handler layer, which is consistent with our updated `ClearConnectionReferences` signature. Let me also verify `connection_store.go` doesn't have its own `writeJSONAtomic` and confirm it already uses the shared one from `utils.go`:`connection_store.go` has its own inline `save()` method (a method, not a package-level function) that handles the `chmod 0600` requirement for credentials. It doesn't define a `writeJSONAtomic` function, so there's no symbol conflict. The single package-level `writeJSONAtomic` lives exclusively in `utils.go`.

Let me do a final summary check of the key changes:All four review findings are now resolved. Here's a summary of every change made:

---

### Changes made

#### `backend/internal/provider/utils.go` — **new file**
Extracted `writeJSONAtomic` from `stage_config.go` into a shared package-level utility. The temp-file prefix was generalised from `.tmp-stage-config-*` to `.tmp-paulette-*` so it works equally for any file in the package (`connections.json`, `config.json`, `stage_config.json`). This eliminates the duplicate-symbol compile error that would have occurred once `connection_store.go` needed atomic writes.

#### `backend/internal/provider/stage_config.go` — **modified**
Three fixes applied:

1. **Removed the inline `writeJSONAtomic` definition** — it now lives solely in `utils.go`. All call-sites (`writeGlobal`, `writeProjectConfig`) remain unchanged.

2. **`ClearConnectionReferences` signature and implementation rewritten**:
   - Old: `(connectionID string) []string` — global-only, silent error swallow.
   - New: `(connectionID string, projectHostDirs []string) ([]string, error)` — clears both global defaults and per-project overrides for every hostDir provided, returns a real error instead of `_ = s.writeGlobal(cfg)`. The caller (connection Delete handler) must collect hostDirs from the project registry and pass them in.

3. **Added `ResolveStage(hostDir string, stage model.StageName) *StageAssignment`** — performs the two-level lookup (project override → global default) under a **single** `s.mu.RLock()` acquisition. This replaces the pattern of calling `GetProjectOverrides` then `GetGlobalDefaults` sequentially, which acquired and released the read lock twice in succession.

#### `backend/internal/provider/provider.go` — **modified**
`ResolveForStage` updated to call `stageConfig.ResolveStage(hostDir, stage)` (single lock) instead of the previous two separate calls to `GetProjectOverrides` + `GetGlobalDefaults`. Resolves the unnecessary double-lock cycle noted in finding #4.