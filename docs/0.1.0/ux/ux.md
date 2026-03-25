# UX Design: paulette

## User Journeys

### JRN-v0.1.0-001: Create and Launch a New Project

A user visits the paulette home screen, sees an ASCII-art branded banner and a grid of existing projects. They click **"+ New Project"**, which opens a modal asking for a project name and a host directory path. On submission, the backend initialises the directory with `.paulette/` scaffolding, runs `git init`, and registers the project. The user is taken into the project workspace at the **Vision** stage.

### JRN-v0.1.0-002: Import an Existing Codebase

From the home screen, the user clicks **"Import Repo"**. A modal accepts either a local directory path or a git URL. On submit, an import agent walks the codebase and reverse-engineers four artifacts (Architecture → UX → Cross-refs → Vision) by calling Claude in sequential steps. The **ImportProgressView** screen shows a live multi-step progress log streamed over SSE, with colour-coded step completion indicators. When all steps are done, a **"Start Review"** button takes the user into the project workspace with pre-populated artifacts marked as auto-generated.

### JRN-v0.1.0-003: Develop the Vision Document

Inside the project workspace, the user is at the **Vision** stage. The chat panel auto-sends an opening prompt: *"Let's start building your product vision..."*. The user exchanges messages with the AI Vision Advisor (streamed live). When satisfied with the vision document appearing in the **Artifact** tab, the user clicks **Approve**. The pipeline advances to UX.

### JRN-v0.1.0-004: Design the UX

At the **UX** stage, the AI UX Advisor auto-initiates discussion of user flows and screens. The stage has three tabs: **Chat**, **UX Design** (the Markdown artifact), and **Mock Preview**.

- The user selects a UI framework from a dropdown (Tailwind, Bootstrap, Material UI, Shadcn, Vanilla CSS, or custom).
- They can enter optional refinement text and click **Generate Mock** to produce an HTML prototype, rendered live in an iframe.
- While mock HTML is being generated, a side panel streams agent activity tokens in real time.
- The user iterates on the mock via refinement prompts until satisfied, then clicks **Approve**.

### JRN-v0.1.0-005: Approve the Architecture

At the **Architecture** stage, the AI Architect proposes the full technical architecture (powered by Opus). The user discusses and refines in the chat panel; the artifact appears in the **Artifact** tab rendered as rich Markdown. When satisfied, the user clicks **Approve**.

### JRN-v0.1.0-006: Generate and Execute the Build Plan

At the **Build** stage, the workspace has three tabs: **Chat**, **Build Plan**, and **Execute**.

1. **Plan phase**: An AI agent creates a build plan document in the Chat/Build Plan tabs.
2. **Generate Beads**: The user clicks **Generate Beads** to parse the plan into a DAG of atomic tasks (beads). A streaming generation panel shows the AI parsing work in real time.
3. **Execute**: The user switches to the **Execute** tab, optionally sets the number of parallel agents (default 1), and clicks **Execute Beads**. A dependency-aware DAG graph visualises beads in real time. Each bead transitions through states: open → claimed (in progress) → done or blocked. An execution log streams agent output at the bottom. A progress bar and agent badge track overall completion. On full completion, the user clicks **Approve** to move to the Complete stage.

### JRN-v0.1.0-007: Review Completion and Enhance

When the pipeline reaches **Complete**, the **CompletionView** screen appears with a green checkmark icon, a summary that streams in live (generated asynchronously), and a list of all approved artifacts (Vision, UX Design, Architecture, Build Plan) each clickable to navigate back to that stage.

The user can initiate an **Enhancement Cycle**:
- Enter a new vision or improvement description in the Enhancement textarea.
- Select a version bump (patch / minor / major).
- Click **Start Enhancement** — or use **Quick Enhance** which auto-extracts suggestions from the summary.
- The pipeline resets to the Vision stage with the enhancement context injected.

The user can also click **Start New Project** to return to the home screen.

### JRN-v0.1.0-008: Use Autonomous Mode

At any point in the project, the user can click the **Auto** toggle in the project header. In autonomous mode:
- Artifacts are auto-approved after generation (2-second delay for Vision and Architecture).
- The UX stage auto-switches to the Mock tab and auto-generates the mock.
- The Build stage auto-switches to Execute and runs beads automatically.
- After completion, the system auto-enhances using suggestions from the summary, iterating up to 10 times.
- An indicator in the Approve bar reads *"Auto-approve active — will advance automatically"*.

### JRN-v0.1.0-009: Inspect and Manage Version History

