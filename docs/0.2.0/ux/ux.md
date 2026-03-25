# UX Design: Paulette v0.2.0

---

## User Journeys

### Preserved from v0.1.0

## JRN-v0.1.0-001: Create New Project
**Actor:** Any user  
**Entry point:** Home page  
**Steps:**
1. User lands on home page; sees project list (empty on first run) and a prominent **"New Project"** button.
2. User clicks "New Project"; a modal/inline form captures project name and optional description.
3. On submit, project is created in the registry, user is navigated to the project detail view, and the Vision stage is activated (blue pip).
4. The agent auto-sends an opening prompt; streaming begins immediately.

**Exit:** User is in the Vision stage Chat tab, watching the first AI response stream in.

---

## JRN-v0.1.0-002: Run a Pipeline Stage
**Actor:** Any user  
**Entry point:** Project detail view, any active stage  
**Steps:**
1. User is in an active stage (blue pip). Chat tab is shown by default.
2. User reads the AI-generated artifact draft in the Artifact tab; may switch back and forth freely (tab state preserved).
3. User types a follow-up message in the chat input and submits; a Stop button appears during streaming.
4. AI streams a response; token count updates in the top-right token meter.
5. User iterates until satisfied.

**Exit:** User is ready to approve; moves to JRN-v0.1.0-003.

---

## JRN-v0.1.0-003: Approve Stage and Advance
**Actor:** Any user  
**Entry point:** Active stage with a satisfactory artifact  
**Steps:**
1. User clicks the persistent green **"Approve"** button anchored at the bottom of the stage panel (disabled during streaming).
2. A confirmation prompt (inline or modal) summarises what will be committed.
3. On confirm: artifact is written to `docs/{version}/`, a git commit is created, the current stage pip turns green, and the next stage pip turns blue.
4. Navigation automatically moves to the newly activated stage.

**Exit:** Next stage is active; user begins JRN-v0.1.0-002 for that stage.

---

## JRN-v0.1.0-004: Enable Autonomous / Hands-Free Mode
**Actor:** Power user  
**Entry point:** Any active stage  
**Steps:**
1. User toggles the **"Autonomous"** switch in the stage header.
2. A warning banner explains that Paulette will auto-approve each stage without further human input.
3. The Approve button is replaced by an informational banner: _"Autonomous mode active — pipeline will self-advance."_
4. Paulette streams each stage, auto-approves, and advances through the full pipeline.
5. User can disengage autonomous mode at any time; the Approve button reappears for the current stage.

**Exit:** Pipeline reaches Complete stage, or user disengages and takes manual control.

---

## JRN-v0.1.0-005: Preview Mock UI (UX Stage)
**Actor:** Any user  
**Entry point:** UX stage, Mock Preview tab  
**Steps:**
1. User navigates to the UX stage and selects the **"Mock Preview"** tab (only visible in UX stage).
2. Paulette renders AI-generated HTML/CSS wireframe in a sandboxed iframe; streaming SSE delivers the HTML progressively.
3. User can interact with the mock (click, scroll) within the sandbox.
4. User switches back to Chat or Artifact tabs without losing mock state.

**Exit:** User is satisfied with the mock and approves the UX stage.

---

## JRN-v0.1.0-006: Execute Build Tasks (Bead DAG)
**Actor:** Developer  
**Entry point:** Build stage, Execute tab  
**Steps:**
1. User navigates to Build stage → **"Execute"** tab.
2. Paulette parses the architecture artifact into atomic beads; a ReactFlow + Dagre DAG is rendered.
3. Node colours indicate status: amber = ready, blue = in-progress, green = done, grey = blocked.
4. User clicks **"Run"**; Paulette executes beads in dependency order via the `bd` CLI.
5. Execution logs stream in real time below the DAG; individual bead logs are expandable.
6. On completion, all nodes turn green.

**Exit:** Build stage is fully executed; user approves to advance to Complete.

---

## JRN-v0.1.0-007: View Version History and Rollback
**Actor:** Any user  
**Entry point:** Project detail view, Version History button  
**Steps:**
1. User clicks the **"Version History"** button (clock icon in the project header).
2. A modal opens showing a full git log; each entry has stage-tag badges and a timestamp.
3. User clicks **"Rollback"** on any historical entry; a confirmation dialog explains which stages will be reset.
4. On confirm, git reset is performed, the registry is updated, and the UI reflects the restored state.
5. A collapsible **"Remote"** section at the bottom of the modal allows push/pull sync with a configured remote.

**Exit:** Modal closed; project is at the selected historical state.

---

## JRN-v0.1.0-008: Import Existing Project
**Actor:** Developer with an existing codebase  
**Entry point:** Home page, "Import Project" button  
**Steps:**
1. User selects a local directory path.
2. Paulette runs a four-step reverse-engineering analysis (streaming SSE for each step).
3. On completion, a new project is created with auto-generated artifacts for each stage.
4. Each stage shows an **"Auto-generated from import"** banner; auto-kickoff is suppressed.
5. User reviews and edits each artifact before approving.

