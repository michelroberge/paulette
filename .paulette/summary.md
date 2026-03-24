# Iteration Summary: Paulette v0.1.0

## Product Overview

Paulette is a local-first, single-binary AI development pipeline tool that guides product builders from raw idea to working code through a structured five-stage workflow: Vision → UX → Architecture → Build → Complete. Targeting solo founders, product-minded developers, and AI-first small teams, it solves the fragmentation problem in software development by enforcing traceability between every stage — ensuring what gets built reflects what was intended. Each stage is driven by a specialized AI agent (backed by the Claude CLI subprocess), with human approval gates at every transition and an opt-in autonomous mode for end-to-end hands-free execution.

## Key UX Decisions

- **Stage-gated linear pipeline with sidebar navigation**: Five stages rendered in a left sidebar with colour-coded status pips (grey=locked, blue=active, green=approved); users cannot skip ahead but can revisit approved stages
- **Tab-based stage views**: Each stage exposes Chat + Artifact tabs, with stage-specific extras — Mock Preview (UX), Execute (Build); tab state is preserved on switch
- **Approve-to-advance pattern**: A persistent green Approve button anchors the bottom of each active stage; disabled during streaming; replaced by an informational banner in autonomous mode
- **Always-streaming SSE interface**: All AI output (chat, artifacts, mock HTML, bead execution logs, summary) streams in real time; a Stop button is available during any active stream
- **Bead DAG visualisation**: ReactFlow + Dagre renders a live dependency graph of atomic build tasks with colour-coded status (amber=ready, blue=in-progress, green=done, grey=blocked)
- **Version History Modal**: Full git log with stage-tag badges, one-click rollback, and a collapsible Remote section for push/pull sync
- **Mobile-responsive layout**: Sidebar collapses to a horizontal strip below 768px; split layouts stack vertically; iOS zoom prevention on inputs
- **Imported project indicators**: Banner per stage noting auto-generated content; auto-kickoff suppressed for imported projects

## Architecture Summary

**Tech Stack:** Go 1.26 backend (chi router, rs/cors), TypeScript 5.9 / React 19 / Vite 8 frontend, embedded via `embed.FS` for single-binary deployment. AI via `claude` CLI subprocess (not direct API). Build task tracking via `bd` (Beads) CLI + Dolt database. Persistence is file-system only (JSON + Markdown under `~/.paulette`). Real-time communication over SSE. Git integration via `os/exec`.

**Major Components:**
| Component | Role |
|---|---|
| `server.go` | Chi router, CORS, SPA fallback, autopilot wiring |
| `pipeline/machine.go` | Stage state machine; approve/promote/commit lifecycle |
| `agent/agent.go` + `chat.go` | Claude CLI subprocess wrapper; SSE streaming of chunk/artifact/done/tokens events |
| `autopilot/orchestrator.go` | Per-project goroutine driving autonomous pipeline execution; attaches to existing runs to survive restarts |
| `handler/import.go` + `agent/import.go` | Four-step reverse-engineering of existing codebases |
| `handler/bead.go` | Parses build plan into DAG, delegates execution to `bd` CLI |
| Frontend `App.tsx` | Top-level state controller; token accounting; stage/tab navigation |

**API Surface:** All routes under `/api/projects/*` — project CRUD, pipeline approve/get, chat SSE, mock generation SSE, bead generate/execute SSE, import SSE, enhance, summary, git history/reset/remote, token accounting.

**Data Model Highlights:** Projects stored as JSON in registry; artifacts as Markdown files under `.paulette/` (working) and `docs/{version}/` (approved). Each approval triggers a git commit. Journey IDs (JRN-v\*-NNN) → Architecture IDs (ARCH-v\*-NNN) provide cross-stage traceability.

## Build Strategy

v0.1.0 is an **initial import baseline** — the entire codebase was reverse-engineered and imported rather than built from scratch via the pipeline. No incremental milestones were executed; the single milestone was the import itself. There are no tracked bead tasks or dependency graphs for this iteration. Definition of done was artifact review and approval by the user.

## Suggested Enhancements

- **Multi-model selection per stage**: Allow users to override the default model (Sonnet for Vision/UX, Opus for Architecture/Build) per project or per stage, unlocking cost/quality tradeoffs and future support for non-Claude providers
- **Direct Anthropic API integration**: Replace the `claude` CLI subprocess with direct HTTP calls to the Anthropic API, removing the local CLI installation requirement and enabling cloud/SaaS deployment scenarios
- **Multi-user / cloud deployment support**: Add lightweight authentication (API key or OAuth), tenant isolation per user, and configurable remote storage (S3 or equivalent) to move beyond the single-user local-first model
- **Enhanced bead execution observability**: Surface per-bead agent conversation logs, retry history, and devil's advocate review iterations in the Execute UI, improving debuggability when beads are blocked
- **CI/CD pipeline integration hooks**: Add webhook or GitHub Actions integration points triggered on stage approval, enabling paulette to slot into existing team workflows without replacing them

## Known Limitations

- **Claude CLI dependency**: Requires local `claude` binary installation and authentication; no direct Anthropic API calls; limits concurrency and cloud deployment
- **Beads CLI dependency**: Requires local `bd` CLI and Dolt database installation on the host machine
- **Single-user, local-first only**: No authentication, multi-tenancy, or user isolation; `REGISTRY_PATH` defaults to `~/.paulette`
- **File-system-only persistence**: No relational database; limits query capability and concurrent access
- **Fixed model assignment**: Claude Sonnet for Vision/UX, Claude Opus for Architecture/Build — no per-project override
- **CORS restricted to localhost**: Only `localhost:5173` and `localhost:8080` are allowed origins
- **Sequential stage advancement only**: No branching pipelines, concurrent stage work, or A/B artifact variants
- **No CI/CD, plugin system, or non-Claude LLM support**: All explicitly deferred to post-v1