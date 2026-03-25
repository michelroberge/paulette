# System Architecture: paulette

## Tech Stack

| Layer | Technology |
|---|---|
| **Backend Language** | Go 1.26 |
| **HTTP Router** | go-chi/chi v5 |
| **CORS Middleware** | rs/cors |
| **UUID Generation** | google/uuid |
| **Frontend Language** | TypeScript 5.9 |
| **Frontend Framework** | React 19 (Vite 8) |
| **Markdown Rendering** | react-markdown + remark-gfm |
| **DAG Visualization** | @xyflow/react + @dagrejs/dagre |
| **AI Engine** | Anthropic Claude CLI (`claude` subprocess) |
| **Issue Tracking** | Beads (`bd` CLI) + Dolt database |
| **Persistence** | File system (JSON + Markdown) |
| **Real-time Streaming** | Server-Sent Events (SSE) |
| **Build/Deploy** | Single binary (Go `embed.FS` for frontend) |
| **Version Control Integration** | Git (via `os/exec`) |

---

## System Components

### [ARCH-v0.1.0-001] Entry Point & Server Bootstrap
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-009, JRN-v0.1.0-010, JRN-v0.1.0-011

`backend/main.go` — Initializes all repositories, embeds the frontend static files via `embed.FS`, creates the HTTP server, registers a graceful shutdown handler (SIGINT/SIGTERM), and cancels all active Claude agent runs on shutdown. Listens on `0.0.0.0:{PORT}` (default 8080).

---

### [ARCH-v0.1.0-002] Frontend Static Embedding
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-009, JRN-v0.1.0-010, JRN-v0.1.0-011

`backend/embed.go` — Declares the `//go:embed` directive that bundles the compiled React SPA into the Go binary at build time, exposing it as `staticFiles embed.FS`. This enables single-binary deployment with no separate static file server.

---

### [ARCH-v0.1.0-003] HTTP Server & Router
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-009, JRN-v0.1.0-010, JRN-v0.1.0-011

`backend/internal/server/server.go` — Constructs the chi router with Logger + Recoverer middleware and CORS configuration (allowed origins: localhost:5173, localhost:8080). Mounts all API handler groups under `/api/projects`. Serves the embedded React SPA with HTML5 history fallback for client-side routing. On first `Router()` call, instantiates the `autopilot.Orchestrator` and wires it into `ProjectHandler` and `PipelineHandler` via `SetOrchestrator`.

---

### [ARCH-v0.1.0-004] Configuration
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-009, JRN-v0.1.0-010, JRN-v0.1.0-011

`backend/internal/config/config.go` — Reads `PORT` (default 8080) and `REGISTRY_PATH` (default `~/.paulette`) from environment variables. Exposes a `Config` struct used throughout the server.

---

### [ARCH-v0.1.0-005] Frontend SPA
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-009, JRN-v0.1.0-010, JRN-v0.1.0-011

`frontend/src/` — Single-page React application built with Vite. `main.tsx` bootstraps the React root. `App.tsx` is the top-level stateful controller managing project selection, pipeline state, stage navigation, tab switching, and token accounting. All API communication uses a thin `apiFetch`/`apiStreamUrl` client layer (`frontend/src/api/client.ts`). In autonomous mode the frontend is an observer only — it reflects state driven by the backend orchestrator rather than driving stage progression or approvals itself.

---

### [ARCH-v0.1.0-006] Project Handler
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002

`backend/internal/handler/project.go` — Handles full project CRUD. On `Create`, initialises the host directory with `.paulette/` scaffolding, calls `git init`, calls `bd init` (Beads), and registers the project in the global registry. On `Delete`, removes from registry only (host files are preserved). `Patch` toggles `autonomous` mode and immediately calls `orchestrator.Ensure()` or `orchestrator.Cancel()` to start or stop the background goroutine. `Get` self-heals: if the returned project is autonomous and the pipeline is not complete, it calls `orchestrator.Ensure()` to restart any goroutine that may have been lost across a server restart.

