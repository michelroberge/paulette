It looks like most of the handler infrastructure already exists. Let me read the actual file contents to understand what's already there and what needs to be added or fixed.The key files are all implemented. Let me build the project to verify everything compiles correctly and identify any issues.The backend builds cleanly. Let me run the tests to confirm everything passes:Let me check if there's a duplicate `connection_handler.go` file mentioned in the directory listing:Everything looks complete. Let me run the tests fresh (not cached) to get a clean confirmation:All tests pass. The backend for Milestone 3 (Backend API Handlers & Integration) is fully implemented. Here's a summary of what exists and was verified:

## Milestone 3 — Verified Complete ✅

All 6 tasks are fully implemented in the existing codebase:

### 3.1 — `handler/connection.go` (475 lines)
Full `ConnectionHandler` with all 9 routes via `RegisterRoutes()`:
- **List** `GET /api/connections` — returns all connections with credentials redacted (`HasCredentials` bool, never raw `APIKey`)
- **Create** `POST /api/connections` — assigns UUID, timestamps, validates, returns 201
- **Get** `GET /api/connections/:id` — single connection, 404 if missing
- **Update** `PUT /api/connections/:id` — preserves API key when client omits it
- **Delete** `DELETE /api/connections/:id` — cascades stage config cleanup, returns `affectedStages[]`
- **Test** `POST /api/connections/:id/test` — probes saved connection + concurrent model discovery
- **TestNew** `POST /api/connections/test` — probes unsaved config inline
- **ListModels** `GET /api/connections/:id/models` — 422 for providers without model list
- **ListModelsNew** `POST /api/connections/models` — unsaved connection model fetch

### 3.2 — `handler/config_handler.go` (259 lines)
Full `StageConfigHandler` with 5 endpoints:
- `GET /api/config/stages` → global defaults
- `PUT /api/config/stages/:stage` → set global default (validates connection exists)
- `GET /api/projects/:id/config/stages` → per-project overrides
- `PUT /api/projects/:id/config/stages/:stage` → set/clear override (null body = inherit)
- `POST /api/projects/:id/config/stages/reset` → clear all project overrides

### 3.3 — `server/server.go`
All routes mounted: `/api/connections/*` (via `connH.RegisterRoutes`) and `/api/config/stages` + per-project config routes.

### 3.4 — `main.go`
`ConnectionStore`, `Registry`, and `StageConfigStore` initialized at startup; missing files treated as empty (zero-config fallback to Claude CLI).

### 3.5 — `handler/chat.go`
`StartChatRun` and `resumeChatRun` use `registry.ResolveForStage()` → `provider.Chat()` instead of calling `agent.Chat()` directly. Connection failures emit structured `StreamEvent{Type: "error"}` with `isConnectionError: true`, `connectionId`, `connectionName`, and `reason` for frontend error banner rendering.

### 3.6 — `handler/mock.go`
`StartMockRun` uses the same provider resolution pattern; HTML envelope retry logic (up to `MaxMockRetries` attempts) works with any configured provider.