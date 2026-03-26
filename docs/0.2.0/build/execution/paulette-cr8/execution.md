Now I have a complete picture of all three issues. Let me implement the fixes:

1. **Dead `discoveredModels` branch** — call `listModels(connectionId)` in `handleConnectionChange` and store results in per-stage local state
2. **Discarded async promise** — prefix all async event handlers with `void`
3. **Race condition on rapid saves** — guard each row's save with a per-stage sequence number ref so stale responses don't overwrite newer stateNow update `saveRow` to use the sequence number guard:Now update `handleConnectionChange` to call `listModels` and populate per-stage model state:Now fix the render section — replace `selectedConn?.discoveredModels` with the local `stageModels` state, and add `void` to all async event handlers:Now add `void` to the async event handlers in the JSX:Also fix the `onRetry` callback in the render:Now let's verify the final state of the file looks correct end-to-end:The file looks exactly right. Let me do a quick type-check build to confirm there are no TypeScript errors:Clean. All three review findings are resolved. Here's a summary of what was changed in `StageDefaultsTab.tsx`:

---

### Fix 1 — `discoveredModels` dead code (permanent → live)

Added a `stageModels` state (`Record<StageName, ModelInfo[]>`) initialised to empty arrays. In `handleConnectionChange`, after clearing the stale models for the stage, `listModels(connectionId)` is called **in the background** (not awaited, so it never blocks the save). If it resolves, the state is updated and the model column transitions from a free-text `<input>` to a `<select>`. If it rejects (provider doesn't support model listing), it silently swallows the error and the free-text input stays — matching the spec's silent fallback (IACT-008). The render now derives `discoveredModels` from `stageModels[stage]` instead of the non-existent `selectedConn?.discoveredModels` field.

### Fix 2 — Discarded async promise

All async event handler call sites in JSX are now prefixed with `void`:
- `onChange={e => void handleConnectionChange(...)}`
- `onChange={e => void handleModelSelect(...)}`
- `onBlur={() => void handleModelBlur(...)}`
- `onRetry={() => void saveRow(...)}`

This makes promise-discarding explicit and intentional rather than an implicit unhandled-rejection risk.

### Fix 3 — Race condition on rapid saves

Added a `saveSeqRef` (`useRef<Record<StageName, number>>`) that acts as a per-row monotonic counter. Each `saveRow` call increments the counter and captures its value (`mySeq`). After the async PUT completes, it checks `saveSeqRef.current[stage] !== mySeq`; if a newer save has started in the meantime, the stale result is silently discarded without applying any state update. This eliminates the possibility of a slow earlier PUT overwriting the result of a fast later PUT on the same row.