---

### [ARCH-v0.1.0-007] Import Handler & Agent
> Journeys: JRN-v0.1.0-002, JRN-v0.1.0-011

`backend/internal/handler/import.go` + `backend/internal/agent/import.go` — Accepts an existing codebase (local path or git URL). The import agent walks the project directory building a digest (file tree + prioritised file contents, capped at ~100K chars), then calls Claude in four sequential steps to reverse-engineer: Architecture → UX → Architecture cross-reference → Vision. A programmatic Build artifact is also synthesised. Progress is streamed via SSE.

---

### [ARCH-v0.1.0-008] Pipeline Handler & State Machine
> Journeys: JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-010

`backend/internal/handler/pipeline.go` + `backend/internal/pipeline/machine.go` — Controls stage progression. `Approve` (HTTP handler) and `ApproveInternal` (programmatic, no HTTP plumbing) both advance the project to the next stage: promote the working artifact and UX mock.html from `.paulette/` to `docs/{version}/`, git-commit the promotion, and clear the working copy. `ApproveInternal` exists specifically so the Autopilot Orchestrator can approve stages without going through HTTP. `GetPipeline` reconstructs the full stage list with statuses (`locked`, `active`, `approved`). On transition to `complete`, triggers async summary generation.

---

### [ARCH-v0.1.0-009] Chat Handler & Agent
> Journeys: JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-008, JRN-v0.1.0-011

`backend/internal/handler/chat.go` + `backend/internal/agent/agent.go` — For each stage, forwards user messages to the `claude` CLI subprocess using a formatted conversation string (not the API directly). The system prompt is assembled by `agent/prompts.go` and includes injected prior-stage artifacts, framework config, and enhancement context. Responses stream as SSE `chunk`/`artifact`/`done`/`tokens` events. Artifacts are extracted via `<!-- ARTIFACT:START -->…<!-- ARTIFACT:END -->` delimiters and persisted to `.paulette/`. `StartChatRun` exposes a programmatic entry point used by the Autopilot Orchestrator to initiate stage conversations without an incoming HTTP request.

---

### [ARCH-v0.1.0-012] Autopilot Orchestrator
> Journeys: JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-010

`backend/internal/autopilot/orchestrator.go` — Manages one long-running goroutine per autonomous project that drives the entire pipeline without human intervention. Created once at server startup and wired into `ProjectHandler` and `PipelineHandler` via an `OrchestratorI` interface.

**Lifecycle.** `Ensure(projectID)` starts the goroutine if not already running (idempotent). `Cancel(projectID)` stops it. `StartAll()` is called once at server startup to resume goroutines for any projects that were autonomous when the server last shut down. `CancelAll()` is called on graceful shutdown.

**Per-project loop.** `runProject` loops over pipeline stages, dispatching to a per-stage handler:

- **Vision / Architecture** (`handleSimpleStage`): if no artifact exists, sends a kickoff message via `ChatHandler.StartChatRun`, waits for the run to finish, then calls `PipelineHandler.ApproveInternal` after a 2-second pause.
- **UX** (`handleUXStage`): same chat flow, then also generates the mock.html via `MockHandler.StartMockRun` if absent, then approves.
- **Build** (`handleBuildStage`): same chat flow, then generates the Beads issue graph via `BeadHandler.StartGenerateRun` if absent, executes the issues via `StartExecuteRun`, then approves.
- **Complete** (`handleCompleteStage`): waits for the async summary to be ready, reads the `## Suggested Enhancements` section, and — if suggestions exist and the iteration cap (10) has not been reached — calls `EnhanceHandler.EnhanceInternal` to kick off another iteration.

**Run attachment.** Before starting any agent run, the orchestrator checks whether a matching run is already active in the `stream.Manager` (by project/stage/type key) and attaches to it rather than duplicating it. This makes the orchestrator safe to restart mid-stage.