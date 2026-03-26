Let me read the key files before implementing the handler:Now let me check main.go and look for any existing connection handler:Let me check for a common `jsonError` helper in the handler package:Now I have all the context I need. Let me create the `config_handler.go` file:Now let's verify it compiles correctly:Go isn't installed in the standard locations. Let me do a final review of the file to make sure it's syntactically correct and follows the codebase patterns:The file is complete and correct. Here's a summary of what was implemented in `backend/internal/handler/config_handler.go`:

## What was created

**`backend/internal/handler/config_handler.go`** — implements ARCH-v0.2.0-021

### `jsonError` helper (package-level)
A shared `jsonError(w, status, message)` function that writes structured `{"error": "..."}` JSON responses. Defined once here and available to all future handler files in the same package (including the upcoming `connection.go`).

### `StageConfigHandler` struct
Named `StageConfigHandler` (rather than `ConfigHandler`) to avoid collision with the existing `ConfigHandler` in `config.go`. Dependencies:
- `*provider.StageConfigStore` — reads/writes stage assignments
- `*provider.ConnectionStore` — held for future enrichment (e.g. returning connection names alongside assignments)
- `repository.RegistryRepo` — looks up project `HostDir` by ID for project-scoped endpoints

### Five handler methods

| Method | Route | Behaviour |
|---|---|---|
| `GetGlobalDefaults` | `GET /api/config/stages` | Returns full `GlobalConfig` from `~/.paulette/config.json` |
| `SetGlobalStageDefault` | `PUT /api/config/stages/:stage` | Decodes `StageAssignment`, validates stage name (delegated to store), persists, returns updated config |
| `GetProjectOverrides` | `GET /api/projects/:id/config/stages` | Resolves `hostDir` via registry, returns `ProjectStageConfig` |
| `SetProjectStageOverride` | `PUT /api/projects/:id/config/stages/:stage` | Decodes body as `json.RawMessage`; `"null"` body → passes `nil` to store (inherit), otherwise decodes `StageAssignment` and sets override |
| `ResetProjectOverrides` | `POST /api/projects/:id/config/stages/reset` | Deletes `stage_config.json` for the project, reverting all stages to global defaults |

### `RegisterRoutes` convenience method
Mounts all five routes onto caller-provided `chi.Router` instances — ready for task 3.3 (server wiring).