- Purpose:
  Add per-project AI mode (Files vs RAG), externalize all prompt templates to editable files in the target repo, and provide a visual workflow editor for viewing/editing prompts per pipeline node.

- Outcome:
  - **AI Mode**: `AIMode` field added to Project model (backend + frontend), with Files as default and RAG gated on server config. UI selector in ProjectHeader.
  - **Prompt Externalization**: New `promptfiles` package with PromptStore that manages ~30 template files in `.paulette/prompts/`. Uses Go `text/template` with named variables. CRUD API endpoints. Auto-scaffolded on project creation. Stage chat prompts (vision, UX, architecture, build) integrated to load from files when available.
  - **Workflow Editor**: Interactive ReactFlow graph showing full pipeline (stages + refinement loop + build execution loop) with clickable nodes. Monaco-based prompt editor slide-over panel with save/reset. Accessible via `/projects/:id/workflow` route.

- Key Decisions:
  - DEC-001: Go text/template for named variables (self-documenting, stdlib)
  - DEC-002: Only externalize system prompts for refinement (user msg stays in Go)
  - DEC-003: Workflow as standalone route (project-level concern)
  - DEC-004: Auto-scaffold prompts on project creation

- Impact:
  - Systems affected: Project model, chat handler, prompt resolution pipeline, frontend routing, project header
  - New files: promptfiles package (2 Go files), handler/prompts.go, 4 frontend components (WorkflowPage, WorkflowGraph, WorkflowNodeTypes, WorkflowPromptEditor), api/prompts.ts
  - Modified files: model/project.go, handler/project.go, handler/chat.go, handler/config.go, handler_helpers.go, server.go, agent/prompts.go, App.tsx, App.css, ProjectHeader.tsx, types/index.ts, api/projects.ts, api/config.ts

- Follow-ups:
  - Wire PromptStore into agent/beads.go (ExecuteBead, ReviewBead, ParseBuildPlan)
  - Create FilePromptSet wrapper for refinement/prompts.go controller integration
  - Wire PromptStore into summary generation (pipeline.go)
  - Add enhancement prompt loading from store (enhancement-*.md.tmpl)
  - Mobile responsive layout for workflow graph