**Exit:** User is in the Vision stage of the newly imported project.

---

### New in v0.2.0

## JRN-v0.2.0-001: First-Time Provider Setup
**Actor:** New user with no Claude CLI installed, or any user onboarding a new inference backend  
**Entry point:** Home page — **"Configure Paulette"** button  
**Steps:**
1. User lands on the home page and sees the **"Configure Paulette"** button (prominently placed alongside "New Project", not buried in a menu).
2. User clicks it; navigates to the **Configure Paulette** page, landing on the **Connections** tab.
3. The connection list is empty; a friendly empty-state message reads: _"No connections yet. Add one to start using a custom LLM provider."_
4. User clicks **"Add Connection"**; a slide-over panel or inline form appears (see JRN-v0.2.0-002).
5. After saving the first connection, user navigates to the **Stage Defaults** tab.
6. User assigns the new connection to each pipeline stage (see JRN-v0.2.0-006).
7. User returns to the home page and creates or opens a project. The pipeline runs using the configured connection.

**Success condition:** Full pipeline executes without `claude` CLI installed.  
**Exit:** Home page; user proceeds with normal project work.

---

## JRN-v0.2.0-002: Add a New Connection
**Actor:** Any user  
**Entry point:** Configure Paulette → Connections tab → "Add Connection" button  
**Steps:**
1. A **Connection Form** panel opens (slide-over on desktop, full-screen on mobile).
2. User fills in:
   - **Name** — free-text label (e.g. "Local Llama3", "Work Copilot")
   - **Provider Type** — dropdown; selecting a type dynamically shows/hides fields:
     - `Ollama` / `LM Studio` → **Base URL** field only (e.g. `http://localhost:11434`); no credential field
     - `Anthropic API` / `OpenAI` / `Gemini` → **API Key** field; optional **Base URL** override; optional **Org/Project ID** (OpenAI only)
     - `Claude CLI` → no fields (subprocess uses existing local auth)
     - `GitHub Copilot` → disabled entry labelled **"Coming Soon (v0.3.0)"** with a muted badge; not selectable
   - **Default Model** — populated via discovery or free-text fallback (see Interaction Patterns)
3. User clicks **"Test Connection"**; inline status appears (see JRN-v0.2.0-005).
4. If test passes and model list was retrieved, the model dropdown is populated; user selects a default model.
5. User clicks **"Save"**; a `chmod 600` credential-warning toast appears: _"Credentials saved to ~/.paulette/connections.json (user-readable only)."_
6. Connection appears in the list with a provider-type badge, name, and model label.

**Exit:** Connection form closed; new entry visible in the Connections list.

---

## JRN-v0.2.0-003: Edit an Existing Connection
**Actor:** Any user  
**Entry point:** Configure Paulette → Connections tab → "Edit" action on a connection row  
**Steps:**
1. The Connection Form panel opens pre-filled with the existing connection's values.
2. User modifies any field (name, URL, API key, model).
3. User may re-run **"Test Connection"** to validate changes.
4. User clicks **"Save"**; `connections.json` is updated; credential-warning toast appears if credentials were changed.
5. Any stage assignments referencing this connection (global or per-project) remain valid — the connection name is the stable key.

**Exit:** Connection form closed; updated entry visible in the list.

---

## JRN-v0.2.0-004: Delete a Connection
**Actor:** Any user  
**Entry point:** Configure Paulette → Connections tab → "Delete" action on a connection row  
**Steps:**
1. A confirmation dialog appears: _"Delete '[Connection Name]'? Any stage assignments using this connection will fall back to the Claude CLI default."_
2. User confirms; connection is removed from `connections.json`.
3. If any global or per-project stage assignments referenced this connection, those assignments are cleared and a warning toast lists the affected stages: _"Stage assignments for Vision, UX were reset to default."_

**Exit:** Connection removed; affected stage assignments reverted to Claude CLI fallback.

---

## JRN-v0.2.0-005: Test a Connection
**Actor:** Any user  
**Entry point:** Connection Form (during Add or Edit), or the Test button in the Connections list row  
**Steps:**
1. User clicks **"Test Connection"**.
2. Button transitions to a loading spinner; a status label reads _"Testing…"_
3. Paulette sends a minimal probe request to the provider.
4. **Success path:** Status label turns green — _"✓ Connected"_; if a model-list endpoint is available, the model dropdown is populated immediately.
5. **Failure path:** Status label turns red — _"✗ Connection failed: [short error reason]"_ (e.g. "Connection refused", "Invalid API key", "Timeout"); the save action is still permitted (user may save an untested or failing connection if they choose).
6. Feedback appears within 3 seconds for local providers (Ollama, LM Studio).

**Exit:** Test result inline; user continues editing or saves.

---

