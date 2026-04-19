# Summary: Autopilot UI auto-advance + OpenAI-compat exponential backoff

## What shipped

### 1. Build wizard now follows the orchestrator
`frontend/src/App.tsx` — new effect keyed on `agentOperation`/`agentActive`/`selectedStage`:
- `beads-generate` active & tab in {`artifact`, `skills`} → jump to `generate`.
- `beads-execute` active & tab in {`artifact`, `skills`, `generate`} → jump to `execute`.

Skills step is skipped automatically in autopilot but remains usable outside it.

### 2. 429 resilience for OpenAI-compat providers (Groq etc.)
`backend/internal/provider/openai_compat.go`:
- Added `waitForRateLimitRetry` / `rateLimitRetryDelay` helpers.
- Exponential backoff: `min(2^attempt, 60)` seconds, honoring `Retry-After` (delta-seconds or HTTP-date, also capped at 60s).
- `rateLimitMaxRetries` = 8.
- `openAICompatChat`: replaced previous linear backoff loop.
- `doOpenAIRound` (agent/tool-use loop): gained a retry loop it previously lacked.

## Validation
- `go build ./... && go vet ./...` in `golang:1.26-bookworm` container — clean (pre-existing vet noise in unrelated files).
- `npm run build` in `node:22-bookworm` — clean (tsc + vite).
- Manual end-to-end not exercised in this session.

## Files touched
- `frontend/src/App.tsx`
- `backend/internal/provider/openai_compat.go`
