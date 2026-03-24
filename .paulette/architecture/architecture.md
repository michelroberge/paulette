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

`backend/internal/server/server.go` — Constructs the chi router with Logger + Recoverer middleware and CORS configuration (allowed origins: localhost:5173, localhost:8080). Mounts all API handler groups under `/api/projects`. Serves the embedded React SPA with HTML5 history fallback for client-side routing.

---

### [ARCH-v0.1.0-004] Configuration
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-009, JRN-v0.1.0-010, JRN-v0.1.0-011

`backend/internal/config/config.go` — Reads `PORT` (default 8080) and `REGISTRY_PATH` (default `~/.paulette`) from environment variables. Exposes a `Config` struct used throughout the server.

---

### [ARCH-v0.1.0-005] Frontend SPA
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-009, JRN-v0.1.0-010, JRN-v0.1.0-011

`frontend/src/` — Single-page React application built with Vite. `main.tsx` bootstraps the React root. `App.tsx` is the top-level stateful controller managing project selection, pipeline state, stage navigation, tab switching, token accounting, autonomous mode, and auto-advance logic. All API communication uses a thin `apiFetch`/`apiStreamUrl` client layer (`frontend/src/api/client.ts`).

---

### [ARCH-v0.1.0-006] Project Handler
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002

`backend/internal/handler/project.go` — Handles full project CRUD. On `Create`, initialises the host directory with `.paulette/` scaffolding, calls `git init`, calls `bd init` (Beads), and registers the project in the global registry. On `Delete`, removes from registry only (host files are preserved). `Patch` currently only toggles `autonomous` mode.

---

### [ARCH-v0.1.0-007] Import Handler & Agent
> Journeys: JRN-v0.1.0-002, JRN-v0.1.0-011

`backend/internal/handler/import.go` + `backend/internal/agent/import.go` — Accepts an existing codebase (local path or git URL). The import agent walks the project directory building a digest (file tree + prioritised file contents, capped at ~100K chars), then calls Claude in four sequential steps to reverse-engineer: Architecture → UX → Architecture cross-reference → Vision. A programmatic Build artifact is also synthesised. Progress is streamed via SSE.

---

### [ARCH-v0.1.0-008] Pipeline Handler & State Machine
> Journeys: JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008, JRN-v0.1.0-010

`backend/internal/handler/pipeline.go` + `backend/internal/pipeline/machine.go` — Controls stage progression. `Approve` advances the project to the next stage, copies the working artifact to `docs/{version}/`, and clears the `.paulette` working copy. `GetPipeline` reconstructs the full stage list with statuses (`locked`, `active`, `approved`). On transition to `complete`, triggers async summary generation.

---

### [ARCH-v0.1.0-009] Chat Handler & Agent
> Journeys: JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-008, JRN-v0.1.0-011

`backend/internal/handler/chat.go` + `backend/internal/agent/agent.go` — For each stage, forwards user messages to the `claude` CLI subprocess using a formatted conversation string (not the API directly). The system prompt is assembled by `agent/prompts.go` and includes injected prior-stage artifacts, framework config, and enhancement context. Responses stream as SSE `chunk`/`artifact`/`done`/`tokens` events. Artifacts are extracted via `<!-- ARTIFACT:START -->…