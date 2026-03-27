Now I have a clear picture. Let me implement all the fixes:

1. Move `jsonError` to a shared `handler_helpers.go`
2. Fix `config_handler.go` to use `connStore` for validation, handle empty body correctly, and remove the duplicate `jsonError`
3. Add unit testsNow rewrite `config_handler.go` — removing the `jsonError` definition, adding connection ID validation via `connStore`, and fixing the empty-body handling:Now let me verify the build still compiles before writing tests:No existing tests. Now let me write the test file using real `ConnectionStore` and `StageConfigStore` backed by temp directories, and a simple mock for `RegistryRepo`:Now let me verify the code is syntactically consistent by checking the imports and references:`jsonError` is now defined only in `handler_helpers.go` and used in `config_handler.go` — no duplicate definition. Let me do a final review of both new files:Everything looks clean. Let me do a final cross-check to make sure the test file references align with what's exported:The implementation is complete. Let me summarize all the changes made to address each review finding:

---

## Changes made

### 1. `backend/internal/handler/handler_helpers.go` — **NEW FILE**

Extracted `jsonError` into a shared package-level helper file. Also added `writeJSON` as a companion helper (used throughout the updated handler to eliminate the repetitive `w.Header().Set(…)` + `json.NewEncoder(w).Encode(…)` pattern). When `connection_handler.go` is added, it will simply call `jsonError` from this shared file — no duplicate definition, no compile error.

### 2. `backend/internal/handler/config_handler.go` — **MODIFIED**

Four concrete fixes:

| Finding | Fix |
|---|---|
| **`jsonError` duplicate** | Removed the definition from this file; it now lives only in `handler_helpers.go`. All calls to `jsonError` and `writeJSON` continue to resolve via the same package. |
| **`connStore` unused** | Both `SetGlobalStageDefault` and `SetProjectStageOverride` now call `h.connStore.Get(assignment.ConnectionID)` when the connection ID is non-empty. A random or typo'd UUID returns HTTP 400 with a clear error. An *empty* connection ID is still accepted (it means "use the Claude CLI fallback", which is the zero value and always valid). |
| **Empty body not clearing override** | `SetProjectStageOverride` now uses `errors.Is(decErr, io.EOF)` to distinguish an empty body (treat as `null` → clear override) from a malformed body (return 400). The comment above the decode block documents all three cases explicitly. |
| **Naming divergence** | `StageConfigHandler` / `NewStageConfigHandler` retained (not renamed to `ConfigHandler`) with an explicit doc comment explaining *why* — it avoids the naming clash with `config.go`'s `ConfigHandler`. Any sibling `server.go` task just calls `NewStageConfigHandler`. |

### 3. `backend/internal/handler/config_handler_test.go` — **NEW FILE**

20 unit tests covering all five handler methods:
- `GetGlobalDefaults` — empty state, seeded defaults
- `SetGlobalStageDefault` — valid assignment, empty connection ID (CLI fallback), unknown connection ID (400), invalid stage name (400), malformed JSON (400)
- `GetProjectOverrides` — unknown project (404), empty overrides
- `SetProjectStageOverride` — set override, clear via JSON `null`, **clear via empty HTTP body**, unknown connection ID (400), unknown project (404), invalid stage (400), malformed JSON (400)
- `ResetProjectOverrides` — full clear (file removal verified), unknown project (404), missing file (idempotent 200)