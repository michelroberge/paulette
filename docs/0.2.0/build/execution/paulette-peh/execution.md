Now let me look at the StagesSidebar to understand how pips are rendered, and check if there's already some override data we need to fetch:Now let me check the stageConfig API and types:Now I have a clear picture. Let me implement the changes:

1. **`ProjectHeader.tsx`** - Add gear icon button with `onShowStageSettings` prop
2. **`StagesSidebar.tsx`** - Add `stagesWithOverrides` prop and show yellow ring accent
3. **`App.tsx`** - Wire everything together: pass gear handler, load overrides, pass to sidebarNow add the gear button style. Let me check the existing App.css for header button styles:Now add CSS for the gear icon button:Now update `StagesSidebar` to accept and render override indicators:Now update the stage item rendering to show a yellow ring when there's an override:Now add CSS for the override ring indicator. Let me find a good place in App.css near the stage-robot styles:Now update `App.tsx` to wire everything together — pass `onShowStageSettings` to `ProjectHeader`, load override state, pass to `StagesSidebar`, and handle `onOverridesChange`:Now add the `stagesWithOverrides` state and a loader function to `ProjectDetailPage`:Now add a function to load and refresh project overrides. I'll add it alongside the `loadPipeline` function:Now call `loadStageOverrides` when the project loads. Find the existing project load effect:Now pass `onShowStageSettings` to `ProjectHeader` and `stagesWithOverrides` to `StagesSidebar`:Now pass `onOverridesChange` to `ProjectStageSettings` so overrides refresh when the user changes settings:Now let's verify the build compiles cleanly:Build succeeds with no TypeScript errors. Let me also do a quick review of the changes to make sure everything is coherent:Everything looks good. Here's a summary of all the changes made:

---

## Changes Summary

### `frontend/src/components/layout/ProjectHeader.tsx`
- Added `onShowStageSettings?: () => void` prop to the `Props` interface
- Added a `⚙` gear icon button with class `stage-settings-button`, `title="Stage connection settings"`, and `aria-label="Stage connection settings"` — rendered between the Autonomous toggle and the Git history button

### `frontend/src/App.css`
- Added `.stage-settings-button` and `.stage-settings-button:hover` CSS rules (styled similarly to the existing `history-button`)
- Added `.stage-robot-wrapper` and `.stage-robot-wrapper--override` CSS rules — the override variant applies a `2px solid #facc15` (yellow-400) outline accent around stage robot icons that have a project-level connection override

### `frontend/src/components/layout/StagesSidebar.tsx`
- Added `stagesWithOverrides?: Set<StageName>` prop
- Wrapped each `<StageRobot>` in a `<span className="stage-robot-wrapper [--override]">` — the `--override` modifier class is applied when the stage name is in the `stagesWithOverrides` set, with an appropriate tooltip

### `frontend/src/App.tsx`
- Imported `getProjectOverrides` from `./api/stageConfig`
- Added `stagesWithOverrides: Set<StageName>` state
- Added `loadStageOverrides()` callback that fetches project overrides and derives which stages have active non-null overrides
- Called `loadStageOverrides()` alongside `loadPipeline()` when a project loads
- Passed `onShowStageSettings={() => setShowStageSettings(true)}` to `<ProjectHeader>`
- Passed `stagesWithOverrides` to `<StagesSidebar>`
- Passed `onOverridesChange={loadStageOverrides}` to `<ProjectStageSettings>` so the pip accents refresh live as the user changes settings