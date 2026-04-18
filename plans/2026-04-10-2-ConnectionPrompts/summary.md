- Purpose:
  Allow per-connection prompt customization so each LLM connection can have tailored prompts. Smaller models get simpler prompts, larger models get richer ones.

- Outcome:
  - **PromptStore fallback chain**: `WithFallback()` method lets stores compose a resolution chain (project → connection → hardcoded defaults). Also added `PromptChain`, `LoadCustomOnly`, `NewForConnection` utilities.
  - **Connection prompt storage**: `~/.paulette/connection-prompts/{connectionId}/` — same template format as project prompts. Init on first workflow visit, cleaned up on connection delete.
  - **Connection prompt API**: 5 CRUD endpoints under `/api/connections/{id}/prompts/`
  - **3-tier resolution in chat handler**: `promptStoreWithConnection()` builds project store with connection fallback. Wired into both chat handler code paths.
  - **WorkflowPromptEditor refactored**: Uses `PromptTarget` discriminated union (`{type:'project',...}` or `{type:'connection',...}`) to switch between project and connection API calls.
  - **ConnectionWorkflowPage**: Dedicated page at `/configure/connections/:connId/workflow`, auto-inits prompt directory on visit.
  - **Workflow button on connection cards**: Purple "Workflow" action button in ConnectionsTab, navigates to the connection workflow page.

- Key Decisions:
  - DEC-005: Store at ~/.paulette/connection-prompts/{connectionId}/ (consistent with project pattern)
  - DEC-006: Resolution order project → connection → hardcoded (connection = model-class defaults, project = per-project tuning)

- Impact:
  - Systems affected: promptfiles package, chat handler, connection handler, ConnectionsTab, WorkflowGraph, WorkflowPromptEditor
  - New files: handler/connection_prompts.go, api/connectionPrompts.ts, ConnectionWorkflowPage.tsx
  - Modified files: promptfiles.go, handler_helpers.go, chat.go, connection.go, server.go, WorkflowGraph.tsx, WorkflowPromptEditor.tsx, WorkflowPage.tsx, ConnectionsTab.tsx, App.tsx

- Follow-ups:
  - Wire connection-aware prompt resolution into bead handler and refinement handler (currently only chat handler uses 3-tier chain)
  - Add visual diff indicator in WorkflowPromptEditor showing when a prompt differs from default
