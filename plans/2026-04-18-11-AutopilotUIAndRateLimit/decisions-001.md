## Decision: Auto-advance build wizard past Skills in autopilot mode
ID: DEC-011-001

- Date: 2026-04-18
- Status: Accepted

- Context:
  In auto mode the orchestrator (`handleBuildStage`) runs chat → generate beads → execute beads, but the build wizard in `App.tsx` has a 5-step UI (chat → artifact → skills → generate → execute) and only one auto-advance rule (chat → artifact). Users on the "Build Plan" tab watched nothing happen while the backend was actually building, because the UI never followed the orchestrator forward.

- Options Considered:
  - Option A: Drop the Skills wizard step entirely.
  - Option B: Auto-advance the wizard based on the current `agentOperation` (`beads-generate`, `beads-execute`) only when autopilot is active.
  - Option C: Have the backend emit an explicit UI-step hint that the frontend follows.

- Decision:
  Option B.

- Rationale:
  Skills analysis is still a useful manual step outside autopilot; we don't want to remove it. The frontend already polls active runs and exposes `agentOperation`, so a small effect that reacts to the two bead operations is minimal and self-contained. Option C would require a backend protocol change for no user-visible benefit.

- Consequences:
  - Positive:
    - Auto mode now visibly tracks backend progress through the wizard.
    - Skills step remains available for manual use.
  - Negative:
    - If the user intentionally opens the Skills tab while autopilot is running, the effect will override their navigation. Acceptable because autopilot owns the flow while active.

- Impact:
  - Affects: `frontend/src/App.tsx` (build wizard effect).
  - Related Plans: 2026-04-18-11-AutopilotUIAndRateLimit

- References:
  - `backend/internal/autopilot/orchestrator.go:383` (`handleBuildStage`)


## Decision: Exponential backoff with Retry-After for OpenAI-compat 429s
ID: DEC-011-002

- Date: 2026-04-18
- Status: Accepted

- Context:
  The OpenAI-compat provider handles 429s from upstream APIs but with linear 15s×(n+1) backoff and only on the streaming chat path. Providers like Groq issue aggressive rate limits and include a `Retry-After` header. The agent/tool-use loop (`doOpenAIRound`) had no 429 handling at all.

- Options Considered:
  - Option A: Keep linear backoff, add it to `doOpenAIRound`.
  - Option B: Exponential backoff capped at 60s, honor `Retry-After` header, apply to both chat and agent rounds.
  - Option C: Use a third-party retry library (e.g. cenkalti/backoff).

- Decision:
  Option B.

- Rationale:
  Exponential 1→2→4→8→16→32→60 reduces unnecessary wait on the first retry (usually the rate limit clears fast) while capping worst-case wait at 60s per attempt. Honoring `Retry-After` lets the server be authoritative when it knows better. Option A keeps a documented flaw; Option C adds a dependency for a tiny helper.

- Consequences:
  - Positive:
    - Groq-style rate limits recover automatically on both chat and agent paths.
    - Server-directed `Retry-After` is respected (also capped at 60s to avoid pathological waits).
  - Negative:
    - Max retry budget grew from 5 to 8 attempts; worst-case wall-clock per request up to ~4 min. Mitigated by ctx cancellation at every sleep.

- Impact:
  - Affects: `backend/internal/provider/openai_compat.go`.
  - Related Plans: 2026-04-18-11-AutopilotUIAndRateLimit

- References:
  - MDN: HTTP Retry-After
  - Groq rate-limit docs
