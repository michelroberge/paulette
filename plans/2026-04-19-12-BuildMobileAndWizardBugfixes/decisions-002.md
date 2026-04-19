## Decision: BeadDetailPanel resilience against hung detail fetch
ID: DEC-005

- Date: 2026-04-19
- Status: Accepted

- Context:
  After the initial fix round, the user reported that clicking a bead on the
  graph showed a permanent "Loading..." placeholder — the request neither
  surfaced an error nor rendered the panel. Root cause: `useBeadDetail.loadDetail`
  only toggled `loading` to false in a `finally` block, which never executes if
  the fetch promise is still pending. The panel gates the whole body on
  `loading && !detail`, so a hung request blocks the UI forever with no signal.

- Options Considered:
  - Option A: Render the panel body unconditionally and show a subtle loader
    in place of lazy-loaded fields — relying on the `bead` prop for the
    primary data.
  - Option B: Keep the gating spinner but add a client-side timeout with a
    visible error/retry banner.
  - Option C: Both.

- Decision:
  Option B (timeout + error/retry banner) plus a new `loadError` state exposed
  through the hook. 15-second timeout clears `loading` and sets `loadError`.
  The panel shows a red banner (reusing `.plan-limit-banner`) with a Retry
  button that re-invokes `loadDetail(beadId)`.

- Rationale:
  Option A would require restructuring every conditional that uses `detail?.*`
  with defaults; risk of regressions. Option B is minimal and gives users
  actionable recovery without a reload.

- Consequences:
  - Positive: no more silent permanent spinner; user sees either success,
    error, or explicit timeout.
  - Negative: 15s fixed timeout may be too short on very slow networks — tune
    if we see complaints.

- Impact:
  - Affects: `useBeadDetail.ts`, `BeadDetailPanel.tsx`.