## JRN-v0.2.0-006: Set Global Stage Defaults
**Actor:** Any user  
**Entry point:** Configure Paulette → Stage Defaults tab  
**Steps:**
1. User sees a table with five rows — one per pipeline stage (Vision, UX, Architecture, Build, Complete).
2. Each row has:
   - **Stage label** (with the stage's colour-coded pip)
   - **Connection picker** — dropdown listing all saved connections + a "Claude CLI (default)" entry
   - **Model field** — dropdown if the selected connection returned a model list on last test; free-text input otherwise; pre-filled with the connection's default model but overridable here
3. User selects connections and models for each stage.
4. Changes are saved immediately to `~/.paulette/config.json` on each row change (no explicit Save button needed; a subtle _"Saved"_ inline confirmation appears per row).
5. New projects created after this point will inherit these stage defaults.

**Exit:** Stage Defaults tab; settings persisted.

---

## JRN-v0.2.0-007: Apply Per-Project Stage Override
**Actor:** Any user  
**Entry point:** Project detail view → gear icon (⚙) in the project header  
**Steps:**
1. User clicks the **⚙** gear icon; a **Project Stage Settings** panel slides in from the right (or opens as a modal on mobile).
2. The panel shows the same five-stage table as the global Stage Defaults tab, but with an additional **"Inherit"** toggle per stage.
3. Stages currently using global defaults show the toggle in the "on" (inherited) state with the global connection name shown in muted text.
4. To override a stage: user turns off the "Inherit" toggle; the row becomes editable with a connection picker and model field.
5. To revert: user turns the "Inherit" toggle back on; row returns to showing the global default.
6. A **"Reset all to global defaults"** link at the panel bottom resets every stage to inherited in one action.
7. Changes are saved immediately to the project's registry JSON.

**Exit:** Panel closed; project detail view reflects per-project assignments (a subtle "Custom" badge appears on any stage pip that has a project-level override).

---

## JRN-v0.2.0-008: Recover from Connection Error During Stage Execution
**Actor:** Any user  
**Entry point:** Any active pipeline stage during a streaming AI call  
**Steps:**
1. A stage fires and its assigned connection is unreachable (network error, auth failure, service down).
2. The streaming area stops; an **inline error banner** appears within the Chat tab: _"⚠ Connection '[Name]' is unreachable: [reason]. [Go to Configure →]"_
3. The error is not a silent hang — it appears immediately upon failure detection.
4. **"Go to Configure →"** is a direct link to the Configure Paulette page, Connections tab, with the failing connection highlighted.
5. User fixes the connection (corrects URL, rotates key, etc.) and re-tests.
6. User returns to the project; the stage remains active (no data lost); user re-sends or re-triggers the failed request.
7. Alternatively, user opens the Project Stage Settings (⚙) and reassigns that stage to a different working connection without leaving the project context.

**Exit:** Stage continues streaming on the repaired or reassigned connection.

---

## JRN-v0.2.0-009: GitHub Copilot Connection (P3 — Coming Soon)
**Actor:** Enterprise developer  
**Entry point:** Configure Paulette → Connections tab → Add Connection → Provider Type → GitHub Copilot  
**Steps (target state for v0.3.0; shown as Coming Soon in v0.2.0):**
1. Provider Type dropdown shows "GitHub Copilot" with a **"Coming Soon"** badge; the option is visible but disabled.
2. A tooltip on hover reads: _"GitHub Copilot OAuth support is planned for v0.3.0."_
3. No form fields are shown or interactive for this provider in v0.2.0.

**Exit:** User selects a different provider type.

---

## Screen Descriptions

### Preserved from v0.1.0

#### SCR-001: Home Page
- Full-width layout with a top `nav` bar containing the Paulette wordmark and a token-usage meter (top-right).
- Below the nav: a two-column responsive grid. Left column: **project list** (cards with project name, last-updated timestamp, current stage pip indicator). Right area (or top on mobile): action buttons.
- **[MODIFIED v0.2.0]** Action buttons row now contains three buttons side by side:
  - `bg-blue-600 text-white` **"New Project"** (primary)
  - `bg-white border border-gray-300 text-gray-700` **"Import Project"** (secondary)
  - `bg-white border border-gray-300 text-gray-700` **"Configure Paulette"** ⚙ **[NEW]** — same visual weight as Import Project; placed to the right of it
- Empty state: centred illustration + "No projects yet. Create your first project to get started." copy.
- Mobile: action buttons stack vertically; project cards are full-width.

#### SCR-002: Project Detail View
- Three-column layout on desktop (`grid grid-cols-[240px_1fr_320px]`):
  - **Left sidebar** — stage navigator (five items, each with a colour-coded pip and label)
  - **Centre panel** — active stage content (chat + tabs)
  - **Right panel** — artifact viewer (collapsible on tablet, hidden on mobile)
- Stage pips: `bg-gray-400` locked, `bg-blue-500` active, `bg-green-500` approved.
- Stage tab strip: `border-b border-gray-200`; active tab `border-b-2 border-blue-600 text-blue-600`.
- **[MODIFIED v0.2.0]** Project header bar (above the three-column layout) gains a **⚙ gear icon button** (`text-gray-500 hover:text-gray-700`) that triggers the Project Stage Settings panel (JRN-v0.2.0-007). Tooltip: _"Stage connection settings"_.
- If any stage has a per-project override, that stage's pip gets a small `ring-2 ring-yellow-400` accent to signal a custom assignment.
- Token meter is persistent in the header.
- Stop button (`bg-red-500 text-white`) appears in the active stage header only during active streaming.
- Approve button: `bg-green-600 hover:bg-green-700 text-white font-semibold` anchored at the bottom of the centre panel; `opacity-50 cursor-not-allowed` during streaming.

#### SCR-003: Chat Interface (within any stage)
- Input: `w-full rounded-lg border border-gray-300 focus:ring-2 focus:ring-blue-500 px-4 py-2 resize-none`.
- Message bubbles: user messages right-aligned `bg-blue-50`; AI messages left-aligned `bg-white border border-gray-100`.
- Streaming text renders progressively; cursor blink animation during active stream.
- **[MODIFIED v0.2.0]** If the assigned connection is unreachable, an inline error banner replaces the streaming area (see SCR-010 below).

#### SCR-004: Mock Preview Tab (UX Stage only)
- Full-height `iframe` with `sandbox="allow-scripts allow-same-origin"`.
- A thin toolbar above the iframe shows viewport toggles (desktop / tablet / mobile) implemented with Tailwind width constraints.
- Progressive HTML rendering via SSE; a skeleton loader (`animate-pulse bg-gray-100`) shows until the first chunk arrives.

#### SCR-005: Bead DAG Execute Tab (Build Stage only)
- ReactFlow canvas fills the centre panel; Dagre layout applied automatically.
- Node styles: amber `bg-amber-100 border-amber-400`, blue `bg-blue-100 border-blue-400`, green `bg-green-100 border-green-400`, grey `bg-gray-100 border-gray-300`.
- Below the DAG: a scrollable log pane (`font-mono text-sm bg-gray-900 text-gray-100`); individual bead log sections are collapsible.
- "Run" button: `bg-blue-600 text-white`; "Stop" button replaces it during execution.

#### SCR-006: Version History Modal
- `max-w-2xl mx-auto` modal with a header ("Version History") and a scrollable git log.
- Each entry: timestamp, short commit hash `font-mono text-xs text-gray-500`, stage-tag badge (`bg-blue-100 text-blue-700 text-xs rounded-full px-2 py-0.5`), and a "Rollback" button (`text-red-600 hover:underline text-sm`).
- Collapsible "Remote" section at the bottom with push/pull buttons.

---

### New in v0.2.0

#### SCR-007: Configure Paulette Page [NEW]
- Full-page route (`/configure`); accessible from the home page "Configure Paulette" button and from the "Go to Configure →" link in connection error banners.
- **Page header:** `text-2xl font-bold text-gray-900` title "Configure Paulette" with a breadcrumb `← Home` link (`text-blue-600 hover:underline text-sm`).
- **Two-tab strip** below the header: `border-b border-gray-200` with tabs "Connections" and "Stage Defaults"; active tab `border-b-2 border-blue-600 font-medium`.
- Layout: `max-w-4xl mx-auto px-6 py-8` — comfortable reading width, not full-bleed.
- Mobile: tabs stack; forms go full-width.

#### SCR-008: Connections Tab [NEW]
- **List area:** each connection is a card `rounded-lg border border-gray-200 bg-white shadow-sm p-4 flex items-center justify-between`.
  - Left: provider-type badge (`text-xs font-medium rounded-full px-2 py-0.5`) — colour-coded by provider:
    - Ollama: `bg-purple-100 text-purple-700`
    - LM Studio: `bg-indigo-100 text-indigo-700`
    - Anthropic API: `bg-orange-100 text-orange-700`
    - OpenAI: `bg-green-100 text-green-700`
    - Gemini: `bg-blue-100 text-blue-700`
    - Claude CLI: `bg-gray-100 text-gray-600`
  - Centre: connection name (`font-medium text-gray-900`) and default model (`text-sm text-gray-500`).
  - Right: **"Test"** (`text-sm text-blue-600 hover:underline`), **"Edit"** (`text-sm text-gray-600 hover:underline`), **"Delete"** (`text-sm text-red-600 hover:underline`) inline actions.
- **Empty state:** `text-center py-16 text-gray-400` with a plug icon and the message: _"No connections yet. Add one to start using a custom LLM provider."_
- **"Add Connection"** button: `bg-blue-600 hover:bg-blue-700 text-white font-medium px-4 py-2 rounded-lg` — placed above the list (top-right of the tab content area).

#### SCR-009: Connection Form Panel [NEW]
- Rendered as a **slide-over** on desktop: `fixed inset-y-0 right-0 w-96 bg-white shadow-xl z-50 flex flex-col`. On mobile: full-screen overlay.
- **Header:** "Add Connection" or "Edit Connection" in `text-lg font-semibold`; an × close button top-right.
- **Form fields** (`space-y-4 p-6 flex-1 overflow-y-auto`):
  - **Name** — `<input type="text">` with label "Connection Name" and placeholder "e.g. Local Llama3".
  - **Provider Type** — `<select>` dropdown; selecting a value triggers conditional field rendering:
    - `Ollama` / `LM Studio`: shows **Base URL** (`<input type="url">` placeholder `http://localhost:11434`); no credential field.
    - `Anthropic API`: shows **API Key** (`<input type="password">`); optional **Base URL** override.
    - `OpenAI`: shows **API Key** (`<input type="password">`); optional **Org ID** and **Project ID** text fields; optional **Base URL** override.
    - `Gemini`: shows **API Key** (`<input type="password">`).
    - `Claude CLI`: no additional fields; a muted info note: _"Uses the local `claude` binary and its existing authentication."_
    - `GitHub Copilot`: entire option is `text-gray-400 cursor-not-allowed` in the dropdown; if somehow focused, a tooltip reads _"Coming soon in v0.3.0"_.
  - **Default Model** — conditional:
    - If model list discovered: `<select>` dropdown populated with discovered models; a `text-xs text-green-600` label "Models fetched from provider" appears below.
    - If not discovered: `<input type="text">` placeholder "e.g. llama3:8b" with a `text-xs text-gray-400` note "Enter model name manually".
  - **Test Connection** button: `w-full border border-blue-600 text-blue-600 hover:bg-blue-50 rounded-lg py-2 font-medium`; loading state shows a spinner; result inline below the button (see JRN-v0.2.0-005).
- **Credential warning banner** (shown when API key field is non-empty): `bg-yellow-50 border border-yellow-200 text-yellow-800 text-xs rounded p-2` — _"⚠ Credentials are saved in plain text to ~/.paulette/connections.json (user-readable only). No keychain integration in v0.2.0."_
- **Footer actions** (`border-t border-gray-200 p-4 flex gap-3`):
  - **"Save"**: `bg-blue-600 text-white px-4 py-2 rounded-lg font-medium flex-1`
  - **"Cancel"**: `bg-white border border-gray-300 text-gray-700 px-4 py-2 rounded-lg flex-1`

#### SCR-010: Stage Defaults Tab [NEW]
- A table `w-full border-collapse` with columns: Stage | Connection | Model.
- Each row `border-b border-gray-100 py-3`:
  - **Stage column**: stage pip (colour-coded dot `w-2 h-2 rounded-full inline-block mr-2`) + stage label `font-medium text-gray-700`.
  - **Connection column**: `<select>` dropdown listing all saved connections + a "Claude CLI (default)" entry at the top. Width `min-w-[200px]`.
  - **Model column**: `<select>` or `<input type="text">` depending on discovery state; shows the connection's default model pre-filled.
- Save behaviour: **auto-save per row** on blur/change; a `text-xs text-green-600 ml-2` inline "Saved ✓" confirmation fades in and out after each save (using `transition-opacity duration-500`). No explicit submit button needed.
- A `text-sm text-gray-500 mt-4` note at the bottom: _"These defaults apply to all new projects. Existing projects can override per-stage from the project's ⚙ settings."_

#### SCR-011: Project Stage Settings Panel [NEW]
- Triggered by the **⚙ gear icon** in the project detail header.
- Rendered as a **slide-over** on desktop: `fixed inset-y-0 right-0 w-[420px] bg-white shadow-xl z-50 flex flex-col`. On mobile: full-screen.
- **Header:** _"Stage Connection Settings — [Project Name]"_ `text-base font-semibold`; × close button.
- **Body** (`p-6 space-y-4 overflow-y-auto flex-1`):
  - Same five-stage table as SCR-010, but each row has an additional **"Inherit"** toggle (`relative inline-flex h-5 w-9` Tailwind switch component) in a fourth column.
  - **Inherit ON** (default): Connection and Model cells are read-only, showing the global default values in `text-gray-400` muted text. Toggle is `bg-blue-500`.
  - **Inherit OFF**: Connection and Model cells become editable dropdowns/inputs. Toggle is `bg-gray-300`.
  - If a stage has a project-level override, its row has a subtle `bg-yellow-50` background tint.
- **Footer** (`border-t border-gray-200 p-4`):
  - **"Reset all to global defaults"** link: `text-sm text-red-600 hover:underline`; clicking flips all toggles back to Inherit ON.
  - Save behaviour mirrors SCR-010: auto-save per row with inline "Saved ✓" confirmation.

#### SCR-012: Inline Connection Error Banner [NEW]
- Appears inside the Chat tab of any active stage, replacing or overlaying the streaming area when a connection fails.
- Styling: `bg-red-50 border border-red-200 rounded-lg p-4 flex items-start gap-3 mx-4 my-2`.
- Icon: `text-red-500` warning triangle (⚠), `text-sm`.
- Text: `text-red-800 text-sm`:
  ```
  Connection "[Name]" is unreachable.
  [Short error reason — e.g. "Connection refused at http://localhost:11434"]
  ```
- CTA link: `text-blue-600 hover:underline font-medium text-sm` — **"Go to Configure →"** — navigates to `/configure` with the failing connection highlighted (deep-linked via query param `?highlight=[connection-id]`).
- Secondary CTA: `text-gray-600 hover:underline text-sm ml-4` — **"Change stage connection"** — opens the Project Stage Settings panel (SCR-011) directly, so the user can reassign without navigating away.
- The banner does not auto-dismiss; it persists until the user either fixes the connection and retries, or navigates away.

---

## Navigation Flow

```
Home Page (/)
├── [New Project] ──────────────────────────────────────────┐
├── [Import Project] ────────────────────────────────────────┤
│                                                            ▼
├── [Configure Paulette ⚙] ──► /configure             Project Detail View
│     ├── Tab: Connections                             (/projects/:id)
│     │     ├── [Add Connection] ──► Connection Form      │
│     │     │     └── [Test] → inline pass/fail            │
│     │     ├── [Edit] ──────► Connection Form             │
│     │     └── [Delete] ─────► Confirmation Dialog        │
│     └── Tab: Stage Defaults                              │
│           └── Per-stage picker (auto-save)               │
│                                                           │
└── Project Cards ──────────────────────────────────────────┘
                                                            │
                        ┌───────────────────────────────────┘
                        ▼
              Project Detail View
              ├── Left Sidebar: Stage Navigator
              │     └── Vision → UX → Architecture → Build → Complete
              │           (grey/blue/green pips)
              ├── Centre: Stage Content (Chat + Artifact + extras)
              │     └── [Inline Connection Error] ──► /configure?highlight=X
              │                                   └──► Project Stage Settings ◄──┐
              ├── Header: [⚙ Gear] ────────────────────────────────────────────┘
              │     └── Project Stage Settings Panel (slide-over)
              │           ├── Per-stage Inherit toggles
              │           └── [Reset all to global defaults]
              └── Header: [Version History] ──► Version History Modal
```

---

## Interaction Patterns

### Preserved from v0.1.0

#### IACT-001: Always-Streaming SSE Output
All AI-driven calls (chat messages, artifact generation, mock HTML, bead execution, import analysis, summary) use Server-Sent Events. Output renders progressively in the UI. A **Stop** button (`bg-red-500 text-white text-sm px-3 py-1 rounded`) appears in the active stage header immediately when a stream begins and disappears when the stream ends or is stopped.

#### IACT-002: Approve-to-Advance Gate
The Approve button is always visible at the bottom of the active stage's centre panel. It is `opacity-50 cursor-not-allowed pointer-events-none` during any active stream. On click, a confirmation step (inline collapsed panel or lightweight modal) summarises the commit that will be created. Confirmation triggers the pipeline advancement and a git commit.

#### IACT-003: Tab State Preservation
Switching between Chat, Artifact, Mock Preview, and Execute tabs within a stage does not reset state. Scroll position, partial input text, and streaming content are preserved in React state (not remounted).

#### IACT-004: Autonomous Mode Toggle
A labelled toggle in the stage header. When enabled: Approve button is hidden; replaced by an `bg-blue-50 border border-blue-200 text-blue-700 text-sm rounded p-2` info banner. Toggling off mid-pipeline restores the Approve button for the current stage. No stage advancement occurs without the toggle active.

#### IACT-005: Mobile Sidebar Collapse
Below `768px` breakpoint (`md:` in Tailwind), the left sidebar collapses into a horizontal stage-pip strip fixed at the bottom of the viewport. Tapping a pip navigates to that stage if accessible. Split layouts (centre + right panel) stack vertically.

#### IACT-006: Imported Project Suppression
For imported projects, each stage renders a `bg-amber-50 border border-amber-200 text-amber-800 text-sm rounded p-3 mb-4` banner: _"This artifact was auto-generated from an import. Review before approving."_ Auto-kickoff (sending the initial agent prompt) is suppressed — the user must manually send the first message.

---

### New in v0.2.0

#### IACT-007: Provider-Type-Driven Form Fields [NEW]
In the Connection Form (SCR-009), selecting a Provider Type from the dropdown immediately and synchronously shows/hides form fields without a round-trip. This is a pure React controlled-component pattern — no loading state between field transitions. Fields that become hidden are cleared from form state so stale values are not persisted.

Fields by provider:
| Provider | Base URL | API Key | Org/Project ID | OAuth Button |
|---|---|---|---|---|
| Ollama | ✅ Required | — | — | — |
| LM Studio | ✅ Required | — | — | — |
| Anthropic API | Optional override | ✅ Required | — | — |
| OpenAI | Optional override | ✅ Required | Optional | — |
| Gemini | Optional override | ✅ Required | — | — |
| Claude CLI | — | — | — | — |
| GitHub Copilot | — | — | — | Disabled / Coming Soon |

#### IACT-008: Opportunistic Model Discovery [NEW]
When the user clicks **"Test Connection"**, Paulette simultaneously:
1. Sends a minimal probe request to verify connectivity/auth.
2. Attempts a model-list fetch (e.g. `GET /api/tags` for Ollama, `GET /v1/models` for OpenAI-compatible).

If the model list fetch succeeds: the Model field transitions from a `<input type="text">` to a `<select>` populated with discovered model names; a `text-xs text-green-600` label "Models fetched ✓" appears.  
If the model list fetch fails (404, unsupported, network error): the Model field silently remains as free-text input; no error is shown for this specifically — only the main connectivity probe result is surfaced. This fallback is **silent and non-blocking**.

The model dropdown/input state is also re-evaluated whenever the user changes the **Base URL** for Ollama/LM Studio connections, but only on explicit Test — not on every keystroke.

#### IACT-009: Credential Warning Toast [NEW]
Whenever credentials (API keys, OAuth tokens) are written to disk, a `fixed bottom-4 right-4 z-50` toast notification appears:
- `bg-yellow-50 border border-yellow-300 text-yellow-800 shadow-lg rounded-lg px-4 py-3 text-sm max-w-sm`
- Content: _"⚠ Credentials saved to ~/.paulette/connections.json (user-readable only). No keychain integration in this version."_
- Auto-dismisses after 6 seconds or on manual click. Uses `transition-opacity duration-300` for fade-out.
- Does **not** appear for Ollama/LM Studio connections (no credentials) or Claude CLI connections.

#### IACT-010: Auto-Save with Inline Confirmation (Stage Defaults) [NEW]
In the Stage Defaults tab (SCR-010) and Project Stage Settings panel (SCR-011), changes are written to disk on each individual row interaction (dropdown change or input blur) rather than requiring a global Save button. After each write:
- A `text-xs text-green-600` label "Saved ✓" fades in next to the changed field using `transition-opacity duration-200`.
- It fades out after 2 seconds (`opacity-0`).
- If the write fails (disk error), the label instead shows `text-red-600` "Save failed — retry?" with a retry link.

This pattern keeps the configuration surface low-friction and eliminates "did I save?" anxiety.

#### IACT-011: Deep-Link to Failing Connection [NEW]
When the user clicks **"Go to Configure →"** from an inline connection error banner (SCR-012), they are routed to `/configure?tab=connections&highlight=[connection-id]`. On page load, the Connections tab is selected and the relevant connection card receives a temporary `ring-2 ring-red-400 animate-pulse` highlight that persists for 3 seconds before fading, drawing the eye to the specific record that needs attention.

#### IACT-012: Inherit Toggle Behaviour in Project Stage Settings [NEW]
The Inherit toggle in SCR-011 controls whether a stage uses the global default or a project-specific override:
- **Toggle ON → OFF** (overriding): The connection/model fields animate from read-only to editable (`transition-all duration-150`); they are pre-populated with the current global default values as a starting point so the user doesn't start from scratch.
- **Toggle OFF → ON** (reverting to global): The fields fade back to muted read-only display; the project-level assignment for that stage is deleted from the project registry JSON.
- **"Reset all to global defaults"** performs the OFF → ON transition for all five stages simultaneously with a single confirmation dialog: _"Reset all stage overrides for [Project Name]? This cannot be undone."_

---

## Component Conventions (Tailwind)

### Preserved from v0.1.0

| Component | Tailwind pattern |
|---|---|
| Primary button | `bg-blue-600 hover:bg-blue-700 text-white font-medium px-4 py-2 rounded-lg transition-colors` |
| Secondary button | `bg-white border border-gray-300 text-gray-700 hover:bg-gray-50 font-medium px-4 py-2 rounded-lg transition-colors` |
| Danger button | `bg-red-600 hover:bg-red-700 text-white font-medium px-4 py-2 rounded-lg transition-colors` |
| Approve button | `bg-green-600 hover:bg-green-700 text-white font-semibold px-6 py-3 rounded-lg w-full transition-colors` |
| Stop button | `bg-red-500 hover:bg-red-600 text-white text-sm px-3 py-1 rounded transition-colors` |
| Text input | `w-full rounded-lg border border-gray-300 focus:ring-2 focus:ring-blue-500 focus:border-transparent px-4 py-2 text-sm` |
| Status pip — locked | `w-2.5 h-2.5 rounded-full bg-gray-400` |
| Status pip — active | `w-2.5 h-2.5 rounded-full bg-blue-500` |
| Status pip — approved | `w-2.5 h-2.5 rounded-full bg-green-500` |
| Info banner | `bg-blue-50 border border-blue-200 text-blue-700 text-sm rounded-lg p-3` |
| Warning banner | `bg-amber-50 border border-amber-200 text-amber-800 text-sm rounded-lg p-3` |
| Card | `rounded-lg border border-gray-200 bg-white shadow-sm p-4` |
| Modal overlay | `fixed inset-0 bg-black/50 z-40 flex items-center justify-center` |
| Modal panel | `bg-white rounded-xl shadow-2xl max-w-2xl w-full mx-4 max-h-[80vh] overflow-y-auto` |
| Monospace log area | `font-mono text-sm bg-gray-900 text-gray-100 rounded-lg p-4 overflow-y-auto` |

### New in v0.2.0

| Component | Tailwind pattern |
|---|---|
| Slide-over panel | `fixed inset-y-0 right-0 w-96 bg-white shadow-xl z-50 flex flex-col` |
| Wide slide-over (Project Stage Settings) | `fixed inset-y-0 right-0 w-[420px] bg-white shadow-xl z-50 flex flex-col` |
| Slide-over backdrop | `fixed inset-0 bg-black/30 z-40` |
| Provider badge — Ollama | `bg-purple-100 text-purple-700 text-xs font-medium rounded-full px-2 py-0.5` |
| Provider badge — LM Studio | `bg-indigo-100 text-indigo-700 text-xs font-medium rounded-full px-2 py-0.5` |
| Provider badge — Anthropic | `bg-orange-100 text-orange-700 text-xs font-medium rounded-full px-2 py-0.5` |
| Provider badge — OpenAI | `bg-green-100 text-green-700 text-xs font-medium rounded-full px-2 py-0.5` |
| Provider badge — Gemini | `bg-blue-100 text-blue-700 text-xs font-medium rounded-full px-2 py-0.5` |
| Provider badge — Claude CLI | `bg-gray-100 text-gray-600 text-xs font-medium rounded-full px-2 py-0.5` |
| Connection test — success | `text-green-600 text-sm font-medium` ("✓ Connected") |
| Connection test — failure | `text-red-600 text-sm font-medium` ("✗ Connection failed: [reason]") |
| Connection test — loading | `text-gray-500 text-sm` + spinner `animate-spin` |
| Error banner (connection fail) | `bg-red-50 border border-red-200 rounded-lg p-4 flex items-start gap-3` |
| Credential warning toast | `fixed bottom-4 right-4 z-50 bg-yellow-50 border border-yellow-300 text-yellow-800 shadow-lg rounded-lg px-4 py-3 text-sm max-w-sm` |
| Inline save confirmation | `text-xs text-green-600 transition-opacity duration-500` |
| Override row highlight | `bg-yellow-50` |
| Deep-link highlight pulse | `ring-2 ring-red-400 animate-pulse` |
| Inherit toggle (ON) | `bg-blue-500 relative inline-flex h-5 w-9 rounded-full transition-colors` |
| Inherit toggle (OFF) | `bg-gray-300 relative inline-flex h-5 w-9 rounded-full transition-colors` |
| Stage override pip accent | `ring-2 ring-yellow-400` (added to existing pip) |
| Coming Soon badge | `bg-gray-100 text-gray-400 text-xs font-medium rounded-full px-2 py-0.5` |
| Gear icon button | `text-gray-500 hover:text-gray-700 p-1 rounded hover:bg-gray-100 transition-colors` |

---

## Traceability

| Journey ID | Screen(s) | Interaction Pattern(s) |
|---|---|---|
| JRN-v0.1.0-001 | SCR-001 | — |
| JRN-v0.1.0-002 | SCR-002, SCR-003 | IACT-001, IACT-003 |
| JRN-v0.1.0-003 | SCR-002 | IACT-002 |
| JRN-v0.1.0-004 | SCR-002 | IACT-004 |
| JRN-v0.1.0-005 | SCR-004 | IACT-001, IACT-003 |
| JRN-v0.1.0-006 | SCR-005 | IACT-001 |
| JRN-v0.1.0-007 | SCR-006 | — |
| JRN-v0.1.0-008 | SCR-001, SCR-002 | IACT-001, IACT-006 |
| JRN-v0.2.0-001 | SCR-001, SCR-007, SCR-008, SCR-009, SCR-010 | IACT-007, IACT-008, IACT-009, IACT-010 |
| JRN-v0.2.0-002 | SCR-008, SCR-009 | IACT-007, IACT-008, IACT-009 |
| JRN-v0.2.0-003 | SCR-008, SCR-009 | IACT-007, IACT-008, IACT-009 |
| JRN-v0.2.0-004 | SCR-008 | — |
| JRN-v0.2.0-005 | SCR-009 | IACT-008 |
| JRN-v0.2.0-006 | SCR-007, SCR-010 | IACT-010 |
| JRN-v0.2.0-007 | SCR-002, SCR-011 | IACT-010, IACT-012 |
| JRN-v0.2.0-008 | SCR-003, SCR-012 | IACT-011 |
| JRN-v0.2.0-009 | SCR-008, SCR-009 | — |