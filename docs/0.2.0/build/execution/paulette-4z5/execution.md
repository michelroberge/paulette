Now let me read a couple of the already-implemented configure components to understand the coding patterns:Now I have all the context I need. I'll implement the full `StageDefaultsTab.tsx`, replacing the stub with a complete working component:Now let me verify it compiles correctly by running the TypeScript check:TypeScript compiles cleanly with no errors. Let me also quickly verify the build works:Build succeeds cleanly (the chunk-size warning is pre-existing and unrelated to this task). Here's a summary of what was implemented:

---

## `StageDefaultsTab.tsx` — Implementation Complete

The stub has been replaced with a fully functional component at `frontend/src/components/configure/StageDefaultsTab.tsx`.

### What it does

**Data loading** — on mount, fetches both `listConnections()` and `getGlobalDefaults()` in parallel, then merges them into per-row state (connectionId + model for each of the 5 stages).

**Table layout** — renders a styled `role="table"` with four columns:
- **Stage** — colour-coded pip + label (Vision, UX Design, Architecture, Build, Complete)
- **Connection** — `<select>` dropdown listing all saved connections plus a "Claude CLI (default)" entry at the top
- **Model** — `<select>` if the selected connection has `discoveredModels`, otherwise a free-text `<input>`
- **Status** — fading inline save indicator (per IACT-010)

**Auto-save per row** — connection changes save immediately; model text-input saves on blur; model dropdown saves immediately on selection. Each call sets status → `'saving'` → `'saved'` (fades out after 2s) or `'error'` (shows "Save failed — retry?" link).

**Smart model pre-fill** — switching the connection automatically pre-fills the model field with the connection's `defaultModel`, so the row is immediately valid.

**Scope note** — a footer note explains these are global defaults for new projects and points users to the ⚙ icon for per-project overrides.