# Iteration Summary: Claudette v0.1.0

## Product Overview
Claudette is a local-first, single-binary AI-guided software development pipeline tool that takes a product idea from raw concept to executable build plan through five sequential, AI-driven stages: Vision → UX → Architecture → Build → Complete. It targets product-minded builders, AI-first small teams, and technical leads who want structured, auditable AI acceleration across the full development lifecycle — not just code generation. Each stage uses a specialized Claude agent, enforces human approval gates, and maintains cross-stage traceability, ensuring that what gets built faithfully reflects what was intended.

## Key UX Decisions
- **Stage-gated left sidebar**: Five stages displayed with color-coded status pips (grey=locked, blue=active, green=approved); users can revisit approved stages but cannot skip forward.
- **Tab-based stage views**: Each stage exposes Chat + Artifact tabs minimum; UX adds Mock Preview; Build adds Execute — tab state is preserved on switch.
- **Approve-to-advance**: A persistent green Approve button anchors each active stage, disabled during streaming; clicking commits the artifact to Git and unlocks the next stage.
- **Always-streaming UI**: All AI output (chat, artifacts, mock HTML, build logs, summary) streams live via SSE with a Stop button replacing Send during active generation.
- **Bead DAG visualization**: Build execution uses ReactFlow + Dagre to render a live dependency graph with color-coded bead states (amber=ready, blue=in-progress, green=done, grey=blocked).
- **Autonomous mode**: A header toggle replaces the Approve bar with an informational banner and drives the entire pipeline end-to-end, including up to 10 enhancement cycles.
- **Version History Modal**: Full Git log with stage-tag badges, one-click rollback, and remote push/pull — accessible from the project header at any time.
- **Mobile-responsive**: Sidebar collapses to horizontal scrollable strip below 768px; split layouts stack vertically; iOS zoom prevented on inputs.

## Architecture Summary
- **Backend**: Go 1.26, chi v5 router, rs/cors middleware; single binary via `embed.FS` bundling the compiled React SPA.
- **Frontend**: React 19 + TypeScript 5.9, built with Vite 8; react-markdown + remark-gfm for rendering; @xyflow/react + @dagrejs/dagre for the bead DAG.
- **AI Engine**: Anthropic Claude CLI (`claude` subprocess) — not the HTTP API; Vision/UX use Sonnet, Architecture/Build use Opus.
- **Persistence**: File system only (JSON + Markdown under `~/.claudette`); Git-backed via `os/exec` for version history and artifact commits.
- **Real-time**: Server-Sent Events (SSE) for all streaming (chat chunks, artifacts, import progress, build logs, summary).
- **Build execution**: `bd` CLI + Dolt database for bead (DAG task) management; parallel agent execution configurable per run.
- **Key API surface**: All routes under `/api/projects`; handlers for project CRUD, import, pipeline state machine, chat, mock generation, bead generation/execution, git operations, and summary.
- **Data model highlights**: Projects stored as JSON with stage statuses, token counts, autonomous flag, framework selection, and enhancement context; artifacts stored as Markdown files; chat histories serialized per stage.

## Build Strategy
v0.1.0 represents an **initial import baseline** — the existing codebase was ingested wholesale via the import agent (reverse-engineered Architecture → UX → Architecture cross-reference → Vision). The build plan contains a single milestone (Import existing codebase) with no task breakdown or execution dependencies. No beads were generated or executed for this iteration. The Definition of Done was artifact review and approval by the user.

## Suggested Enhancements

- **Bead-level build execution for Claudette itself**: Generate a real task DAG from the architecture and build artifacts, break work into atomic beads, and execute them — exercising the core pipeline end-to-end and producing actual code changes rather than documentation only.

- **Model selection per project/stage**: Allow users to override which Claude model is used at the project or stage level (currently hardcoded: Sonnet for Vision/UX, Opus for Architecture/Build), enabling cost/quality trade-offs and future multi-provider support.

- **Multi-user / authentication layer**: Add lightweight authentication (API key or OAuth) and per-user registry paths to support small-team shared deployments, which is the next natural growth step beyond single-user local use.

- **Direct Anthropic API integration as an alternative to the CLI**: Replace or supplement the `claude` subprocess invocation with direct HTTP API calls, removing the CLI installation requirement, enabling proper concurrency control, and unlocking streaming via the API's native SSE support.

- **CI/CD pipeline export**: Add a "Export as GitHub Actions workflow" or similar output at the Complete stage, translating the approved build plan and beads into runnable automation — closing the loop between planning and deployment.

## Known Limitations
- **Claude CLI dependency**: Requires `claude` binary installed and authenticated on the host; no direct API integration; concurrency is subprocess-limited.
- **Beads CLI dependency**: `bd` CLI and Dolt database must be installed; bead execution is tightly coupled to this toolchain.
- **Single-user, local-first only**: No authentication, multi-tenancy, or user isolation; `REGISTRY_PATH` defaults to `~/.claudette`.
- **File-system persistence only**: No relational database; limited query capability and no multi-user concurrent writes.
- **Sequential stage advancement**: No branching pipelines, parallel stage work, or A/B artifact variants.
- **CORS locked to localhost**: Only `localhost:5173` and `localhost:8080` are allowed origins, enforcing the local-first constraint.
- **v0.1.0 build plan is a stub**: The current build plan contains no actionable tasks or beads — it is purely an import baseline pending real task decomposition in future iterations.
- **Out of scope (deferred)**: Cloud/SaaS hosting, non-Claude AI providers, relational/cloud database, CI/CD integration, plugin system, native mobile apps.