# Product Vision: paulette

## Problem Statement

Building software from scratch is a fragmented, high-friction process. Product ideas get lost in translation between vision, design, architecture, and implementation. Developers spend enormous effort context-switching between tools, tracking decisions, and maintaining traceability from requirements to code. Existing AI coding assistants help with isolated tasks but do not guide the end-to-end journey from idea to deployed application — leaving teams to stitch together their own workflows with no guarantees of coherence, auditability, or repeatability.

This matters because the cost of misalignment between intent and implementation compounds at every stage. A weak vision produces a flawed UX design; a flawed UX produces a broken architecture; a broken architecture produces unmaintainable code. paulette enforces structure and traceability at every step, ensuring that what gets built is what was actually intended.

## Target Users

### Primary Users

**Product-minded builders** — solo founders, product managers, and technical leads who have a clear product idea but want AI acceleration across the full development lifecycle, not just code generation.

**AI-first development teams** — small teams who want to leverage autonomous agents for end-to-end development, with human approval gates at strategic checkpoints rather than continuous micro-management.

**Architects and tech leads** — engineers responsible for system design who want structured AI assistance that maintains cross-stage traceability (UX journeys → architecture components → build tasks).

### Secondary Users

**Legacy project teams** — developers who need to onboard an existing codebase into a structured, documented pipeline by importing and reverse-engineering its architecture, UX, and vision.

### User Needs

- A guided, stage-gated workflow that prevents skipping foundational decisions
- Real-time, conversational AI collaboration (not one-shot generation)
- Clear artifact versioning and rollback for every design decision
- Traceability from user journey IDs to architecture components to build tasks
- The ability to hand off execution to autonomous agents when desired

## Core Features

### 1. Five-Stage AI-Guided Pipeline
A sequential pipeline with mandatory human approval gates: **Vision → UX → Architecture → Build → Complete**. Each stage uses a specialised AI agent (Vision Advisor, UX Advisor, Architect, Build Advisor) tailored to that domain. Artifacts produced at each stage are injected as context into subsequent stages, ensuring coherent, cross-referenced outputs.

### 2. Conversational Artifact Refinement
Every stage is driven by real-time chat with a specialised AI agent streamed live via SSE. The user iterates on the artifact through natural conversation until satisfied, then approves to advance. Artifacts are extracted inline using structured delimiters and rendered as rich Markdown.

### 3. UX Mock Preview
At the UX stage, users can generate an HTML prototype rendered live in an iframe. Users select a UI framework (Tailwind, Bootstrap, Material UI, Shadcn/UI, Vanilla CSS, or custom) and iteratively refine the mock through chat prompts before approving the design.

### 4. Dependency-Aware Build Execution with Beads
The Build stage parses the approved build plan into a DAG of atomic tasks ("beads") using the `bd` CLI backed by a Dolt database. Beads are executed in parallel by configurable numbers of AI agents, with a devil's advocate review pattern applied up to three iterations per bead. A live DAG visualisation tracks bead status (open → in progress → done / blocked) in real time.

### 5. Cross-Stage Traceability
Journey IDs (JRN-v\*-NNN) link UX flows to architecture components (ARCH-v\*-NNN) to build tasks, creating an auditable trace from user story to implementation.

### 6. Git-Backed Version History & Rollback
Every artifact approval is committed to Git. Users can view the full commit history, reset to any prior commit, and roll back individual pipeline stages (with cascade to dependent stages). Remote sync (push/pull) is supported.

### 7. Autonomous Mode
A project-level toggle enables end-to-end autonomous execution: artifacts are auto-approved after generation, mocks are auto-generated, beads are auto-executed, and enhancement cycles run continuously (up to 10 iterations) using suggestions extracted from the project summary.

### 8. Enhancement Cycles
From the Completion screen, users can enter a new vision or improvement description, select a semantic version bump (patch/minor/major), and restart the pipeline with the enhancement context injected into all subsequent stage prompts — iterating on the product without starting from scratch.

### 9. Project Import & Reverse Engineering
An import agent accepts an existing local directory or git URL, walks the codebase, and calls Claude in four sequential steps to reverse-engineer Architecture → UX → Architecture cross-reference → Vision, pre-populating all artifacts with auto-generated content ready for human review.

### 10. Single-Binary Deployment
The compiled React SPA is embedded into the Go binary via `embed.FS`. The entire application ships as one executable with no external runtime dependencies beyond the `claude` CLI and `bd` CLI.

## User Experience