At any time while in a project, the user clicks the **History** button in the project header. The **Version History Modal** opens, showing a chronological list of git commits with hash, author, date, and message. Tags (e.g., approval stage markers) are shown as blue badge chips. The user can:
- **Reset to commit**: Click "Reset to", confirm in an inline confirmation dialog, and roll the project back to that point.
- **Discard uncommitted changes** with the Discard button when there are dirty (uncommitted) files.
- Expand the **Remote** section to set or remove a remote URL, then Push or Pull to sync with a remote git repository.

### JRN-v0.1.0-010: Reset or Roll Back a Stage

In the left sidebar, hovering over any approved stage reveals a reset icon (⟳). Clicking it opens a confirmation dialog. Resetting the **current (active) stage** clears its chat history and artifact. Resetting a **previously approved stage** (rollback) cascades: all subsequent stages are also cleared. The user confirms, and the pipeline is rolled back to the selected stage which becomes active again.

### JRN-v0.1.0-011: Review an Imported Project

When a project was imported (reverse-engineered from existing code), a banner is shown at the top of each active stage's view reading *"This artifact was auto-generated from your codebase. Review and refine via chat, then approve."* The auto-kickoff message is suppressed; the user manually reviews the pre-filled artifact and can refine it through chat before approving.

---

## Screen Descriptions

### Home Screen (Project List)
- **paulette ASCII banner** with gradient title, subtitle, and version label.
- **Project grid**: Cards for each existing project showing name, current stage (as colour-coded progress pips — grey=locked, blue=active, green=approved), current stage label, last-updated timestamp, and a delete (✕) button.
- **New Project card**: Dashed-border card with a "+" icon that opens the create modal.
- **Import Repo card**: Dashed-border card that opens the import modal.
- **Copyright footer** at the bottom.

### Create Project Modal
- Text input for project name.
- Text input for host directory path.
- Cancel and Create buttons.

### Import Repo Modal
- Text input for repository path or git URL.
- Cancel and Import buttons.

### Delete Confirmation Modal
- Confirmation prompt with the project name.
- Cancel and Delete buttons.

### Import Progress View (full-screen overlay within workspace)
- Step-by-step progress list: Architecture Analysis → UX Reverse-Engineering → Architecture Cross-Reference → Vision Synthesis → Build Plan Synthesis.
- Each step has a colour indicator: grey (pending), animated blue dot (in progress), green (done), red (error).
- Live scrolling log output streamed from the server.
- Error message displayed if any step fails.
- "Start Review" button shown on completion.

### Project Workspace (main application shell)
The shell has three zones:
1. **ProjectHeader** (top bar): Back button, project name, version, author, token counter, Autonomous toggle, History button.
2. **StagesSidebar** (left): Stages list with status icons, token counts per stage, and a reset button on hover. paulette ASCII branding at the bottom.
3. **Main Content** (right): Stage content area.

### Vision Stage View
- **Tabs**: Chat | Artifact
- Chat panel (default): Conversational interface with the Vision Advisor.
- Artifact tab: Markdown-rendered vision document.
- Approve bar at bottom.

### UX Stage View
- **Tabs**: Chat | UX Design | Mock Preview
- Chat tab: Conversational interface with the UX Advisor.
- UX Design tab: Markdown-rendered UX artifact.
- Mock Preview tab: Full-height iframe showing generated HTML prototype. Framework selector and refinement input with Generate/Stop buttons in the tab bar.
- Mock generation activity panel (right side panel) with streaming tokens and live agent output.
- Approve bar at bottom.

### Architecture Stage View
- **Tabs**: Chat | Artifact
- Chat tab: Conversational interface with the Architect.
- Artifact tab: Markdown-rendered architecture document.
- Approve bar at bottom.

### Build Stage View
- **Tabs**: Chat | Build Plan | Execute
- Chat tab: Conversational interface with the Build Advisor.
- Build Plan tab: Markdown-rendered build plan artifact. "Generate Beads" button in tab bar.
- Execute tab: DAG graph of beads (colour-coded by status), bead config bar (max parallel agents input), execution log panel, and action buttons (Execute Beads / Stop).
- Plan limit warning banner if Claude hits its turn limit.
- Approve bar at bottom.

### Complete / Completion View
- Split layout: left panel (actions), right panel (summary).
- Left: Green checkmark icon, "Build Complete!" heading, subtitle, list of approved artifacts (each a clickable link to navigate to that stage), enhancement form (vision textarea + version bump radio + Start Enhancement / Quick Enhance buttons), and "Start New Project" button.
- Right: Streaming or fetched project summary in Markdown, with a Retry button if generation fails.

