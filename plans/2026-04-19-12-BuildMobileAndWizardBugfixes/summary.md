# Summary — Build Mobile + Wizard Bugfixes (2026-04-19)

## What changed

- **Architecture/UX Generate button** is now a compact pill in a side-column
  next to Send, labeled "Generate" instead of a full-width banner.
  (`ChatPanel.tsx`, `App.css`)
- **Build wizard step persistence**: per-project localStorage restores the
  last-visited substep when re-entering the Build stage, so users no longer
  re-walk through Next and risk regenerating beads.
  (`App.tsx`)
- **Implement step**: removed the duplicate dancing-Paulette animation
  (header had one, graph area had one). Header now shows a "Loading beads…" /
  "N beads loaded" status. `BuildingAnimation` in the graph only shows while
  actually generating.
  (`BuildPanel.tsx`)
- **Bead error surfacing**: `useBeads` now tracks `buildError` (populated on
  stream `error` events and thrown exceptions). `BuildPanel` renders an
  inline red banner reusing the plan-limit-banner layout, with dismiss.
  (`useBeads.ts`, `BuildPanel.tsx`)
- **No auto-regeneration**: the artifact-mode button label and flow no longer
  suggests "Regenerate Beads" when none exist; says "Generate Beads". With
  beads present, loadGraph just loads — regen only happens on explicit click.
- **Mobile chat input visibility**: added `flex-shrink: 0` on `.chat-input`,
  tuned `.wizard-content` / `.wizard-steps` / `.wizard-nav` for small screens
  so the input never gets pushed off-screen inside the Build wizard.
  (`App.css`)
- **Mobile artifact refinement**: `ArtifactSelectionToolbar` now listens to
  `touchend` + `selectionchange` in addition to `mouseup`, and implements a
  500ms long-press that auto-selects the enclosing paragraph / list-item /
  heading and opens the toolbar.
  (`ArtifactSelectionToolbar.tsx`)

- **BeadDetailPanel stuck on "Loading..."**: follow-up fix. `useBeadDetail`
  now guards `loadDetail` with a 15s timeout, exposes a `loadError` state,
  and the panel renders a dismissible error banner with a Retry button
  instead of a permanent spinner.
  (`useBeadDetail.ts`, `BeadDetailPanel.tsx`)

## Files touched

- `frontend/src/components/chat/ChatPanel.tsx`
- `frontend/src/components/build/BuildPanel.tsx`
- `frontend/src/components/build/BeadDetailPanel.tsx`
- `frontend/src/components/artifact/ArtifactSelectionToolbar.tsx`
- `frontend/src/hooks/useBeads.ts`
- `frontend/src/hooks/useBeadDetail.ts`
- `frontend/src/App.tsx`
- `frontend/src/App.css`

## Validation

- `npx tsc -b` clean (via docker node:20).
- `npx vite build` succeeds.
