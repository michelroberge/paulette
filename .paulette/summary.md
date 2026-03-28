# Iteration Summary: Paulette v0.2.0

## Product Overview
Paulette is a structured five-stage AI-assisted product development pipeline (Vision → UX → Architecture → Build → Complete) for solo founders, product-minded developers, and AI-first small teams. v0.1.0 hard-wired the pipeline to the Claude CLI, creating a barrier for teams with existing inference infrastructure. v0.2.0 introduces a **pluggable LLM provider layer** that decouples the pipeline logic from any single model backend, enabling users to route each stage to Ollama, LM Studio, OpenAI, Anthropic API, Gemini, or the legacy Claude CLI — including mixing providers across stages for cost/capability optimization. The Claude CLI remains fully functional as one entry in the provider list; existing projects require zero reconfiguration.

---

## Key UX Decisions

- **"Configure Paulette" button** is placed prominently on the home page in the header card with the paulette ASCII drawing and title.
- **Two-tab Configure page** (`/configure`): "Connections" tab for CRUD on named connection records; "Stage Defaults" tab for assigning connections to each of the five pipeline stages globally.
- **Per-project stage overrides** are accessible via a ⚙ gear icon in the project header, opening a slide-over panel with per-stage "Inherit" toggles. Stage pips with project-level overrides show a `ring-2 ring-yellow-400` accent.
- **Provider-type-driven form fields**: selecting a provider type in the Connection Form instantly shows/hides relevant fields (Base URL, API Key, Org/Project ID) with no round-trip — pure React controlled-component pattern. Hidden fields are cleared from state.
- **Opportunistic model discovery**: clicking "Test Connection" simultaneously probes connectivity and attempts a model-list fetch. On success, the model field upgrades from free-text to a populated dropdown. Failure silently falls back to free-text — non-blocking.
- **Auto-save with inline confirmation** in Stage Defaults and Project Stage Settings: changes persist on blur/change per row with a fading "Saved ✓" label; no global Save button required.
- **Inline connection error banner** (not a silent hang): on provider failure during streaming, a red banner appears in the Chat tab with the error reason, a "Go to Configure →" deep-link (with `?highlight=[connection-id]`), and a "Change stage connection" shortcut that opens the Project Stage Settings panel without navigating away.
- **Credential warning toast**: fixed bottom-right, auto-dismisses after 6 seconds, shown only when API keys are saved (not for Ollama/LM Studio/Claude CLI).
- **GitHub Copilot** listed in provider dropdown as "Coming Soon (v0.3.0)" — visible but disabled.
- **Navigation flow**: React Router added with three routes: `/` (home), `/projects/:id` (project detail), `/configure` (configure page). Deep-link support via `?tab=` and `?highlight=` query params.

---

## Architecture Summary

**Tech Stack:** Go 1.26 backend (chi v5 router), React 19 + TypeScript 5.9 frontend, Vite 8, Tailwind CSS 4.x. No new Go or npm dependencies added — all provider integrations use Go stdlib `net/http` + `encoding/json`.

**New backend packages (`provider/`):**
- `provider.go` — `Provider` interface (`Chat`, `TestConnection`, `ListModels`), `Registry` with `ForConnection()` and `ResolveForStage()` (3-level fallback: project override → global default → Claude CLI hardcoded)
- `connection.go` — `Connection`, `ConnectionResponse` (credentials redacted), `ProviderType` constants
- `connection_store.go` — CRUD with atomic writes + `chmod 600` on `~/.paulette/connections.json`; UUID assignment; `Delete` returns affected stage assignments
- `stage_config.go` — Global defaults (`~/.paulette/config.json`) and per-project overrides (`<hostDir>/.paulette/stage_config.json`)
- `claude_cli.go` — Thin adapter wrapping existing `agent.Chat()` behind the `Provider` interface
- `openai_compat.go` — Shared SSE-based chat/model-list/test implementation reused by OpenAI and LM Studio
- `ollama.go` — NDJSON streaming via `POST /api/chat`; model list via `GET /api/tags`
- `lmstudio.go` — Delegates to `openai_compat` functions; no auth
- `openai.go` — Delegates to `openai_compat`; `Authorization: Bearer` + optional org/project headers
- `anthropic.go` — Typed SSE event parsing (`content_block_delta`, `message_stop`); hardcoded model list
- `gemini.go` — NDJSON array streaming via `streamGenerateContent`; API key as query param

**New HTTP handlers:**
- `handler/connection.go` — 9 endpoints: CRUD + Test (saved/unsaved) + ListModels (saved/unsaved)
- `handler/config_handler.go` — 5 endpoints: global defaults + per-project overrides + reset

**Modified handlers:**
- `handler/chat.go` and `handler/mock.go` — Replace direct `agent.Chat()` with `registry.ResolveForStage()` → `provider.Chat()`; connection errors emit structured `StreamEvent{Type: "error"}`

**New frontend components:**
- `components/configure/ConfigurePage.tsx`, `ConnectionsTab.tsx`, `ConnectionForm.tsx`, `StageDefaultsTab.tsx`, `ProjectStageSettings.tsx`, `CredentialWarningToast.tsx`
- `components/chat/ConnectionErrorBanner.tsx`
- `api/connections.ts`, `api/stageConfig.ts`, `types/provider.ts`

**API surface additions (all new, no existing endpoints changed):**

| Group | Endpoints |
|---|---|
| Connections | `GET/POST /api/connections`, `GET/PUT/DELETE /api/connections/:id`, `POST /api/connections/:id/test`, `POST /api/connections/test`, `GET /api/connections/:id/models`, `POST /api/connections/models` |
| Stage Config | `GET/PUT /api/config/stages/:stage`, `GET/PUT /api/projects/:id/config/stages/:stage`, `POST /api/projects/:id/config/stages/reset` |