### Version History Modal
- List of git commits (hash chip, message, author, date, stage tag badges, Reset button).
- Dirty indicator banner with Discard button when there are uncommitted changes.
- Remote settings accordion (show/hide): URL input, Set/Remove buttons, Push/Pull buttons.
- Error banner for failed operations.
- Close button.

---

## Navigation Flow

```
Home Screen (Project List)
    │
    ├── Click project card → Project Workspace [current stage]
    │       │
    │       ├── Stages Sidebar → Click any unlocked stage → [that stage view]
    │       │
    │       ├── Vision Stage
    │       │       └── Approve → UX Stage
    │       │
    │       ├── UX Stage
    │       │       └── Approve → Architecture Stage
    │       │
    │       ├── Architecture Stage
    │       │       └── Approve → Build Stage
    │       │
    │       ├── Build Stage
    │       │       └── Approve → Complete View
    │       │
    │       ├── Complete View
    │       │       ├── Enhance → resets to Vision Stage (new iteration)
    │       │       └── New Project → Home Screen
    │       │
    │       ├── History button → Version History Modal
    │       │       └── Reset to commit → Project Workspace [rolled-back state]
    │       │
    │       └── Back button → Home Screen
    │
    ├── "New Project" card → Create Modal → Project Workspace [Vision]
    │
    └── "Import Repo" card → Import Modal → ImportProgressView → Project Workspace [Vision, imported]
```

---

## Interaction Patterns

### Streaming & Live Feedback
- All AI-generated content (chat messages, artifacts, mock HTML, build execution logs, summary) streams in real time via Server-Sent Events (SSE).
- An animated "thinking" indicator (bouncing dots) is shown in the chat message role header while the agent is working.
- Token counts accumulate per stage and are displayed in the sidebar and header.
- A **Stop** button replaces the Send button during streaming; clicking it cancels the active SSE stream.

### Tab-Based Stage Navigation
- Each stage shows its content in multiple tabs (Chat, Artifact, and optionally Mock Preview or Execute).
- The active tab is highlighted with a bottom border. Tab switching is instant (client-side).
- Tabs reset to "Chat" when switching stages.

### Approve-to-Advance Flow
- An **Approve** button (green) is anchored at the bottom of the main content area for any active stage.
- The button is disabled while streaming is active.
- In autonomous mode the approve bar is replaced by an informational indicator.

### Modal Confirmation Dialogs
- Destructive actions (delete project, reset/rollback stage, reset to git commit) require a two-step confirmation: click action → confirmation modal with Cancel / Confirm buttons.
- Modals use a full-screen dark overlay; clicking outside closes them.

### Autonomous Mode
- A toggle button in the project header with an `active` CSS state (blue highlight when on).
- When active, the system advances automatically through stages with a 2-second delay between completion and approval.
- Progress is fully visible in real time even when autonomous.

### Bead Graph Visualisation
- Beads are rendered as a DAG using ReactFlow + Dagre layout.
- Node colours communicate status: amber = ready, blue = in progress, green = done, grey = blocked/locked.
- A legend at the bottom explains colours.
- An animated "agents active" indicator appears in the legend bar when execution is running.
- A progress bar shows X of N beads completed.

### Framework Selector (UX Stage)
- Dropdown in the UX tab bar with 6 preset options.
- Selecting "Other" reveals an inline text input for a custom framework name.
- Custom name is confirmed by blurring the field or pressing Enter.
- Selection is persisted to the server immediately.

### Token Accounting
- Tokens are counted per stage and summed into a grand total shown in the project header.
- Stage token counts appear in the sidebar next to each stage name.
- Counts use human-friendly formatting: values ≥ 1,000,000 show as "X.XXM", values ≥ 1,000 show as "XXXk", smaller values show as raw numbers.
- Token counts are persisted in the project model and restored when re-opening a project.

### Imported Project Indicators
- An informational banner at the top of each active stage's content area explains the pre-populated artifact.
- The auto-kickoff AI message is suppressed for imported projects — the user drives the conversation manually.
- The import progress screen (ImportProgressView) acts as a full-page interstitial before the standard workspace is shown.

### Git Remote Sync
- The Version History modal includes a collapsible Remote section.
- A green badge shows "remote set" when a remote URL is configured.
- Push and Pull buttons trigger git operations and surface errors inline.

### Responsive / Mobile Layout
- Below 768px, the left sidebar becomes a horizontal scrollable strip at the top of the app body.
- Split layouts (mock activity panel, execute details sidebar, completion left panel) stack vertically.
- Font sizes and padding reduce; `font-size: 16px` is applied to textarea/input elements to prevent iOS zoom.
- Sidebar branding and stage reset buttons are hidden on mobile.