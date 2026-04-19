# Plan — Build Mobile + Wizard Bugfixes

Fix four UX bugs reported by the user.

## Bug 1 — Architecture "Generate" button full-width

**Symptom:** "Generate Architecture" button above the chat input textarea takes full width. User wants it inline beside/below Send, labeled just "Generate".

**File:** `frontend/src/components/chat/ChatPanel.tsx`

**Fix:** Move the generate button into the same flex row as the Send button (right side). Use compact label "Generate".

## Bug 2a — Mobile: Build stage chat input hidden

**Symptom:** On mobile, in Build wizard Chat step, the chat input is hidden.

**Cause hypothesis:** `.build-wizard` is `display: flex; flex-direction: column; overflow: hidden`. On mobile, the inner `.chat-panel` messages may consume all space and push the chat-input out. Also wizard-nav + wizard-steps + viewport bars leaves little room, and `chat-panel` may not be sized properly.

**Fix:** Ensure `.chat-panel` inside `.wizard-content` can shrink and its `.chat-input` remains visible. Add mobile CSS that:
- Constrains `.messages` with `flex: 1; min-height: 0; overflow-y: auto;`
- Keeps `.chat-input` `flex-shrink: 0; position: sticky; bottom: 0;` (or simply ensures the parent flex layout works).

Also: textarea has `rows={2}`, which is fine.

## Bug 2b — Mobile: long-press to select paragraph + refine

**Symptom:** On mobile, text selection doesn't trigger the refinement toolbar.

**Current code:** `ArtifactSelectionToolbar` listens to `mouseup` only.

**Fix:** Add `touchend` listener. Also add long-press detection on the `.artifact-content` element — when user holds a word for ~500ms, programmatically select the entire paragraph (the containing block element) and open the toolbar.

Implementation:
- Add `touchstart` + `touchend` with timer; if held ≥500ms, `preventDefault()` the default selection and select the paragraph containing the touch target (walk up to nearest `p, li, h1-6, blockquote, pre`).
- Use `window.getSelection().selectAllChildren()` or construct a `Range` for the paragraph.
- Trigger `handleSelection` to show toolbar.

## Bug 3 — Build wizard doesn't remember substep

**Symptom:** Navigating away from Build stage and back resets to first step; must click Next many times and re-triggers bead generation.

**Cause:** `App.tsx` calls `setActiveTab('artifact')` on every stage-selection change (line ~312-321), and doesn't restore last position.

**Fix:** Persist last active wizard step per-project in `localStorage` key `paulette.build.wizard.{projectId}`. When entering build stage:
1. Read saved step.
2. Prefer saved step if content exists (beads loaded → still show saved step).
3. Fall back to current heuristic (execute if beads exist, artifact otherwise).

On `setActiveTab` when in build stage, save to localStorage.

## Bug 4 — Implement step shows 2 Paulettes, regenerates, hides errors

**Symptom(s):**
- Two `BuildingAnimation` visible simultaneously on Implement step.
- Clicking Generate Beads when beads exist re-generates them instead of loading.
- Connection failures silent.

**Cause (2 paulettes):** `BuildPanel.tsx` in `mode === 'execute'`:
- Header bar renders `<BuildingAnimation />` when `loading && !hasBeads && !isGenerating` (line 143-145).
- `.execute-graph` also renders `<BuildingAnimation />` for `(isGenerating || loading)` (line 224-228).
When loading with no beads, both render → 2 paulettes.

**Fix:** Remove the header BuildingAnimation; keep only the graph-area one. In the header show a small text status instead. Also, only show animation while actually generating; if just loading existing beads, show nothing (loadGraph is quick).

**Cause (regenerate on revisit):** In `BuildPanel.tsx` line 146-154, when `!hasBeads && !isGenerating && !loading`, it shows a "Regenerate Beads" button. Clicking triggers `handleGenerate()` → `generate()` which always calls `generateBeads()`. `useBeads.loadGraph` already loads existing beads on mount. But if the user navigates to `execute` tab when beads exist, no regen happens (button only shows when `!hasBeads`). So the real issue from user report: "when I go to generate beads, it should not regenerate the beads if they were already generated. Just load them." — On the Generate Beads substep itself, if beads already exist we should not show a regenerate button that triggers re-gen. Instead, show "Beads loaded" + allow navigating to Implement.

**Fix:** 
- `Generate Beads` substep (mode='execute'): if beads already exist, display bead list summary + "Proceed to Implement" action; no Regenerate by default.
- On Implement substep: just display beads (graph on desktop, list on mobile). No auto-regenerate.

**Cause (silent connection error):** `useBeads.execute` only logs to executionLog on error. User needs a visible banner.

**Fix:** Surface errors as a banner in the implement view similar to `plan-limit-banner`. Add `executionError` state to useBeads, show banner when set, with dismiss button.

## Implementation order

1. Create plan dir (done).
2. Bug 1 (ChatPanel) — small.
3. Bug 3 (App.tsx wizard substep persistence) — small.
4. Bug 4 (BuildPanel double paulette + error banner + no-regen) — medium.
5. Bug 2a (CSS for mobile chat-input visibility inside wizard).
6. Bug 2b (ArtifactSelectionToolbar touch handling).
7. `npm run build` in frontend for type check.
8. Commit + push.

## Files to modify

- `frontend/src/components/chat/ChatPanel.tsx`
- `frontend/src/components/build/BuildPanel.tsx`
- `frontend/src/hooks/useBeads.ts`
- `frontend/src/components/artifact/ArtifactSelectionToolbar.tsx`
- `frontend/src/App.tsx`
- `frontend/src/App.css` (mobile wizard chat-input rules)