**Data model highlights:**
- `Connection` struct stores credentials in plain text in `connections.json` (file protected by `chmod 600`, no keychain)
- API responses use `ConnectionResponse` which replaces `APIKey` with `HasCredentials: bool`
- `StageAssignment` — `{connectionId, model}` pair stored in global `config.json` and per-project `stage_config.json`
- Per-project `stage_config.json` uses `*StageAssignment` (nil = inherit global)

**File layout additions:**
```
~/.paulette/connections.json      (chmod 600)
~/.paulette/config.json
<hostDir>/.paulette/stage_config.json
```

**Streaming compatibility matrix:**

| Provider | Protocol | Auth |
|---|---|---|
| Ollama | NDJSON | None |
| LM Studio | SSE | None |
| OpenAI | SSE | `Authorization: Bearer` |
| Anthropic | Typed SSE | `x-api-key` header |
| Gemini | NDJSON array | `?key=` query param |
| Claude CLI | subprocess stdio | Local auth |

All providers wrap the same XML-envelope system prompt (`<!-- RESPONSE:START --><discussion>...<artifact>...`) required by `agent/parse.go` — no per-provider prompt variation.

**HTTP client timeouts:** 30s for connection tests; 10min for chat streaming.

---

## Build Strategy

Six milestones, designed for parallelism:

1. **M1 — Provider Interface & Core Infrastructure** (foundation; blocks all backend work): `Provider` interface, `ConnectionStore`, `StageConfigStore`, Claude CLI adapter
2. **M2 — LLM Provider Implementations** (parallel tasks per provider): `openai_compat` shared base → Ollama, LM Studio, OpenAI, Anthropic, Gemini; then wire `Registry.ForConnection()`
3. **M3 — Backend API Handlers & Integration** (depends on M1+M2): connection handler, config handler, route mounting, `main.go` init, chat/mock handler refactor
4. **M4 — Frontend Foundation & Routing** (parallel to M1–M3): React Router setup, TypeScript types, API clients, "Configure Paulette" button on home page
5. **M5 — Frontend Configure Page & Components** (depends on M4): all new UI components in dependency order (page shell → tabs/form → panel/toast → error banner → chat integration)
6. **M6 — Integration Testing & Polish**: Claude CLI regression, Ollama-only pipeline, mixed provider routing, error recovery, deletion cascade, file permissions, autonomous mode

**Critical path:** M1 → M2 → M3.5 (chat handler modification) → M6.1 (regression test)

**Test environment:** Ollama at `http://10.0.0.57:11434` with `gemma3:4b` (Vision/UX/Complete) and `codellama:7b` (Architecture/Build).

---

## Suggested Enhancements

1. **Encrypted credential storage (OS keychain integration)** — Currently credentials are stored in plain-text JSON with only file-permission protection. Integrating with OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager) via a library like `zalando/go-keyring` would eliminate the `chmod 600` warning and significantly reduce credential exposure risk. High security impact, especially for multi-user or shared machines.

2. **Lenient XML-envelope parser / model capability fallback** — Smaller models (Ollama `gemma3:4b`, Gemini small variants) frequently fail to follow the strict `<!-- RESPONSE:START --><artifact>...</artifact><!-- RESPONSE:END -->` format that `agent/parse.go` requires, silently breaking artifact extraction. Adding a lenient parsing mode (e.g., treat all content as artifact body if no envelope detected) or a per-provider response format setting would dramatically improve reliability with local/small models — the primary new user segment unlocked by v0.2.0.

3. **GitHub Copilot OAuth device-code flow (P3 → delivery)** — This was explicitly deferred to v0.3.0 but is already stubbed in the UI as "Coming Soon." The device-code flow is well-specified (RFC 8628) and the GitHub OAuth endpoints are public. Completing this unlocks the enterprise developer segment (one of three explicit target user types) and fulfils the promised v0.3.0 feature.

4. **Automatic provider failover / retry** — Currently, a connection failure during streaming surfaces an error and stops. Adding optional fallback logic (e.g., "if primary connection fails, try fallback connection X") would improve pipeline resilience for autonomous/hands-free mode, where an unattended failure halts the entire run. Could be a simple per-stage "fallback connection" field in the Stage Defaults UI.

5. **Provider-level cost and token tracking** — The token meter (existing) only counts tokens for the active session. With multi-provider support, users will want per-provider cost visibility (especially when mixing cheap local models with expensive cloud APIs). Adding per-connection token tallying to the activity log and surfacing estimated cost (using published provider pricing as configuration) would directly serve the "cost-conscious builder" target persona.

---

## Known Limitations

- **Credentials stored in plain text** — `~/.paulette/connections.json` is `chmod 600` but not encrypted; no keychain integration. A visible warning is displayed in the UI when credentials are saved.
- **No model capability validation** — Paulette cannot verify whether an assigned model can reliably follow the XML response format or handle complex architecture/build prompts. This is explicitly the user's responsibility.
- **GitHub Copilot not functional** — Listed as "Coming Soon" in the provider dropdown; OAuth device-code flow targeted for v0.3.0.
- **No provider-level rate limiting, cost tracking, or token-budget enforcement** — All out of scope for v0.2.0.
- **No automatic failover** — If a provider call fails, the pipeline stops; no retry or fallback to an alternate connection.
- **CORS still restricted to localhost** — No cloud deployment or multi-user support.
- **XML envelope compliance not enforced** — Small/local models may not follow the structured response format; `agent/parse.go` has no lenient fallback.
- **No CI/CD integration hooks, plugin system, or non-provider extensibility** — Carried forward from v0.1.0 deferral list.
- **Gemini streaming format complexity** — NDJSON array format is unusual and may have edge cases with partial JSON chunks in long responses.