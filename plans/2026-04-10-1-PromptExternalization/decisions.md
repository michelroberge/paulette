## Decision: Template Engine for Externalized Prompts
ID: DEC-001

- Date: 2026-04-10
- Status: Accepted

- Context:
  Prompt templates currently use fmt.Sprintf with positional %s placeholders. When externalized to user-editable files, positional args are fragile and opaque.

- Options Considered:
  - Option A: Keep fmt.Sprintf with positional %s
  - Option B: Go text/template with named {{.VarName}} syntax
  - Option C: Custom {{variable}} parser

- Decision:
  Go text/template with named variables

- Rationale:
  Self-documenting (users see variable names), stdlib (no deps), powerful enough for conditionals if needed later

- Consequences:
  - Positive:
    - User-friendly template editing
    - No new dependencies
  - Negative:
    - Must convert all existing fmt.Sprintf templates to text/template syntax
    - Small template injection surface (mitigated by restricted FuncMap)

- Impact:
  - Affects: all prompt files, agent/prompts.go, prompts/stage.go, prompts/bead.go, refinement/prompts.go
  - Related Plans: 2026-04-10-1-PromptExternalization

---

## Decision: Refinement Prompt Externalization Scope
ID: DEC-002

- Date: 2026-04-10
- Status: Accepted

- Context:
  Refinement prompts are methods on VisionPromptSet that return (system, user) pairs. The user message is dynamically assembled from LoopState data.

- Options Considered:
  - Option A: Externalize both system and user messages
  - Option B: Externalize only system prompts, keep user message assembly in Go

- Decision:
  Externalize only system prompts

- Rationale:
  User messages require complex Go data (LoopState fields, loops, conditionals). Exposing this in templates adds complexity with little user benefit. System prompts are where the AI behavior is tuned.

- Consequences:
  - Positive:
    - Simpler template files
    - Users can still tune AI behavior via system prompts
  - Negative:
    - User message format is not customizable

- Impact:
  - Affects: refinement/prompts.go (FilePromptSet wrapper)
  - Related Plans: 2026-04-10-1-PromptExternalization

---

## Decision: Workflow Graph as Separate View
ID: DEC-003

- Date: 2026-04-10
- Status: Accepted

- Context:
  Need a visual workflow editor. Could be a tab within StageView or a standalone route.

- Options Considered:
  - Option A: New route /projects/:id/workflow with button in ProjectHeader
  - Option B: Always-visible tab within StageView

- Decision:
  New route with button in ProjectHeader (Option A)

- Rationale:
  The workflow is a project-level concern (spans all stages), not stage-specific. A dedicated route gives it proper screen real estate and avoids cluttering stage tabs.

- Consequences:
  - Positive:
    - Clean separation of concerns
    - Full screen for graph layout
  - Negative:
    - Navigation away from current stage view

- Impact:
  - Affects: App.tsx routing, ProjectHeader.tsx
  - Related Plans: 2026-04-10-1-PromptExternalization

---

## Decision: Auto-scaffold Prompts on Project Creation
ID: DEC-004

- Date: 2026-04-10
- Status: Accepted

- Context:
  When aiMode=files (the default), the .paulette/prompts/ directory needs to exist in the target repo.

- Options Considered:
  - Option A: Auto-create on project creation
  - Option B: Manual init via API/UI button
  - Option C: Lazy init on first prompt access

- Decision:
  Auto-create on project creation (Option A)

- Rationale:
  Default mode is "files", so prompts dir should exist immediately. Users expect to see and edit prompts right away.

- Consequences:
  - Positive:
    - Zero friction for default mode
    - Prompts available immediately
  - Negative:
    - Adds ~30 files to target repo on project creation

- Impact:
  - Affects: handler/project.go Create()
  - Related Plans: 2026-04-10-1-PromptExternalization
