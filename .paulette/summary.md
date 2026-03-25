# Iteration Summary: Paulette v0.2.0

## Product Overview
Paulette is a structured five-stage AI-assisted software development pipeline tool (Vision → UX → Architecture → Build → Complete) for solo founders, product-minded developers, and AI-first small teams. v0.1.0 hardwired the pipeline to the Claude CLI, creating a barrier for teams with existing inference infrastructure. v0.2.0 introduces a **pluggable LLM provider layer** that makes the pipeline model-agnostic, enabling users to route each pipeline stage to any supported inference backend (local Ollama/LM Studio, Anthropic API, OpenAI, Gemini, or the existing Claude CLI) while preserving 100% backward compatibility for existing Claude CLI users.

---

## Key UX Decisions

- **"Configure Paulette" button** placed prominently on the home page alongside "New Project" and "Import Project" — not buried in a settings menu — because first-time provider setup is a critical onboarding path for new user segments.
- **`/configure` page** uses a two-tab layout: **Connections** (CRUD for named provider connections) and **Stage Defaults** (per-stage connection + model assignment).
- **Slide-over panels** used for Connection Form (w-96) and Project Stage Settings (w-420px) to avoid navigating away from context.
- **Provider-type-driven conditional fields**: selecting a provider type synchronously shows/hides relevant fields (URL, API key, org/project ID) with no loading state — pure React controlled-component pattern.
- **Auto-save with inline "Saved ✓" confirmation** on Stage Defaults and Project Stage Settings rows — eliminates "did I save?" anxiety, no global Save button needed.
- **Opportunistic model discovery**: Test Connection simultaneously probes connectivity AND fetches model list; silently falls back to free-text input if model list unavailable — never blocks save.
- **Inherit toggle per stage** in Project Stage Settings: ON = read-only muted global default; OFF = editable with global value pre-populated as starting point. Yellow `bg-yellow-50` row tint + `ring-2 ring-yellow-400` pip accent signal active overrides.
- **Inline connection error banner** in the Chat tab replaces streaming area on failure — no silent hangs. Includes deep-linked "Go to Configure →" (with `?highlight=[connection-id]` causing a 3-second pulse) and "Change stage connection" shortcut to reassign without navigating away.
- **GitHub Copilot** visible but disabled in provider dropdown with "Coming Soon (v0.3.0)" badge — sets expectations without hiding the planned feature.
- **Credential warning toast** (yellow, fixed bottom-right, auto-dismisses at 6s) fires whenever API keys are written to disk; not shown for credential-free providers (Ollama, LM Studio, Claude CLI).

---

## Architecture Summary

**Tech Stack:** Go 1.26 backend (chi v5 router), React 19 + TypeScript 5.9 frontend, Vite 8, Tailwind CSS 4.x. **No new dependencies added** — all provider integrations use Go stdlib `net/http` + `encoding/json`.

**New backend package: `provider/`**
- `provider.go` — Core `Provider` interface (`Chat`, `TestConnection`, `ListModels`) + `Registry` with `ForConnection()` and `ResolveForStage()` (3-level fallback: project override → global default → Claude CLI hardcoded).
- `connection_store.go` — CRUD for `~/.paulette/connections.json` with atomic writes + `chmod 600`.
- `stage_config.go` — Global defaults (`~/.paulette/config.json`) and per-project overrides (`<hostDir>/.paulette/stage_config.json`).
- `claude_cli.go` — Thin adapter wrapping existing `agent.Chat()` behind the interface.
- `ollama.go` — NDJSON streaming via `POST /api/chat`; model list via `GET /api/tags`.
- `openai_compat.go` — Shared SSE parsing for OpenAI-compatible APIs (used by both LM Studio and OpenAI providers).
- `lmstudio.go`, `openai.go`, `anthropic.go`, `gemini.go` — Provider-specific implementations.

**Modified handlers:** `chat.go` and `mock.go` replace direct `agent.Chat()` calls with `registry.ResolveForStage()` → `provider.Chat()`; connection failures emit structured `StreamEvent{Type: "error"}`.

**New API surface (17 endpoints):**
- Connection CRUD: `GET/POST /api/connections`, `GET/PUT/DELETE /api/connections/:id`, `POST /api/connections/:id/test`, `POST /api/connections/test`, `GET /api/connections/:id/models`, `POST /api/connections/models`
- Stage config: `GET/PUT /api/config/stages/:stage`, `GET/PUT /api/projects/:id/config/stages/:stage`, `POST /api/projects/:id/config/stages/reset`

**Key data model additions:**
- `Connection` struct with `ProviderType`, `BaseURL`, `APIKey` (stored plain text), `OrgID`, `ProjectID`, `DefaultModel`.
- `ConnectionResponse` (API-facing) redacts credentials to `HasCredentials bool`.
- `StageAssignment{ConnectionID, Model}` used in both global config and project overrides.
- `ProjectStageConfig.Overrides` uses `*StageAssignment` where `null` = inherit.

