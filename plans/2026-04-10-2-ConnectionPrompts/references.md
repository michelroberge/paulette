- Relevant Decisions:
  - [PromptExternalization] DEC-001: Go text/template for named variables
    - Impact: Connection prompts will use the same template format
  - [PromptExternalization] DEC-004: Auto-scaffold prompts on project creation
    - Impact: Connection prompts should also be scaffoldable

- Reusable Patterns:
  - PromptStore (backend/internal/promptfiles/promptfiles.go) — Init/Load/Save/Render/List/Reset
    - Can be reused with a different base directory for connections
  - WorkflowGraph + WorkflowPromptEditor (frontend/src/components/pipeline/)
    - Can be reused with a connectionId prop instead of projectId
  - Prompt CRUD API pattern (handler/prompts.go)
    - Same endpoint structure, different base path
  - Connection cards in ConnectionsTab.tsx have inline action buttons (Test, Edit, Delete)
    - Add "Workflow" button in the same pattern

- Risks / Conflicts:
  - Prompt resolution order needs 3 levels now: project → connection → hardcoded default
  - Connection prompts have no "project" context, so workflow graph shows all nodes but in a generic context
  - Storage: connections are in ~/.paulette/connections.json (flat file), prompts need a directory per connection
