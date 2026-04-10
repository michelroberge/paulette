# Plan: Per-Connection Prompt Templates

## Goal
Allow per-connection prompt customization so that each LLM connection (e.g., Claude, Ollama 7B) can have its own tailored prompts. Smaller models get simpler prompts, larger models get richer ones.

## Prompt Resolution Order (3-tier)
1. **Project** — `.paulette/prompts/` in target repo (most specific)
2. **Connection** — `~/.paulette/connection-prompts/{connectionId}/` (per-model defaults)
3. **Hardcoded** — Go code defaults (fallback)

## Backend Changes

### PromptStore — add fallback chain
- Add `NewWithFallback(primary, fallback *PromptStore)` or modify `Load` to accept a fallback store
- Simpler: create a `PromptChain` type that wraps multiple stores and tries each in order
- File: `backend/internal/promptfiles/promptfiles.go`

### Connection prompt storage
- Directory: `~/.paulette/connection-prompts/{connectionId}/`
- Created on demand (when user clicks Workflow on a connection and inits prompts)
- Cleaned up on connection delete
- File: modify `backend/internal/provider/connection_store.go` Delete() to also rm the prompts dir

### API endpoints for connection prompts
- `POST   /api/connections/{id}/prompts/init`
- `GET    /api/connections/{id}/prompts`
- `GET    /api/connections/{id}/prompts/{name}`
- `PUT    /api/connections/{id}/prompts/{name}`
- `POST   /api/connections/{id}/prompts/reset/{name}`
- File: `backend/internal/handler/connection_prompts.go` (new)
- Register in `server.go`

### Wire connection store into prompt resolution
- `promptStoreForProject` in `handler_helpers.go` needs the resolved connection ID
- At the call site (chat handler, bead handler), the connection is already resolved
- Pass the connection ID into a new helper: `promptChainForProject(project, connectionID, registryPath)`
- This builds: project store → connection store → nil (hardcoded fallback)

## Frontend Changes

### Workflow button on connection cards
- File: `frontend/src/components/configure/ConnectionsTab.tsx`
- Add a "Workflow" button next to Test/Edit/Delete
- Navigates to `/configure/connections/{id}/workflow`

### Connection workflow route + page
- New route: `/configure/connections/:connId/workflow`
- New component: `ConnectionWorkflowPage.tsx` — loads connection name, renders WorkflowGraph
- Reuses `WorkflowGraph` + `WorkflowPromptEditor` with connection-scoped API calls

### Frontend API for connection prompts
- `frontend/src/api/connectionPrompts.ts` (new)
- Same shape as `api/prompts.ts` but hits `/api/connections/{id}/prompts/*`

### WorkflowPromptEditor — support connection mode
- Currently hardcoded to call `getPrompt(projectId, name)` etc.
- Add a `mode` prop: `{ type: 'project', projectId } | { type: 'connection', connectionId }`
- Switch API calls based on mode

## Related Decisions
- DEC-005: Storage at ~/.paulette/connection-prompts/{connectionId}/
- DEC-006: Resolution order project → connection → hardcoded