**Streaming format normalization:** Each provider converts its native format (Ollama NDJSON, OpenAI SSE, Anthropic typed SSE, Gemini NDJSON array) to the shared `StreamEvent{Type, Content}` channel. HTTP client timeouts: 30s for connection test, 10min for chat streaming.

**File persistence layout additions:**
```
~/.paulette/connections.json        (chmod 600)
~/.paulette/config.json             (global stage defaults)
<hostDir>/.paulette/stage_config.json  (per-project overrides)
```

**Frontend additions:** React Router added for `/`, `/projects/:id`, `/configure` routes. New components: `ConfigurePage`, `ConnectionsTab`, `ConnectionForm`, `StageDefaultsTab`, `ProjectStageSettings`, `ConnectionErrorBanner`, `CredentialWarningToast`. New API clients: `api/connections.ts`, `api/stageConfig.ts`. New types: `types/provider.ts`.

---

## Build Strategy

**6 milestones** organized around the critical dependency chain:

1. **M1 — Provider Interface & Core Infrastructure** (greenfield `provider/` package): interface, connection model, ConnectionStore, StageConfigStore, Claude CLI adapter.
2. **M2 — LLM Provider Implementations** (can parallelize individual providers): OpenAI-compat base, then Ollama, LM Studio, OpenAI, Anthropic, Gemini + registry wiring.
3. **M3 — Backend API Handlers & Integration**: connection/config handlers, route mounting, main.go wiring, chat/mock handler modification.
4. **M4 — Frontend Foundation & Routing** (can start in parallel with M1–M3): React Router setup, TypeScript types, API clients, home page button addition.
5. **M5 — Frontend Configure UI** (depends on M4): all new components built in sequence (page shell → tabs/form → panel/toast → chat integration).
6. **M6 — Integration Testing & Polish**: regression validation, Ollama-only pipeline, mixed-provider routing, error recovery, cascade deletion, chmod verification, autonomous mode.

**Critical path:** M1 → M2 → M3.5 (chat handler) → M6.1 (regression test). M1 and M4 can start simultaneously.

---

## Suggested Enhancements

1. **Encrypted credential storage / OS keychain integration** — v0.2.0 stores API keys in plain-text JSON (chmod 600 with a visible warning). Integrating OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager) via a library like `zalando/go-keyring` would eliminate the most significant security gap without changing the UX model. High impact for enterprise and security-conscious users.

2. **GitHub Copilot OAuth device-code flow (P3 → v0.3.0)** — Already designed in the UX (journey JRN-v0.2.0-009, disabled UI slot). The backend needs a device-code OAuth poller, token storage in `connections.json`, and token refresh logic. Unlocks the enterprise developer segment that was a primary motivation for v0.2.0.

3. **Response format resilience / lenient artifact parser** — The system prompts require XML envelope responses (`<response><artifact>...</artifact></response>`), but smaller local models (Ollama, LM Studio) may not reliably comply. Adding a lenient fallback parser in `agent/parse.go` that attempts to extract artifact content even from malformed responses would dramatically improve the Ollama/LM Studio user experience without changing the happy path.

4. **Per-stage provider connection health indicator** — The Stage Defaults tab and stage navigator currently don't show whether assigned connections are reachable. Adding a lightweight background health check (or on-demand ping) with a green/red/grey status dot next to each stage assignment would let users catch broken configurations before starting a pipeline run, reducing the jarring mid-pipeline error experience.

5. **Provider-level rate limiting, cost estimation, and token-budget display** — The token meter currently exists per-stage but doesn't account for provider-side pricing differences. Extending the token model with approximate cost-per-token for known providers (OpenAI, Anthropic, Gemini) and surfacing cumulative cost estimates alongside token counts would support the "cost-conscious builders" persona who want to optimize spend across stages.

---

## Known Limitations

- **Plain-text credential storage**: API keys written to `~/.paulette/connections.json` with no encryption; only file permission (`chmod 600`) as protection. No keychain integration in v0.2.0 (deferred).
- **GitHub Copilot not functional**: Provider type exists in the data model and UI (as "Coming Soon") but the OAuth device-code flow is deferred to v0.3.0.
- **No model capability validation**: Paulette cannot verify whether a configured model can actually follow the XML response envelope format required by the pipeline parser. Compliance with small/uncensored models is the user's responsibility.
- **No provider-level rate limiting, failover, or cost tracking**: If a provider call fails mid-pipeline (not a connectivity error, but a 429 or 5xx), there is no automatic retry or failover to a backup connection.
- **No multi-user or cloud deployment**: All config and credential files are local and user-scoped; no server-side credential management.
- **CORS restricted to localhost**: External deployment not supported in this iteration.
- **Copilot listed as coming soon**: Not an actionable connection type in v0.2.0 UI.