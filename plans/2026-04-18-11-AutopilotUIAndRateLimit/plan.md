# Plan: Autopilot UI auto-advance + OpenAI-compat exponential backoff

## Issue 1 — Autopilot build wizard stays on "Build Plan"

### Root cause
- Backend orchestrator (`backend/internal/autopilot/orchestrator.go:383-427`, `handleBuildStage`) runs chat → generate beads → execute beads without any "skills" step.
- Frontend build wizard (`frontend/src/components/build/BuildWizardView.tsx`) has 5 steps: `chat → artifact → skills → generate → execute`.
- Only one auto-advance effect exists (`frontend/src/App.tsx:339-345`) and it only moves `chat → artifact`.
- While autopilot is running `beads-generate` / `beads-execute` in the background, the UI stays on `artifact` tab, giving the impression that nothing is happening.

### Fix
In `frontend/src/App.tsx`, add an effect that fires whenever `agentOperation` changes while `selectedStage === 'build'`:

- If `agentOperation === 'beads-generate'` and `activeTab` is `artifact` or `skills` → `setActiveTab('generate')`.
- If `agentOperation === 'beads-execute'` and `activeTab` is `artifact`, `skills`, or `generate` → `setActiveTab('execute')`.

This bypasses the `skills` step in autopilot (skills analysis stays user-initiated via the "Analyze Build Plan" button on the Skills tab when the user visits it manually).

## Issue 2 — OpenAI-compat rate-limit handling (Groq etc.)

### Current behavior
- `backend/internal/provider/openai_compat.go:186-198` — streaming chat retries 429s with linear backoff `15s * (attempt+1)`, max 5 attempts. No `Retry-After` honored.
- `doOpenAIRound` (line ~505) used by the agent/tool-use loop has **no** 429 retry at all — a single 429 surfaces immediately as an error.

### Fix
Introduce a shared helper `waitForRetry(ctx, attempt, resp) -> bool` that:

1. Reads `Retry-After` header (seconds or HTTP-date); if present and ≤ 60s, uses it.
2. Otherwise uses exponential backoff: `min(2^attempt, 60)` seconds — so 1, 2, 4, 8, 16, 32, 60, 60…
3. Sleeps via `select { <-ctx.Done(); <-time.After(…) }`.
4. Returns `false` if the context was cancelled or the retry budget is exhausted.

Apply it in:
- `openAICompatChat` — replace current linear-backoff block, bump `maxRetries` to 8 so total wait is bounded by ~2 minutes worst-case.
- `doOpenAIRound` — wrap the request in the same retry loop; must rebuild the `*http.Request` each iteration because the body reader is consumed.

## Files touched
- `frontend/src/App.tsx` — add auto-advance effect.
- `backend/internal/provider/openai_compat.go` — add retry helper, use in chat + agent round.

## Out of scope
- Bead execution's own backend retry (separate path).
- Anthropic/Gemini/Ollama providers — they have their own error shapes; only user-reported issue is OpenAI-compat (Groq).
- Any changes to the Skills analysis flow itself.

## Validation
- Backend: `go build ./...` and `go vet ./...` clean.
- Frontend: `npm run build` clean.
- Manual: not exercised in this session (requires a running stack + Groq key + real autopilot project).
