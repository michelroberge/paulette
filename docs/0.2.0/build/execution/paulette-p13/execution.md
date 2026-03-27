Now I have everything I need. Let me create the `api/stageConfig.ts` file:Now let me verify the file looks correct and compiles cleanly:No TypeScript errors. The file `frontend/src/api/stageConfig.ts` has been created with all 5 required functions:

| Function | Method | Endpoint | Description |
|---|---|---|---|
| `getGlobalDefaults()` | `GET` | `/api/config/stages` | Fetches global stage defaults from `~/.paulette/config.json` |
| `setGlobalStageDefault(stage, assignment)` | `PUT` | `/api/config/stages/:stage` | Sets global default for one stage; auto-saves |
| `getProjectOverrides(projectId)` | `GET` | `/api/projects/:id/config/stages` | Fetches per-project overrides |
| `setProjectStageOverride(projectId, stage, assignment)` | `PUT` | `/api/projects/:id/config/stages/:stage` | Sets or clears (via `null`) a project-level override |
| `resetProjectOverrides(projectId)` | `POST` | `/api/projects/:id/config/stages/reset` | Clears all project overrides, reverting all stages to inherit |

Key design decisions:
- **`setProjectStageOverride` accepts `StageAssignment | null`** — sending `null` serialises to a JSON `null` body, signalling the backend to clear that stage's override and revert to the global default (the "Inherit" toggle OFF → ON flow from IACT-012)
- All functions delegate to the existing `apiFetch` helper — no inline `fetch` calls
- Types imported from `types/provider.ts` (which already existed) and `types/index.ts` for `StageName`
- No error catching inside the module — errors propagate to callers as per project convention