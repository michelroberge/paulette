Now let me check if there's an existing connection_store.go to understand the pattern:Now I have all the context I need. The `registryPath` is `~/.paulette/`. Let me implement the `stage_config.go` file:Now let's verify the file compiles correctly with the rest of the package:Go isn't installed in the shell environment, but I can verify the implementation is correct by cross-checking the types against what `provider.go` already references. Let me do a quick review:All types and method signatures used in `provider.go` correctly match what I implemented in `stage_config.go`. The implementation is complete.

Here's a summary of what was created in `backend/internal/provider/stage_config.go`:

---

### `provider/stage_config.go` — What was implemented

**Data types:**
- `StageAssignment` — `{ConnectionID, Model}` pair, JSON-tagged for persistence
- `GlobalConfig` — wraps `map[StageName]StageAssignment` under `"stageDefaults"`, persisted at `~/.paulette/config.json`
- `ProjectStageConfig` — wraps `map[StageName]*StageAssignment` under `"overrides"` (nil pointer = inherit), persisted at `<hostDir>/.paulette/stage_config.json`

**`StageConfigStore` methods:**
| Method | Behaviour |
|---|---|
| `GetGlobalDefaults()` | Reads `~/.paulette/config.json`; returns empty `GlobalConfig` if missing/corrupt |
| `SetGlobalStageDefault(stage, assignment)` | Validates stage, merges into global config, atomically writes file |
| `GetProjectOverrides(hostDir)` | Reads `<hostDir>/.paulette/stage_config.json`; returns empty config if missing |
| `SetProjectStageOverride(hostDir, stage, *assignment)` | nil = inherit (key stays, value null); non-nil = override; atomically writes file; creates parent dirs |
| `ResetProjectOverrides(hostDir)` | Removes `stage_config.json` entirely — all stages revert to global |
| `ClearConnectionReferences(connectionID)` | Removes global-default entries referencing the deleted connection; returns affected stage names |

**Atomic write pattern:** All writes go through `writeJSONAtomic` — writes to a temp file in the same directory, `chmod`s it, then `os.Rename`s it to the final path, ensuring no partial writes survive a crash.

**Stage validation:** `validateStage()` rejects any name not in `{vision, ux, architecture, build, complete}` with a clear error message.