# Product Vision: Paulette v0.2.0

## Problem Statement [ENHANCED]
Software builders lose context, coherence, and momentum when switching between tools. Paulette solves this by enforcing a structured five-stage pipeline (Vision → UX → Architecture → Build → Complete) where every stage is traceable to the one before it.

In v0.1.0, this pipeline was hard-wired to the Claude CLI. This creates a real barrier for teams that already have working inference infrastructure (local Ollama rigs, enterprise Copilot licences, OpenAI API access, Gemini credits) and don't want to add another mandatory dependency. It also locks out cost-optimisation: running a fast local model for lightweight stages while reserving a powerful cloud model for Architecture and Build is a natural pattern that the current design prevents.

v0.2.0 adds a **pluggable LLM provider layer** — so Paulette's pipeline logic becomes model-agnostic, and users can route each stage to whatever inference backend suits them.

---

## Target Users
Solo founders, product-minded developers, and AI-first small teams building software with AI assistance. Specifically expanded in v0.2.0:

- **Enterprise developers** who have GitHub Copilot through their employer and want to use that entitlement instead of a personal Claude subscription
- **Local-first power users** running Ollama or LM Studio on capable hardware who want zero cloud dependency
- **Cost-conscious builders** who want cheap/fast models for early stages and expensive/powerful models only where it matters (Architecture, Build)

---

## Core Features [ENHANCED]

### Existing (v0.1.0 baseline — unchanged)
- Five-stage linear pipeline with human approval gates at every stage transition
- Stage-gated navigation with colour-coded status pips
- Chat + Artifact tabs per stage; stage-specific extras (Mock Preview, Execute)
- Approve-to-advance with autonomous / hands-free mode
- Bead DAG visualisation for build task dependency tracking
- Version history, git integration, rollback
- Project import (reverse-engineer existing codebases)
- Mobile-responsive layout

### [NEW] LLM Provider Abstraction Layer

**Provider support — phased:**

| Priority | Providers | Auth method |
|---|---|---|
| P1 | Ollama, LM Studio | Base URL only (no credentials) |
| P2 | Anthropic API (direct), OpenAI, Gemini | API key (+ optional org/project ID for OpenAI) |
| P3 | GitHub Copilot | GitHub OAuth device-code flow |

The existing **Claude CLI subprocess** is retained as a valid provider ("Claude CLI") and continues to work exactly as today — it is simply one entry in the provider list rather than the hardwired default.

**Connections:**
Each connection is a named, reusable record describing how to reach an inference backend:
- `name` — user-defined label (e.g. "Work Copilot", "Local Llama3", "OpenAI GPT-4o")
- `provider_type` — one of the supported provider types above
- `base_url` — required for Ollama and LM Studio; optional override for others
- `credentials` — API key (P2), OAuth token (P3), or empty (P1 / Claude CLI)
- `model` — default model for this connection (can be overridden per-stage assignment)
- Model discovery: if the provider exposes a model-list endpoint, Paulette fetches it and renders a dropdown; otherwise a free-text field is shown
- **Test Connection** button: sends a minimal probe request and reports success/failure inline

**Stage–Connection Mapping (two-level hierarchy):**
- **Global defaults** — stored in `~/.paulette/config.json`; each of the five pipeline stages can be assigned a connection + model; new projects inherit these defaults
- **Per-project overrides** — stored in the project's registry JSON; same structure as global defaults but scoped to that project; a "Reset to global defaults" action is available

**Credential storage:**
- All connection data (including API keys) is written to `~/.paulette/connections.json` on disk as JSON
- File is readable only by the owning user (`chmod 600` applied on write)
- No keychain / secret-manager integration in v0.2.0; a visible warning is shown when credentials are saved

### [NEW] Configure Paulette Setup Page

Accessible from the home page via a **"Configure Paulette"** button. Two-tab layout:

**Tab 1 — Connections**
- List of all defined connections with provider-type badge, name, and model
- Add / Edit / Delete actions per connection
- Per-connection: Test Connection button with inline status
- Provider-type selector drives which fields appear (URL field, credential field, OAuth flow button)

**Tab 2 — Stage Defaults**
- Table of the five pipeline stages
- Per stage: connection picker (dropdown of named connections) + model field (dropdown if discoverable, free-text fallback)
- Changes saved immediately to `~/.paulette/config.json`

**Per-project override entry point:**
- A gear icon on the project detail view opens a condensed version of Tab 2 scoped to that project
- Shows which stages are using global defaults vs. project-specific overrides
- "Inherit from global" toggle per stage

---

## User Experience [ENHANCED]

**Feeling:** Paulette should feel like a thoughtful dev tool, not a configuration nightmare. Provider setup is a one-time task, not a recurring one. Once connections are defined and stage defaults are set, the pipeline experience is identical to v0.1.0.

**Key interaction patterns (unchanged from v0.1.0):**
- Always-streaming SSE output; Stop button during any active stream
- Approve-to-advance; autonomous mode available
- Tab state preserved on switch

**[NEW] Provider setup interactions:**
- The "Configure Paulette" entry point is prominently placed on the home page (not buried in a settings menu) because first-time setup is a common path
- Ollama / LM Studio connections: paste a URL, hit Test — done in under 10 seconds
- API key providers: paste key, Test returns the model list if available
- Copilot (P3): clicking "Connect GitHub Copilot" launches the device-code OAuth flow in-browser; Paulette polls for the token and saves it automatically; no manual token handling by the user
- If a stage fires and its assigned connection is unreachable, Paulette surfaces a clear inline error (not a silent hang) with a "Go to Configure" shortcut
- Model discovery is opportunistic: Paulette attempts a model-list fetch on connection test; if it fails, the UI silently falls back to free-text entry without blocking the save

---

## Success Metrics [ENHANCED]

- A user with **no Claude CLI installed** can run the full pipeline using Ollama alone
- A user can define 3+ connections and assign different connections to different stages in under 5 minutes
- Stage execution latency is not meaningfully degraded by the abstraction layer (provider call overhead < 10ms on top of baseline)
- Connection test gives pass/fail feedback in under 3 seconds for local providers (Ollama, LM Studio)
- **[NEW]** Zero pipeline regressions: all existing Claude CLI–backed projects continue to work without reconfiguration after upgrade

---

## Constraints & Assumptions [ENHANCED]

- **File-system persistence only**: connections and config written as JSON under `~/.paulette/`; no relational DB or secret manager in v0.2.0
- **`claude` CLI remains a valid provider**, not removed — existing users are not broken
- **Claude CLI is the implicit fallback**: if no stage assignment is configured, behaviour is identical to v0.1.0 (Claude CLI, Sonnet for Vision/UX, Opus for Architecture/Build)
- **Copilot OAuth is P3**: the device-code flow is scoped out of the initial delivery; Copilot connection type may be listed in the UI as "coming soon" to set expectations
- **No multi-user / cloud deployment** in this iteration — all credential files are local and user-scoped
- **Model capability is not validated**: Paulette cannot know whether a given model is actually capable of the task assigned to it; this is the user's responsibility
- **CORS still restricted to localhost** in this iteration

---

## Out of Scope (V1 of this feature / v0.2.0) [ENHANCED]

- **GitHub Copilot OAuth flow** (P3 — targeted for v0.3.0)
- Encrypted credential storage / OS keychain integration
- Provider-level rate limiting, cost tracking, or token-budget enforcement
- Automatic failover between providers if a call fails
- Non-LLM tool integrations (e.g. code execution sandboxes, search APIs)
- Multi-user support or cloud deployment (tracked as a future enhancement from v0.1.0 — unchanged)
- CI/CD integration hooks (tracked from v0.1.0 — unchanged)
- Plugin system or non-provider extensibility (tracked from v0.1.0 — unchanged)