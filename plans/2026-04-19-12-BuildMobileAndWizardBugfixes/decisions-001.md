## Decision: Move Generate artifact button inline with Send
ID: DEC-001

- Date: 2026-04-19
- Status: Accepted

- Context:
  On Architecture / UX stages, the "Generate [label]" button spanned the full
  width above the chat textarea. User wanted it smaller, next to Send.

- Options Considered:
  - Option A: Keep full-width banner, relabel only.
  - Option B: Stack Send + Generate in a vertical action column to the right of
    the textarea.

- Decision:
  Option B: render a `.chat-input-actions` column with Send on top and Generate
  below. Label collapses to just "Generate".

- Rationale:
  The chat-input flex-row already aligns textarea + Send. Adding a sibling
  column keeps Generate discoverable but out of the way, and doesn't change the
  width of the textarea.

- Consequences:
  - Positive: compact, matches user expectation.
  - Negative: slight extra markup on every chat input.

- Impact:
  - Affects: `ChatPanel.tsx`, `.chat-input-actions` / `.chat-input-generate` CSS

---

## Decision: Persist build wizard step in localStorage per-project
ID: DEC-002

- Date: 2026-04-19
- Status: Accepted

- Context:
  Build wizard reset to the first populated step every time the user navigated
  away and back, forcing re-clicks through Next and risking re-running bead
  generation.

- Options Considered:
  - Option A: Store in URL hash.
  - Option B: Store in localStorage keyed by project id.
  - Option C: Persist on project record (server round-trip).

- Decision:
  Option B.

- Rationale:
  Local, immediate, no backend change, naturally project-scoped. URL would
  leak into shares; server persistence is overkill for UI cursor state.

- Consequences:
  - Positive: navigating stages preserves wizard progress.
  - Negative: cross-device position not synced (acceptable — this is UI state).

- Impact:
  - Affects: `App.tsx` stage-selection and activeTab persistence effects.

---

## Decision: Surface bead-stream errors as an inline banner
ID: DEC-003

- Date: 2026-04-19
- Status: Accepted

- Context:
  A failed AI connection during bead generate/execute silently appended to the
  executionLog, which is collapsed/hidden by default. Users thought the app
  hung.

- Options Considered:
  - Option A: Toast notification.
  - Option B: Inline banner in the execute view (same pattern as the Claude
    plan-limit banner).

- Decision:
  Option B, re-using the `.plan-limit-banner` styling with an error color so
  the dismiss UX is consistent.

- Rationale:
  Keeps error visible until acknowledged, colocated with the stuck UI, and
  avoids introducing a new notification system.

- Consequences:
  - Positive: clear recovery signal.
  - Negative: two near-identical banners share one CSS class — if styling
    diverges later we'll want to split.

- Impact:
  - Affects: `useBeads.ts` (new `buildError` state), `BuildPanel.tsx` banner.

---

## Decision: Long-press paragraph-selection on mobile artifacts
ID: DEC-004

- Date: 2026-04-19
- Status: Accepted

- Context:
  Mobile users couldn't trigger the refine-with-AI / manual-edit toolbar:
  `mouseup` didn't fire for touch, and precise text selection is hard on a
  phone.

- Options Considered:
  - Option A: Only add `touchend` listener — rely on native word-pick selection.
  - Option B: Add `touchend` + a 500ms long-press that auto-selects the
    enclosing paragraph/list-item/heading.

- Decision:
  Option B.

- Rationale:
  The refinement flow operates best on whole paragraphs; hitting an exact
  sentence on mobile is painful. Long-press-to-grab-block matches the user's
  "user holds a word, bring the paragraph in the edit" request directly.

- Consequences:
  - Positive: viable mobile refine workflow.
  - Negative: may fight iOS's built-in text-selection magnifier; tested with
    `passive: true` on touchstart to avoid blocking scroll.

- Impact:
  - Affects: `ArtifactSelectionToolbar.tsx`.
