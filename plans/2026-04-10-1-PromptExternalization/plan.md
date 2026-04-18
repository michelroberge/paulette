# Plan: AI Mode + Externalized Prompts + Visual Workflow Editor

## Goal
Allow per-project prompt customization by externalizing all templates to `.paulette/prompts/` in the target repo. Provide a visual workflow editor where users can click any pipeline node to view/edit its prompt. Formalize RAG vs Files as a per-project AI mode setting.

## Phase 1: AI Mode Per Project
- Add `AIMode` field to Project model (backend + frontend)
- Extend PATCH handler to accept aiMode, validate RAG availability
- Add UI selector in project settings

## Phase 2: Externalized Prompt Templates
- New `promptfiles` package with PromptStore (Init/Load/Save/Render/List)
- Extract ~30 prompts into defaults.go, convert fmt.Sprintf to text/template
- API endpoints for prompt CRUD
- Modify all prompt consumers to accept optional PromptStore
- Auto-init on project creation
- Frontend API module

## Phase 3: Visual Workflow Editor
- WorkflowGraph component using @xyflow/react + dagre
- Custom node types (StageNode, RefinementNode, BeadPhaseNode)
- WorkflowPromptEditor slide-over with Monaco editor
- New route /projects/:id/workflow

## Related Decisions
- DEC-001: Go text/template for named variables
- DEC-002: Externalize only system prompts for refinement
- DEC-003: Workflow as standalone route
- DEC-004: Auto-scaffold prompts on project creation
