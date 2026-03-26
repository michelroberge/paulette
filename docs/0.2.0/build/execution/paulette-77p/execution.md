Now I have a clear picture of the patterns. Let me check one more existing API file and then create the connections module:Now I have everything I need. The `connections.ts` file doesn't exist yet — I'll create it as a new file following the established patterns from `stageConfig.ts` and `projects.ts`:Now let me verify the file looks correct and check if there's an `index.ts` that re-exports API modules (so I know whether to add an export there):Good — there's no barrel index to update. Let me do a final check to confirm the file compiles cleanly by verifying the TypeScript types referenced all exist in `types/provider.ts`:No TypeScript errors. The implementation is complete. Here's a summary of what was created:

## `frontend/src/api/connections.ts` — Created

Implements all 8 required functions for connection management, following the exact same patterns as the existing `stageConfig.ts` and `projects.ts` API modules:

| Function | HTTP | Endpoint | Description |
|---|---|---|---|
| `listConnections()` | `GET` | `/connections` | List all connections (credentials redacted) |
| `createConnection(input)` | `POST` | `/connections` | Create and persist a new connection |
| `getConnection(id)` | `GET` | `/connections/:id` | Fetch a single connection by UUID |
| `updateConnection(id, input)` | `PUT` | `/connections/:id` | Update an existing connection |
| `deleteConnection(id)` | `DELETE` | `/connections/:id` | Delete and return affected stage names |
| `testConnection(id)` | `POST` | `/connections/:id/test` | Test a saved connection (+ model discovery) |
| `testNewConnection(input)` | `POST` | `/connections/test` | Test an unsaved config before saving |
| `listModels(id)` | `GET` | `/connections/:id/models` | Fetch model list from a saved connection |
| `listModelsForNew(input)` | `POST` | `/connections/models` | Fetch model list from an unsaved config |

Key design choices:
- All types imported from `../types/provider` (`Connection`, `ConnectionInput`, `ModelInfo`, `TestResult`, `DeleteResult`) — no duplication
- Uses the existing `apiFetch` helper — no inline `fetch` calls
- `testConnection` and `testNewConnection` both perform simultaneous connectivity + model discovery probing (IACT-008) — the backend handles the dual probe; the client just surfaces the unified `TestResult`
- `deleteConnection` returns `DeleteResult` with `affectedStages[]` so the UI can warn the user which stage assignments were cleared (JRN-v0.2.0-004)