### Guiding Principles
- **Linear but reversible**: The pipeline enforces forward progression while making rollback safe and easy.
- **Always streaming**: All AI output streams in real time via SSE; users never wait for a spinner to resolve.
- **Human in the loop by default, autonomous on demand**: Approval gates are explicit; autonomous mode is opt-in.

### Key Interaction Patterns

**Stage-gated pipeline navigation**: The left sidebar displays all five stages with colour-coded status pips (grey = locked, blue = active, green = approved). Users can revisit approved stages but cannot skip ahead.

**Tab-based stage views**: Each stage exposes multiple tabs — Chat (always present), Artifact/Design, and stage-specific extras (Mock Preview for UX, Execute for Build). Tab switching is instant; content is preserved.

**Approve-to-advance**: A persistent green Approve button anchors the bottom of each active stage. It is disabled during streaming. Clicking it commits the artifact to Git and unlocks the next stage.

**Streaming with stop control**: A Stop button replaces Send during active streaming, allowing the user to interrupt generation at any point.

**Version History Modal**: Accessible from the project header, it shows the full Git log with stage-tag badges, supports one-click rollback to any commit, and includes a Remote section for push/pull sync.

**Autonomous mode indicator**: When autonomous mode is on, the Approve bar is replaced by an informational banner: *"Auto-approve active — will advance automatically"*, and the system self-drives with all activity visible in real time.

**Mobile-responsive layout**: Below 768px, the sidebar collapses to a horizontal scrollable strip; split layouts stack vertically; iOS zoom is prevented on input fields.

## Success Metrics

- **Pipeline completion rate**: Percentage of created projects that reach the Complete stage.
- **Time-to-completion**: Median wall-clock time from project creation to Build approval.
- **Enhancement adoption**: Percentage of completed projects that initiate at least one enhancement cycle.
- **Autonomous mode usage**: Percentage of projects run in autonomous mode; completion rate in autonomous vs. manual mode.
- **Artifact rollback frequency**: How often users roll back a stage (high frequency may indicate prompt quality issues).
- **Bead execution success rate**: Percentage of beads reaching `done` status without being blocked.
- **Import-to-review conversion**: Percentage of imported projects where the user approves at least one stage after import.
- **Token efficiency**: Average token cost per completed pipeline, trending toward lower cost over model and prompt improvements.

## Constraints & Assumptions

- **Claude CLI dependency**: The system invokes the `claude` binary as a subprocess rather than calling the Anthropic API directly. This constrains deployment (the host must have the CLI installed and authenticated) and limits concurrency control.
- **Beads CLI dependency**: The `bd` CLI and its Dolt-backed database must be installed on the host. Bead execution is tightly coupled to this tool.
- **File-system persistence**: All state (projects, artifacts, chat histories) is stored as JSON/Markdown files. There is no relational database. This is intentional for Git-friendliness but limits query capability and multi-user access.
- **Single-user, local-first**: The default `REGISTRY_PATH` (`~/.paulette`) and single-binary model assume a single developer running the tool locally. There is no authentication, multi-tenancy, or user isolation.
- **Sequential stage advancement**: The pipeline does not support concurrent stage work or branching pipelines; stages must be approved in order.
- **Model selection is fixed per stage**: Vision/UX use Claude Sonnet; Architecture/Build use Claude Opus. Users cannot currently override model selection per project.
- **CORS allows localhost only**: The server allows origins `localhost:5173` and `localhost:8080`, confirming the local-first assumption.

## Out of Scope (V1)

- **Multi-user collaboration**: No real-time co-editing, user accounts, or access control.
- **Cloud hosting / SaaS deployment**: No managed cloud infrastructure, authentication layer, or tenant isolation.
- **Direct Anthropic API integration**: No HTTP API calls to Anthropic; only the `claude` CLI subprocess is used.
- **Non-Claude AI providers**: No support for OpenAI, Gemini, or other LLM backends.
- **Relational or cloud database**: No PostgreSQL, MySQL, or managed storage; file-system only.
- **CI/CD pipeline integration**: No native hooks into GitHub Actions, GitLab CI, or other automation platforms.
- **Plugin or extension system**: No marketplace or third-party agent extensibility.
- **Mobile-native applications**: Responsive web only; no iOS/Android native apps.
- **Branching pipelines**: No support for A/B artifact variants or parallel stage tracks.
- **Real-time multi-agent collaboration within a stage**: Agents operate sequentially per stage; only the Build stage parallelises execution across beads.