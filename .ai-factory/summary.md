# Iteration Summary: Claudette v0.1.0

## Product Overview
Claudette is a local-first, single-binary AI-guided development pipeline tool that transforms a product idea into implemented code through five sequential stages: Vision → UX → Architecture → Build → Complete. Targeting solo founders, product-minded builders, AI-first small teams, and tech leads, it solves the fragmentation and context-loss problem between ideation and implementation by enforcing a stage-gated workflow with mandatory human approval checkpoints, full cross-stage traceability, and Git-backed versioning. Each stage is driven by a specialised Claude AI agent with real-time streaming output, and the Build stage executes as a dependency-aware DAG of atomic tasks ("beads") with optional parallelism and autonomous end-to-end mode.

## Key UX Decisions
- **Stage-gated linear pipeline** with a left sidebar showing colour-coded status pips (grey=locked, blue=active, green=approved); users can revisit approved stages but cannot skip forward
- **Tab-based stage views**: every stage has Chat + Artifact tabs; UX adds Mock Preview; Build adds Execute tab
- **Approve-to-advance**: a persistent green Approve button anchors each active stage, disabled during streaming
- **Always-streaming**: all AI output (chat, artifacts, mock HTML, bead logs, summary) delivered via SSE with animated thinking indicators and a Stop button during generation
- **Autonomous mode toggle** in the project header auto-approves stages with a 2s delay, auto-generates mocks, auto-executes beads, and runs up to 10 enhancement cycles unattended
- **Bead DAG visualisation** using ReactFlow + Dagre with colour-coded node states (amber=ready, blue=in-progress, green=done, grey=blocked) and a live progress bar
- **Version History Modal** accessible from the header: full git log with stage-tag badges, one-click rollback with cascade clearing of dependent stages
- **Imported project banner** suppresses auto-kickoff and signals artifacts are auto-generated and awaiting review
- **Mobile-responsive**: sidebar collapses to horizontal strip below 768px; split layouts stack vertically; iOS zoom prevented

## Architecture Summary
- **Backend**: Go 1.26 with chi v5 router; file-system persistence (JSON + Markdown under `~/.claudette`); Git operations via `os/exec`; `claude` CLI invoked as subprocess (not direct API); `bd` CLI + Dolt for bead tracking
- **Frontend**: React 19 + TypeScript 5.9 built with Vite 8; `react-markdown` + `remark-gfm` for artifact rendering; `@xyflow/react` + `@dagrejs/dagre` for DAG; thin `apiFetch`/`apiStreamUrl` API client
- **Deployment**: single Go binary with frontend SPA embedded via `embed.FS`; no external runtime beyond `claude` and `bd` CLIs
- **Real-time**: Server-Sent Events for all streaming; events typed as `chunk` / `artifact` / `done` / `tokens`
- **Key API surface**: `/api/projects` CRUD; pipeline approve/reset/state; chat per stage; import SSE endpoint; bead generate/execute; git history/rollback/remote sync; summary generation
- **Data model**: projects registered in a global registry file; per-project state in `.ai-factory/` (working artifacts, chat histories, pipeline state JSON); approved artifacts promoted to `docs/{version}/`
- **Traceability IDs**: `JRN-v*-NNN` (user journeys) → `ARCH-v*-NNN` (architecture components) → build tasks

## Build Strategy
v0.1.0 is an **initial import baseline** — the codebase was reverse-engineered via Claudette's own import agent and pre-populated with artifacts awaiting human review. There is a single milestone (Import existing codebase) with no task decomposition or bead execution performed. The build plan explicitly defers all enhancement work to subsequent iteration cycles, with the definition of done being artifact approval by the user.

## Suggested Enhancements

- **Direct Anthropic API integration** — Replace the `claude` CLI subprocess with direct HTTP calls to the Anthropic API, enabling cloud deployment, multi-user access, proper concurrency control, and elimination of the CLI install prerequisite
- **Multi-user support with authentication** — Add user accounts, session management, and tenant-isolated registry paths to allow teams to run a shared Claudette instance rather than one per developer machine
- **Model selection per stage** — Expose a per-project (or per-stage) model override UI, allowing users to trade cost vs. quality (e.g., Haiku for Vision, Opus for Architecture) rather than fixed Sonnet/Opus defaults
- **Relational persistence layer** — Introduce an embedded SQLite (or similar) store for project metadata, chat histories, and token accounting to enable search, filtering, and multi-project analytics without sacrificing local-first operation
- **CI/CD pipeline integration hooks** — Add webhook or GitHub Actions trigger support so that bead execution and enhancement cycles can be initiated from external events (e.g., a PR merge auto-triggers a patch enhancement cycle)

## Known Limitations
- `claude` CLI must be installed and authenticated on the host machine; no direct API fallback
- `bd` CLI and Dolt database must be present for bead execution; tightly coupled
- File-system-only persistence limits query capability and concurrent writes
- Single-user, local-first only; no authentication, multi-tenancy, or access control
- CORS restricted to `localhost:5173` and `localhost:8080`; not suitable for networked/cloud deployment as-is
- Model selection is hardcoded per stage (Vision/UX → Sonnet; Architecture/Build → Opus)
- Sequential stage progression only; no branching pipelines or parallel stage tracks
- v0.1.0 build plan contains no executed beads — the codebase was imported, not built through the pipeline itself