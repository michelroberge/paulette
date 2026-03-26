Now let me also read the stage_config.go and connection.go to understand the full context:Now I have a clear picture of all four issues. Let me implement the fixes:Now fix `NewConnectionStore` to log the error instead of silently discarding it:Add input validation to `Create` and `Update`, and update the `Delete` method to actually call `ClearConnectionReferences` and return affected stages:Now replace the `Delete` method with the corrected implementation that actually calls `ClearConnectionReferences` and returns affected stages:Now update the `loadFromDisk` doc comment to make the concurrency constraint explicit and add a `ReloadFromDisk` method that acquires the lock properly:Now add the validation helpers before `loadFromDisk`. I'll insert them after the `snapshot` function:Now let me verify the final state of the file looks correct:Good. Now let me check if anything calls the old `Delete(id string)` signature that needs updating:Good — the only `.Delete(` call patterns in the backend are the HTTP router helper (which takes a path string, not a connection ID) and `registry.Delete(id)` from the project handler (a different `Delete` method on a different type). No existing callers of `ConnectionStore.Delete` yet. Now let me verify the package compiles cleanly:Go isn't available in this environment, so let me do a thorough manual review of the code for correctness:Good — `handler/connection.go` doesn't exist yet, so there are no call sites to the old signature that need updating. The file is clean. Let me do a final structural review of the key changed sections:All four review findings have been addressed. Here's a summary of every change made to `backend/internal/provider/connection_store.go`:

---

### Fixes applied

**1. `Delete` always returned `nil` for affected stage assignments**

Changed the `Delete` signature from:
```go
func (s *ConnectionStore) Delete(id string) ([]string, error)
```
to:
```go
func (s *ConnectionStore) Delete(id string, stageConfig *StageConfigStore, projectHostDirs []string) ([]string, error)
```
The method now calls `stageConfig.ClearConnectionReferences(id, projectHostDirs)` (if stageConfig is non-nil) **before** removing the connection from disk, and returns the resulting `[]string` of affected stage names. The two stores use independent mutexes so there's no deadlock. The `nil` fallback (when `stageConfig == nil`) is preserved for test scenarios. Since `handler/connection.go` doesn't exist yet, there are no call-site breaks.

**2. Corrupt file silently dropped all connections**

`NewConnectionStore` now inspects the error from `loadFromDisk()`:
```go
if err := s.loadFromDisk(); err != nil {
    log.Printf("[paulette] WARNING: could not load %s: %v — starting with empty connection store", s.path, err)
}
```
A missing file (first run) still returns `nil` and is silent; a corrupted file logs a `WARNING` with the path and underlying error so operators can diagnose data loss.

**3. No input validation on `Create`/`Update`**

Added `validateConnectionInput()` — called at the top of both `Create` and `Update` — which rejects:
- Empty/whitespace-only `Name`
- Unknown `ProviderType` (validated against `validProviderTypes` map mirroring all 7 constants in `connection.go`)
- Empty `DefaultModel` for non-`claude_cli` providers (`claude_cli` uses a hardcoded model map so blank is acceptable)

**4. `loadFromDisk` concurrency documentation gap**

Added a `ReloadFromDisk()` public method that properly acquires the write lock before delegating to `loadFromDisk()`, making the safe reload pattern explicit and ready for any future "hot-reload on external edit" feature. The doc comment on `loadFromDisk` now explicitly states the "must NOT be called while lock is held" constraint and points to `ReloadFromDisk` as the safe alternative.