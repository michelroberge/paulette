# Iteration Summary: claudine v0.1.0

## Product Overview
claudine is a local-first, AI-guided software development pipeline tool that takes a product idea from concept to running code through five sequential stages: Vision → UX → Architecture → Build → Complete. It targets product-minded builders, AI-first development teams, and tech leads who want structured, traceable AI collaboration across the full development lifecycle — not just isolated code generation. Each stage uses a specialized Claude-backed agent, enforces human approval gates, and produces versioned artifacts committed to Git, ensuring that what gets built matches what was intended.

## Key UX Decisions
- **Five-stage linear pipeline** with a left sidebar showing colour-coded status pips (grey/blue/green); users cannot skip stages forward but can roll back
- **Tab-based stage views**: every stage has a Chat tab + Artifact tab; UX adds Mock Preview; Build adds an Execute tab with a live DAG graph
- **Approve-to-advance** pattern: a persistent green Approve button anchors the bottom, disabled during active streaming
- **Streaming-first**: all AI output (chat, artifacts, mock HTML, build logs, summary) streams live via SSE with a Stop button replacing Send during generation
- **UX Mock Preview**: framework selector (Tailwind, Bootstrap, Material UI, Shadcn/UI, Vanilla CSS, custom) + iframe live render with iterative refinement
- **Bead DAG visualisation** using ReactFlow + Dagre; nodes colour-coded by status (amber/blue/green/grey); parallel agent count configurable
- **Autonomous mode toggle** in project header; replaces Approve bar with an informational banner and self-drives the full pipeline
- **Version History Modal**: full Git log with stage-tag badge chips, one-click rollback, and Remote push/pull sync
- **Import flow**: full-screen `ImportProgressView` interstitial with step-by-step SSE progress before workspace entry; imported stages show a review banner and suppress auto-kickoff
- **Mobile-responsive**: sidebar collapses to horizontal strip below 768px; split layouts stack vertically; 16px inputs prevent iOS zoom

## Architecture Summary
- **Backend**: Go 1.26, `go-chi/chi` v5 router, `rs/cors` (localhost:5173 & :8080), `google/uuid`; single binary via `embed.FS`
- **Frontend**: React 19 (TypeScript 5.9, Vite 8), `react-markdown` + `remark-gfm`, `@xyflow/react` + `@dagrejs/dagre`
- **AI Engine**: Anthropic `claude` CLI invoked as a subprocess (not direct API); conversation history serialized as a formatted string
- **Build/Issue Tracking**: `bd` CLI + Dolt database for bead DAG execution
- **Persistence**: File system only — JSON project registry + Markdown/text artifacts under `~/.claudine` (no relational DB)
- **Real-time**: Server-Sent Events for all streaming (chat chunks, artifact delimiters, token counts, import progress, build logs)
- **Git integration**: `os/exec` shell calls; every artifact approval = a Git commit; rollback via hard reset
- **Key API surface**: `POST /api/projects`, `PATCH /api/projects/:id`, `DELETE /api/projects/:id`, `/api/projects/:id/pipeline`, `/api/projects/:id/chat/:stage` (SSE), `/api/projects/:id/import` (SSE), `/api/projects/:id/beads/*`, `/api/projects/:id/git/*`
- **Artifact extraction**: inline delimiters `<!-- ARTIFACT:START -->…<!-- ARTIFACT:END -->` parsed from Claude output
- **Stage-to-model mapping**: Vision/UX → Claude Sonnet; Architecture/Build → Claude Opus (not user-configurable)

## Build Strategy
v0.1.0 was an **initial codebase import** — the build plan contains a single milestone ("Initial Import") acknowledging the existing implementation was reverse-engineered from source. No phased delivery milestones were defined. The definition of done was: all pipeline artifacts reviewed and approved by the human operator. This means the entire feature set described in Vision and UX was already implemented at the point of first iteration entry, rather than being incrementally built.

## Suggested Enhancements

1. **Direct Anthropic API Integration** — Replace `claude` CLI subprocess calls with direct HTTP calls to the Anthropic Messages API. This removes the host CLI dependency, enables proper token streaming control, exposes model selection per-stage as a project setting, and makes cloud/containerized deployment viable.

2. **Multi-User Support with Auth** — Add a lightweight authentication layer (JWT or API-key based) and per-user project namespacing. This unblocks team usage and shared-server deployments without requiring a full SaaS infrastructure build-out.

3. **Stage-Level Prompt Customization** — Allow users to edit or extend the system prompt for each stage (Vision Advisor, UX Advisor, Architect, Build Advisor) via the project settings panel. This addresses the current fixed-prompt limitation and lets teams encode domain conventions directly into the pipeline.

4. **Structured Artifact Diffing on Rollback** — When a user rolls back a stage or resets to a Git commit, show a side-by-side Markdown diff of the affected artifact(s) before confirming. Reduces accidental loss of approved work and makes rollback consequences explicit.

5. **CI/CD Export Hook** — Add a "Export to CI" action in the Completion view that generates a GitHub Actions or GitLab CI workflow file from the approved build plan beads, enabling teams to bridge the claudine pipeline into their existing automation infrastructure.

## Known Limitations
- **`claude` CLI subprocess dependency**: requires the CLI installed, authenticated, and on PATH on the host machine; no direct Anthropic API path
- **`bd` CLI + Dolt dependency**: bead execution is tightly coupled to this external toolchain
- **File-system-only persistence**: no relational DB; limits query capability and makes multi-user access unsafe
- **Single-user, local-first only**: `~/.claudine` default path; no authentication, multi-tenancy, or user isolation
- **Sequential stage advancement only**: no concurrent stage work, branching pipelines, or A/B artifact variants
- **Fixed model-per-stage**: Sonnet for Vision/UX, Opus for Architecture/Build; no per-project override
- **CORS locked to localhost**: `localhost:5173` and `localhost:8080` only; blocks any non-local deployment
- **v0.1.0 build plan is a no-op**: the initial build milestone contains no actionable tasks — future iterations must establish a real incremental build strategy
- **Out of scope (deferred to future versions)**: cloud/SaaS hosting, non-Claude AI providers, plugin/extension system, real-time multi-user collaboration, mobile-native apps, branching pipelines