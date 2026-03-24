# Iteration Summary: paulette v0.1.0

## Product Overview
paulette is a local-first, single-binary AI-guided development pipeline tool that takes a product idea from concept to code through five sequential stages: Vision → UX → Architecture → Build → Complete. Built for product-minded solo builders and AI-first small teams, it eliminates the fragmentation between design thinking and implementation by enforcing structured artifact handoffs between specialized AI agents (Vision Advisor, UX Advisor, Architect, Build Advisor), with human approval gates at each stage and full Git-backed traceability from user journeys to architecture components to executable build tasks.

## Key UX Decisions
- **Stage-gated linear pipeline** with color-coded sidebar (grey=locked, blue=active, green=approved); users can revisit approved stages but cannot skip ahead
- **Tab-based stage views**: every stage has Chat + Artifact tabs; UX adds Mock Preview; Build adds Execute tab with live DAG graph
- **Approve-to-advance pattern**: persistent green Approve button anchored at stage bottom, disabled during streaming; clicking commits artifact to Git and unlocks next stage
- **Always-streaming interface**: all AI output (chat, artifacts, mock HTML, execution logs, summary) streams live via SSE with animated thinking indicator and Stop button
- **Autonomous mode toggle** in project header: replaces Approve bar with informational banner, self-drives entire pipeline with 2s delay between completion and approval, caps enhancement cycles at 10
- **Version History Modal**: full Git log with stage-tag badges, one-click rollback, and remote push/pull sync
- **Stage reset with cascade**: resetting a previously approved stage clears all subsequent stages; resetting active stage clears only current chat/artifact
- **Import flow interstitial**: ImportProgressView full-screen overlay with step-by-step progress before workspace loads; imported stages show review banner and suppress auto-kickoff
- **Bead DAG visualization**: ReactFlow + Dagre layout with amber/blue/green/grey node status colors and animated agents-active indicator
- **Mobile-responsive**: sidebar collapses to horizontal strip below 768px; split layouts stack vertically; 16px font on inputs prevents iOS zoom

## Architecture Summary
**Tech Stack:** Go 1.26 backend (chi router, rs/cors, embed.FS for single-binary deploy), TypeScript/React 19 frontend (Vite 8), react-markdown + remark-gfm, @xyflow/react + @dagrejs/dagre for DAG, SSE for real-time streaming, filesystem JSON/Markdown persistence, Git via os/exec, Claude CLI subprocess (not direct API), Beads `bd` CLI + Dolt database for issue tracking.

**Major Components:**
- `backend/main.go` — server bootstrap, graceful shutdown, cancels active Claude runs on SIGTERM
- `backend/internal/server/server.go` — chi router, CORS (localhost:5173/8080 only), SPA HTML5 history fallback, autopilot wiring
- `backend/internal/handler/project.go` — project CRUD, `bd init`/`git init` on create, autonomous mode toggle triggers orchestrator
- `backend/internal/handler/pipeline.go` + `pipeline/machine.go` — stage state machine, Approve promotes `.paulette/` working artifacts to `docs/{version}/`, git-commits, triggers summary generation on complete
- `backend/internal/handler/chat.go` + `agent/agent.go` — Claude CLI subprocess invocation, SSE streaming of chunk/artifact/done/tokens events, artifact extraction via `<!-- ARTIFACT:START/END -->` delimiters
- `backend/internal/autopilot/orchestrator.go` — one goroutine per autonomous project, handles Vision/Architecture (chat→approve), UX (chat→mock→approve), Build (chat→generate beads→execute→approve), Complete (extract enhancements→iterate); run attachment prevents duplicate agent launches on restart
- `backend/internal/handler/import.go` + `agent/import.go` — codebase digest (file tree + ~100K chars of content), four sequential Claude calls to reverse-engineer Arch→UX→Cross-refs→Vision
- `backend/internal/config/config.go` — PORT (8080) and REGISTRY_PATH (~/.paulette) from env
- `backend/embed.go` — `//go:embed` bundles compiled React SPA into binary

**API Surface:** All routes under `/api/projects`; SSE endpoints for chat streaming, mock generation, bead generation/execution, import progress, and summary generation.

**Data Model:** File-system JSON for project registry and project metadata; Markdown files for artifacts stored in `.paulette/` (working) and `docs/{version}/` (approved); Git repo per project for version history; Beads/Dolt for build task DAG.

**Cross-Stage Traceability IDs:** Journey IDs (`JRN-v*-NNN`), Architecture component IDs (`ARCH-v*-NNN`), linked through artifact text.

## Build Strategy
v0.1.0 is an **initial import baseline** — the entire codebase was reverse-engineered and imported as Milestone 1 with no incremental build tasks. The build plan is intentionally minimal: one milestone, no dependencies, no active task breakdown. The definition of done for this iteration is artifact review and approval. Future iterations should generate a proper decomposed build plan with atomic beads.

## Suggested Enhancements

- **Direct Anthropic API Integration**: Replace `claude` CLI subprocess invocation with direct HTTP calls to the Anthropic API, eliminating the hard dependency on a locally installed and authenticated CLI, enabling cloud deployment, improving error handling, and enabling per-request model/parameter control.

- **Multi-user & Authentication Layer**: Add session-based authentication and per-user project namespacing to support team usage, enabling shared pipelines, reviewer roles, and eventual SaaS deployment without changing the core pipeline model.

- **Model Selection Per Stage/Project**: Expose a project-level (or stage-level) model override UI so users can choose between Sonnet/Opus/Haiku per stage, balancing cost vs. quality for their use case rather than using hardcoded stage-to-model assignments.

- **Bead Execution Retry & Error Recovery**: Add retry logic and an error-recovery UI for blocked or failed beads during execution — currently a blocked bead stalls the DAG with no automated remediation path; a retry + human-intervention escape hatch would significantly improve Build stage completion rates.

- **Relational or Embedded DB Persistence**: Replace flat-file JSON project registry with an embedded database (e.g., SQLite via `modernc.org/sqlite`) to enable querying, filtering, sorting, and indexing of projects and artifacts — a prerequisite for multi-user support and analytics on success metrics.

## Known Limitations
- **Claude CLI dependency**: Must have `claude` binary installed and authenticated on host; no direct API path; limits cloud deployment and concurrency control
- **Beads/Dolt CLI dependency**: `bd` CLI must be installed; tightly coupled to bead execution
- **Single-user, local-first only**: No authentication, multi-tenancy, or user isolation; CORS restricted to localhost
- **File-system-only persistence**: No relational queries; limited scalability; unsuitable for multi-user
- **Sequential stage advancement only**: No concurrent stage work, branching pipelines, or A/B artifact variants
- **Fixed model per stage**: Vision/UX use Sonnet, Architecture/Build use Opus; no per-project override
- **No CI/CD integration, plugin system, or non-Claude AI providers** (all explicitly V1 out-of-scope)
- **Build plan for v0.1.0 is trivial**: Import baseline only; no decomposed task graph was generated for this iteration