# System Architecture: Paulette v0.2.0

---

## Tech Stack

### Preserved from v0.1.0

| Layer | Technology | Version | Rationale |
|---|---|---|---|
| Backend runtime | Go | 1.26 | Single-binary deployment, goroutine-native concurrency for SSE streams |
| HTTP router | chi | v5 | Lightweight, idiomatic middleware chain |
| CORS | rs/cors | v1 | Standard CORS middleware |
| Frontend framework | React | 19 | Component model, hooks, concurrent features |
| Frontend language | TypeScript | 5.9 | Type safety across API boundary |
| Build tooling | Vite | 8 | Fast HMR, ESBuild bundling |
| Graph visualisation | @xyflow/react + Dagre | 12.x | Bead DAG rendering |
| Code editor | Monaco Editor | latest | Artifact editing |
| Markdown | react-markdown + remark-gfm | latest | Artifact display |
| Styling | Tailwind CSS | 4.x | Utility-first, matches UX spec conventions |
| AI inference (existing) | Claude CLI subprocess | — | Retained as one provider option |
| Build task runner | bd (Beads) CLI + Dolt | — | Bead DAG execution |
| Version control | git (via os/exec) | — | Artifact versioning, rollback |
| Persistence | File-system JSON + Markdown | — | `~/.paulette/` directory tree |
| Binary embedding | Go `embed.FS` | — | Frontend assets compiled into server binary |

### New in v0.2.0

| Layer | Technology | Rationale |
|---|---|---|
| HTTP client (provider calls) | Go `net/http` (stdlib) | Zero new dependencies; all provider APIs are HTTP/JSON |

No new Go or npm dependencies are required. All provider integrations use standard HTTP + JSON over the Go stdlib `net/http` client and the existing `encoding/json` package.

---

## System Components

### Preserved from v0.1.0

### [ARCH-v0.2.0-001] Server Entry Point (`main.go`)
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.1.0-004, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-007, JRN-v0.1.0-008

**[MODIFIED]** — Initialises the new `provider.Registry` and `ConnectionStore` on startup; passes them to the server constructor. Loads `~/.paulette/connections.json` and `~/.paulette/config.json` at boot.

```go
// main.go additions
connStore := provider.NewConnectionStore(cfg.RegistryPath)
providerRegistry := provider.NewRegistry(connStore)
stageConfig := provider.NewStageConfigStore(cfg.RegistryPath)

srv := server.New(cfg, registry, projectRepo, artifactRepo, chatRepo, activityRepo,
    staticSub, connStore, providerRegistry, stageConfig)
```

### [ARCH-v0.2.0-002] HTTP Server & Router (`server/server.go`)
> Journeys: JRN-v0.1.0-001, JRN-v0.1.0-002, JRN-v0.1.0-003, JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-006

**[MODIFIED]** — Server struct gains `connStore`, `providerRegistry`, and `stageConfig` fields. Router mounts new `/api/connections/*` and `/api/config/*` route groups. All existing routes unchanged.

### [ARCH-v0.2.0-003] Pipeline State Machine (`pipeline/machine.go`)
> Journeys: JRN-v0.1.0-003, JRN-v0.1.0-004

Unchanged. Stage promotion logic, approval gates, and git commit lifecycle are unaffected by the provider layer.

### [ARCH-v0.2.0-004] Stream Manager (`stream/manager.go`)
> Journeys: JRN-v0.1.0-002, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.2.0-008

Unchanged. The `Run` / `Manager` / `StreamTo` pattern continues to work identically — the only difference is that events now originate from any provider rather than only the Claude CLI subprocess. The `StreamEvent` struct is unchanged.

### [ARCH-v0.2.0-005] Agent Package — Claude CLI Provider (`agent/agent.go`)
> Journeys: JRN-v0.1.0-002, JRN-v0.1.0-005, JRN-v0.1.0-006, JRN-v0.1.0-008

**[MODIFIED]** — The existing `agent.Chat()` function is refactored to implement the new `provider.Provider` interface (see ARCH-v0.2.0-010). Externally, Claude CLI is now accessed via the provider registry like any other backend. The `stageModels` map is retained as the fallback default when no stage configuration exists.

### [ARCH-v0.2.0-006] Chat Handler (`handler/chat.go`)
> Journeys: JRN-v0.1.0-002, JRN-v0.2.0-008

**[MODIFIED]** — `StartChatRun()` no longer calls `agent.Chat()` directly. Instead it:
1. Resolves the connection + model for the current project + stage via `provider.ResolveProvider()`.
2. Calls `provider.Chat()` on the resolved provider, which returns the same `<-chan StreamEvent`.
3. Error handling enhanced: if the provider is unreachable, emits a `StreamEvent{Type: "error", Content: "..."}` with structured error info so the frontend can render SCR-012.

### [ARCH-v0.2.0-007] Mock Handler (`handler/mock.go`)
> Journeys: JRN-v0.1.0-005, JRN-v0.2.0-008

**[MODIFIED]** — Same refactor as Chat Handler: mock generation routes through the provider layer.

### [ARCH-v0.2.0-008] Bead Handler (`handler/bead.go`)
> Journeys: JRN-v0.1.0-006

Unchanged. Bead execution delegates to the `bd` CLI, not to the LLM provider. Bead graph generation (parsing architecture artifact) is a local operation.

### [ARCH-v0.2.0-009] Autopilot Orchestrator (`autopilot/orchestrator.go`)
> Journeys: JRN-v0.1.0-004, JRN-v0.2.0-008

**[MODIFIED]** — The orchestrator's autonomous stage-advance loop now uses the same provider-resolved path. No structural change; it continues calling handler methods which internally resolve the provider.

---

### New in v0.2.0

### [ARCH-v0.2.0-010] Provider Interface & Registry (`provider/provider.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005, JRN-v0.2.0-008

The core abstraction layer. Defines the interface that all LLM backends must implement and a registry that instantiates the correct provider for a given connection.

```go
package provider

// Provider is the interface every LLM backend implements.
type Provider interface {
    // Chat sends a conversation to the LLM and streams events back.
    Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)

    // TestConnection sends a minimal probe and returns nil on success.
    TestConnection(ctx context.Context) error

    // ListModels returns available models, or ErrModelListUnsupported.
    ListModels(ctx context.Context) ([]ModelInfo, error)
}

type ChatRequest struct {
    Model        string
    SystemPrompt string
    History      []Message
    UserMessage  string
    ProjectDir   string // working directory context (used by Claude CLI)
}

type ModelInfo struct {
    ID   string // e.g. "llama3:8b", "gpt-4o"
    Name string // human-readable label
}

var ErrModelListUnsupported = errors.New("model listing not supported")

// StreamEvent is re-exported from agent package — same struct.
type StreamEvent = agent.StreamEvent

// Registry maps provider types to factory functions.
type Registry struct {
    connStore *ConnectionStore
}

func NewRegistry(cs *ConnectionStore) *Registry

// ForConnection returns a configured Provider instance for the given connection ID.
func (r *Registry) ForConnection(connectionID string) (Provider, error)

// ResolveForStage returns the Provider + model for a given project + stage,
// consulting per-project overrides first, then global defaults, then the
// hardcoded Claude CLI fallback.
func (r *Registry) ResolveForStage(
    projectID string,
    stage model.StageName,
    stageConfig *StageConfigStore,
    projectRepo repository.ProjectRepo,
    hostDir string,
) (Provider, string, error)
```

**Resolution order** in `ResolveForStage`:
1. Check per-project overrides in `<hostDir>/.paulette/stage_config.json`
2. Check global defaults in `~/.paulette/config.json`
3. Fall back to Claude CLI with the hardcoded `stageModels` map (Sonnet for Vision/UX, Opus for Architecture/Build)

### [ARCH-v0.2.0-011] Claude CLI Provider (`provider/claude_cli.go`) [NEW]
> Journeys: JRN-v0.1.0-002, JRN-v0.1.0-005, JRN-v0.2.0-001

Wraps the existing `agent.Chat()` function behind the `Provider` interface. This is a thin adapter:

```go
type ClaudeCLIProvider struct {
    claudePath string // path to claude binary
}

func (p *ClaudeCLIProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
func (p *ClaudeCLIProvider) TestConnection(ctx context.Context) error // runs "claude --version"
func (p *ClaudeCLIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) // returns ErrModelListUnsupported
```

### [ARCH-v0.2.0-012] Ollama Provider (`provider/ollama.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005

Implements `Provider` using Ollama's HTTP API:

| Operation | Ollama API endpoint |
|---|---|
| Chat | `POST /api/chat` (streaming JSON) |
| TestConnection | `GET /api/tags` (returns model list) |
| ListModels | `GET /api/tags` → parse `models[].name` |

```go
type OllamaProvider struct {
    baseURL    string // e.g. "http://localhost:11434"
    httpClient *http.Client
}
```

**Streaming adaptation:** Ollama streams newline-delimited JSON objects with `{"message":{"content":"..."}, "done":false}`. The provider reads these line-by-line and converts each to a `StreamEvent{Type: "chunk", Content: ...}`. On `"done":true`, it emits `StreamEvent{Type: "done"}`.

### [ARCH-v0.2.0-013] LM Studio Provider (`provider/lmstudio.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005

LM Studio exposes an OpenAI-compatible API. This provider reuses the OpenAI-compatible chat completions protocol:

| Operation | API endpoint |
|---|---|
| Chat | `POST /v1/chat/completions` (streaming SSE, `stream: true`) |
| TestConnection | `GET /v1/models` |
| ListModels | `GET /v1/models` → parse `data[].id` |

```go
type LMStudioProvider struct {
    baseURL    string
    httpClient *http.Client
}
```

### [ARCH-v0.2.0-014] OpenAI Provider (`provider/openai.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005

Standard OpenAI Chat Completions API:

| Operation | API endpoint |
|---|---|
| Chat | `POST /v1/chat/completions` (streaming SSE) |
| TestConnection | `GET /v1/models` (validates API key) |
| ListModels | `GET /v1/models` → parse `data[].id` |

```go
type OpenAIProvider struct {
    baseURL    string // default "https://api.openai.com"
    apiKey     string
    orgID      string // optional
    projectID  string // optional
    httpClient *http.Client
}
```

**Streaming adaptation:** OpenAI streams `data: {"choices":[{"delta":{"content":"..."}}]}` SSE events. The provider parses each SSE line and converts `delta.content` chunks into `StreamEvent{Type: "chunk"}`. A `data: [DONE]` sentinel triggers `StreamEvent{Type: "done"}`.

### [ARCH-v0.2.0-015] Anthropic API Provider (`provider/anthropic.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005

Direct Anthropic Messages API (bypassing `claude` CLI):

| Operation | API endpoint |
|---|---|
| Chat | `POST /v1/messages` (streaming SSE, `stream: true`) |
| TestConnection | `POST /v1/messages` (minimal 1-token request) |
| ListModels | Returns hardcoded list of known Anthropic models |

```go
type AnthropicProvider struct {
    baseURL    string // default "https://api.anthropic.com"
    apiKey     string
    httpClient *http.Client
}
```

**Streaming adaptation:** Anthropic streams typed SSE events (`content_block_delta`, `message_stop`, etc.). The provider maps `content_block_delta` → `StreamEvent{Type: "chunk"}` and `message_stop` → `StreamEvent{Type: "done"}`.

### [ARCH-v0.2.0-016] Gemini Provider (`provider/gemini.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005

Google Gemini API via the `generateContent` endpoint:

| Operation | API endpoint |
|---|---|
| Chat | `POST /v1beta/models/{model}:streamGenerateContent?key={key}` |
| TestConnection | `GET /v1beta/models?key={key}` |
| ListModels | `GET /v1beta/models?key={key}` → parse `models[].name` |

```go
type GeminiProvider struct {
    baseURL    string // default "https://generativelanguage.googleapis.com"
    apiKey     string
    httpClient *http.Client
}
```

**Streaming adaptation:** Gemini streams newline-delimited JSON array entries. Each object contains `candidates[0].content.parts[0].text` which maps to `StreamEvent{Type: "chunk"}`.

### [ARCH-v0.2.0-017] OpenAI-Compatible Base (`provider/openai_compat.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-005

Shared implementation for LM Studio and OpenAI providers (and potentially future OpenAI-compatible backends). Contains:
- `openAICompatChat()` — streaming chat completions with SSE parsing
- `openAICompatListModels()` — `GET /v1/models` parsing
- `openAICompatTestConnection()` — model list as health check
- Message format conversion (`ChatRequest` → OpenAI messages array)

Both `OpenAIProvider` and `LMStudioProvider` delegate to these shared functions, differing only in auth headers and base URL defaults.

### [ARCH-v0.2.0-018] Connection Store (`provider/connection_store.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004

Manages CRUD for connection records, persisted to `~/.paulette/connections.json`:

```go
type ConnectionStore struct {
    path        string // ~/.paulette/connections.json
    mu          sync.RWMutex
    connections []Connection
}

func NewConnectionStore(registryPath string) *ConnectionStore
func (s *ConnectionStore) List() []Connection
func (s *ConnectionStore) Get(id string) (*Connection, error)
func (s *ConnectionStore) Create(conn Connection) (*Connection, error)   // assigns UUID, chmod 600
func (s *ConnectionStore) Update(conn Connection) (*Connection, error)
func (s *ConnectionStore) Delete(id string) ([]string, error)           // returns affected stage assignments
func (s *ConnectionStore) save() error                                   // writes JSON + chmod 600
```

On every `save()`, the file is written atomically (write to temp, rename) and `os.Chmod(path, 0600)` is applied.

### [ARCH-v0.2.0-019] Stage Config Store (`provider/stage_config.go`) [NEW]
> Journeys: JRN-v0.2.0-006, JRN-v0.2.0-007

Manages global stage defaults and per-project overrides:

```go
type StageConfigStore struct {
    globalPath string // ~/.paulette/config.json
    mu         sync.RWMutex
}

type StageAssignment struct {
    ConnectionID string `json:"connectionId"`
    Model        string `json:"model"`
}

type GlobalConfig struct {
    StageDefaults map[model.StageName]StageAssignment `json:"stageDefaults"`
}

type ProjectStageConfig struct {
    Overrides map[model.StageName]*StageAssignment `json:"overrides"` // nil = inherit
}

func NewStageConfigStore(registryPath string) *StageConfigStore
func (s *StageConfigStore) GetGlobalDefaults() GlobalConfig
func (s *StageConfigStore) SetGlobalStageDefault(stage model.StageName, assignment StageAssignment) error
func (s *StageConfigStore) GetProjectOverrides(hostDir string) ProjectStageConfig
func (s *StageConfigStore) SetProjectStageOverride(hostDir string, stage model.StageName, assignment *StageAssignment) error
func (s *StageConfigStore) ResetProjectOverrides(hostDir string) error
func (s *StageConfigStore) ClearConnectionReferences(connectionID string) []string // returns affected stage names
```

**File locations:**
- Global: `~/.paulette/config.json`
- Per-project: `<hostDir>/.paulette/stage_config.json`

### [ARCH-v0.2.0-020] Connection Handler (`handler/connection.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004, JRN-v0.2.0-005

HTTP handlers for connection CRUD and testing:

```go
type ConnectionHandler struct {
    connStore        *provider.ConnectionStore
    providerRegistry *provider.Registry
}

func (h *ConnectionHandler) List(w, r)           // GET    /api/connections
func (h *ConnectionHandler) Create(w, r)         // POST   /api/connections
func (h *ConnectionHandler) Get(w, r)            // GET    /api/connections/:id
func (h *ConnectionHandler) Update(w, r)         // PUT    /api/connections/:id
func (h *ConnectionHandler) Delete(w, r)         // DELETE /api/connections/:id
func (h *ConnectionHandler) Test(w, r)           // POST   /api/connections/:id/test
func (h *ConnectionHandler) TestNew(w, r)        // POST   /api/connections/test (unsaved)
func (h *ConnectionHandler) ListModels(w, r)     // GET    /api/connections/:id/models
func (h *ConnectionHandler) ListModelsNew(w, r)  // POST   /api/connections/models (unsaved)
```

### [ARCH-v0.2.0-021] Config Handler (`handler/config_handler.go`) [NEW]
> Journeys: JRN-v0.2.0-006, JRN-v0.2.0-007

HTTP handlers for stage configuration:

```go
type ConfigHandler struct {
    stageConfig  *provider.StageConfigStore
    connStore    *provider.ConnectionStore
    registry     repository.RegistryRepo
    projectRepo  repository.ProjectRepo
}

func (h *ConfigHandler) GetGlobalDefaults(w, r)       // GET  /api/config/stages
func (h *ConfigHandler) SetGlobalStageDefault(w, r)    // PUT  /api/config/stages/:stage
func (h *ConfigHandler) GetProjectOverrides(w, r)      // GET  /api/projects/:id/config/stages
func (h *ConfigHandler) SetProjectStageOverride(w, r)  // PUT  /api/projects/:id/config/stages/:stage
func (h *ConfigHandler) ResetProjectOverrides(w, r)    // POST /api/projects/:id/config/stages/reset
```

### [ARCH-v0.2.0-022] Frontend: Configure Paulette Page (`components/configure/ConfigurePage.tsx`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-006

Top-level page component for `/configure`. Renders the two-tab layout (Connections, Stage Defaults) as described in SCR-007. Uses React Router for navigation from the home page.

### [ARCH-v0.2.0-023] Frontend: Connections Tab (`components/configure/ConnectionsTab.tsx`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004

Renders the connection list (SCR-008) with Add/Edit/Delete/Test actions. Manages connection list state and delegates to `ConnectionForm` for creation/editing.

### [ARCH-v0.2.0-024] Frontend: Connection Form (`components/configure/ConnectionForm.tsx`) [NEW]
> Journeys: JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-005

Slide-over form panel (SCR-009). Implements:
- Provider-type-driven conditional field rendering (IACT-007)
- Test Connection with inline status display
- Opportunistic model discovery (IACT-008)
- Credential warning banner
- Save/Cancel actions

### [ARCH-v0.2.0-025] Frontend: Stage Defaults Tab (`components/configure/StageDefaultsTab.tsx`) [NEW]
> Journeys: JRN-v0.2.0-006

Renders the five-stage configuration table (SCR-010). Each row has a connection picker and model field. Implements auto-save with inline confirmation (IACT-010).

### [ARCH-v0.2.0-026] Frontend: Project Stage Settings Panel (`components/configure/ProjectStageSettings.tsx`) [NEW]
> Journeys: JRN-v0.2.0-007

Slide-over panel triggered by the ⚙ gear icon in the project header (SCR-011). Shows per-stage Inherit toggles (IACT-012) and connection/model pickers for overridden stages. Auto-saves to project-level config.

### [ARCH-v0.2.0-027] Frontend: Connection Error Banner (`components/chat/ConnectionErrorBanner.tsx`) [NEW]
> Journeys: JRN-v0.2.0-008

Inline error banner component (SCR-012). Displays connection failure details with "Go to Configure →" deep-link (IACT-011) and "Change stage connection" shortcut.

### [ARCH-v0.2.0-028] Frontend: Credential Warning Toast (`components/configure/CredentialWarningToast.tsx`) [NEW]
> Journeys: JRN-v0.2.0-002, JRN-v0.2.0-003

Toast notification component (IACT-009). Auto-dismisses after 6 seconds. Only shown when API key credentials are saved.

### [ARCH-v0.2.0-029] Frontend: API Client — Connections (`api/connections.ts`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004, JRN-v0.2.0-005

```typescript
export function listConnections(): Promise<Connection[]>
export function createConnection(conn: ConnectionInput): Promise<Connection>
export function updateConnection(id: string, conn: ConnectionInput): Promise<Connection>
export function deleteConnection(id: string): Promise<DeleteResult>
export function testConnection(id: string): Promise<TestResult>
export function testNewConnection(conn: ConnectionInput): Promise<TestResult>
export function listModels(id: string): Promise<ModelInfo[]>
export function listModelsForNew(conn: ConnectionInput): Promise<ModelInfo[]>
```

### [ARCH-v0.2.0-030] Frontend: API Client — Stage Config (`api/stageConfig.ts`) [NEW]
> Journeys: JRN-v0.2.0-006, JRN-v0.2.0-007

```typescript
export function getGlobalDefaults(): Promise<GlobalStageConfig>
export function setGlobalStageDefault(stage: StageName, assignment: StageAssignment): Promise<void>
export function getProjectOverrides(projectId: string): Promise<ProjectStageConfig>
export function setProjectStageOverride(projectId: string, stage: StageName, assignment: StageAssignment | null): Promise<void>
export function resetProjectOverrides(projectId: string): Promise<void>
```

### [ARCH-v0.2.0-031] Frontend: TypeScript Types — Provider (`types/provider.ts`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-006, JRN-v0.2.0-007

```typescript
export type ProviderType = 'ollama' | 'lmstudio' | 'anthropic' | 'openai' | 'gemini' | 'claude_cli' | 'github_copilot'

export interface Connection {
  id: string
  name: string
  providerType: ProviderType
  baseUrl?: string
  hasCredentials: boolean   // never expose actual key to frontend
  orgId?: string
  projectId?: string
  defaultModel: string
  discoveredModels?: ModelInfo[]
}

export interface ConnectionInput {
  name: string
  providerType: ProviderType
  baseUrl?: string
  apiKey?: string          // sent on create/update only
  orgId?: string
  projectId?: string
  defaultModel: string
}

export interface ModelInfo {
  id: string
  name: string
}

export interface TestResult {
  success: boolean
  error?: string
  models?: ModelInfo[]
}

export interface DeleteResult {
  affectedStages: string[]
}

export interface StageAssignment {
  connectionId: string
  model: string
}

export interface GlobalStageConfig {
  stageDefaults: Partial<Record<StageName, StageAssignment>>
}

export interface ProjectStageConfig {
  overrides: Partial<Record<StageName, StageAssignment | null>>
}
```

### [ARCH-v0.2.0-032] Frontend: Router Setup [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-008

**[MODIFIED]** — `App.tsx` gains client-side routing (React Router or simple state-based routing) to support the `/configure` page and deep-link navigation from error banners. Routes:
- `/` — Home page (project list)
- `/projects/:id` — Project detail view
- `/configure` — Configure Paulette page (with `?tab=` and `?highlight=` query params)

### [ARCH-v0.2.0-033] Frontend: Home Page Modifications (`App.tsx` / `components/project/ProjectList.tsx`) [MODIFIED]
> Journeys: JRN-v0.2.0-001

The home page action buttons row gains the "Configure Paulette" button alongside "New Project" and "Import Project" per SCR-001.

### [ARCH-v0.2.0-034] Frontend: Project Header Modifications (`components/layout/ProjectHeader.tsx`) [MODIFIED]
> Journeys: JRN-v0.2.0-007

The project header bar gains the ⚙ gear icon button that triggers the Project Stage Settings panel. Stage pips with per-project overrides display a `ring-2 ring-yellow-400` accent.

---

## API Design

### Preserved from v0.1.0

All existing endpoints are unchanged:

| Method | Path | Handler | SSE |
|---|---|---|---|
| GET | `/api/config` | ConfigHandler.GetInfo | |
| GET | `/api/auth/status` | AuthHandler.Status | |
| GET | `/api/auth/login` | AuthHandler.Login | |
| POST | `/api/auth/login/input` | AuthHandler.LoginInput | |
| POST | `/api/auth/logout` | AuthHandler.Logout | |
| POST | `/api/projects` | ProjectHandler.Create | |
| GET | `/api/projects` | ProjectHandler.List | |
| GET | `/api/projects/:id` | ProjectHandler.Get | |
| PATCH | `/api/projects/:id` | ProjectHandler.Patch | |
| DELETE | `/api/projects/:id` | ProjectHandler.Delete | |
| GET | `/api/projects/activity` | ActivityHandler.Summary | |
| GET | `/api/projects/:id/pipeline` | PipelineHandler.GetPipeline | |
| GET | `/api/projects/:id/pipeline/watch` | PipelineHandler.WatchPipeline | ✓ |
| POST | `/api/projects/:id/pipeline/approve` | PipelineHandler.Approve | |
| GET | `/api/projects/:id/pipeline/summary` | PipelineHandler.GetSummary | |
| GET | `/api/projects/:id/pipeline/summary/watch` | PipelineHandler.WatchSummary | ✓ |
| POST | `/api/projects/:id/pipeline/summary` | PipelineHandler.RegenerateSummary | |
| POST | `/api/projects/:id/pipeline/summary/approve` | PipelineHandler.ApproveSummary | |
| GET | `/api/projects/:id/download` | PipelineHandler.DownloadProject | |
| GET | `/api/projects/:id/stages/:stage/artifact` | ArtifactHandler.Get | |
| GET | `/api/projects/:id/stages/:stage/chat` | ChatHandler.GetHistory | |
| POST | `/api/projects/:id/stages/:stage/chat` | ChatHandler.Send | ✓ |
| POST | `/api/projects/:id/stages/:stage/chat/resume` | ChatHandler.Resume | ✓ |
| GET | `/api/projects/:id/stages/ux/mock` | MockHandler.Get | |
| POST | `/api/projects/:id/stages/ux/mock` | MockHandler.Generate | ✓ |
| GET | `/api/projects/:id/stages/ux/framework` | MockHandler.GetFramework | |
| PUT | `/api/projects/:id/stages/ux/framework` | MockHandler.SetFramework | |
| GET | `/api/projects/:id/stages/build/beads` | BeadHandler.GetGraph | |
| POST | `/api/projects/:id/stages/build/beads/graph` | BeadHandler.ExecuteGraph | ✓ |
| POST | `/api/projects/:id/pipeline/enhance` | EnhanceHandler.Enhance | ✓ |

### New in v0.2.0

#### Connection Management

| Method | Path | Handler | Description |
|---|---|---|---|
| GET | `/api/connections` | ConnectionHandler.List | List all connections (credentials redacted) |
| POST | `/api/connections` | ConnectionHandler.Create | Create a new connection |
| GET | `/api/connections/:id` | ConnectionHandler.Get | Get single connection (credentials redacted) |
| PUT | `/api/connections/:id` | ConnectionHandler.Update | Update connection fields |
| DELETE | `/api/connections/:id` | ConnectionHandler.Delete | Delete connection; returns affected stages |
| POST | `/api/connections/:id/test` | ConnectionHandler.Test | Test a saved connection |
| POST | `/api/connections/test` | ConnectionHandler.TestNew | Test an unsaved connection (body contains full config) |
| GET | `/api/connections/:id/models` | ConnectionHandler.ListModels | Fetch models from a saved connection |
| POST | `/api/connections/models` | ConnectionHandler.ListModelsNew | Fetch models from unsaved connection config |

**POST `/api/connections`** — Request:
```json
{
  "name": "Local Llama3",
  "providerType": "ollama",
  "baseUrl": "http://localhost:11434",
  "apiKey": "",
  "orgId": "",
  "projectId": "",
  "defaultModel": "llama3:8b"
}
```
Response `201`:
```json
{
  "id": "uuid",
  "name": "Local Llama3",
  "providerType": "ollama",
  "baseUrl": "http://localhost:11434",
  "hasCredentials": false,
  "defaultModel": "llama3:8b"
}
```

**POST `/api/connections/:id/test`** — Response:
```json
{
  "success": true,
  "models": [
    {"id": "llama3:8b", "name": "llama3:8b"},
    {"id": "mistral:7b", "name": "mistral:7b"}
  ]
}
```
Or on failure:
```json
{
  "success": false,
  "error": "Connection refused at http://localhost:11434"
}
```

**DELETE `/api/connections/:id`** — Response:
```json
{
  "affectedStages": ["vision", "ux"]
}
```

#### Stage Configuration

| Method | Path | Handler | Description |
|---|---|---|---|
| GET | `/api/config/stages` | ConfigHandler.GetGlobalDefaults | Get global stage defaults |
| PUT | `/api/config/stages/:stage` | ConfigHandler.SetGlobalStageDefault | Set default for one stage |
| GET | `/api/projects/:id/config/stages` | ConfigHandler.GetProjectOverrides | Get per-project overrides |
| PUT | `/api/projects/:id/config/stages/:stage` | ConfigHandler.SetProjectStageOverride | Set per-project override for one stage |
| POST | `/api/projects/:id/config/stages/reset` | ConfigHandler.ResetProjectOverrides | Reset all project overrides |

**GET `/api/config/stages`** — Response:
```json
{
  "stageDefaults": {
    "vision": {"connectionId": "uuid-1", "model": "llama3:8b"},
    "ux": {"connectionId": "uuid-1", "model": "llama3:8b"},
    "architecture": {"connectionId": "uuid-2", "model": "gpt-4o"},
    "build": {"connectionId": "uuid-2", "model": "gpt-4o"},
    "complete": {"connectionId": "uuid-1", "model": "llama3:8b"}
  }
}
```

**PUT `/api/config/stages/:stage`** — Request:
```json
{
  "connectionId": "uuid-1",
  "model": "llama3:8b"
}
```

**PUT `/api/projects/:id/config/stages/:stage`** — Request:
```json
{
  "connectionId": "uuid-2",
  "model": "gpt-4o"
}
```
Send `null` body to clear the override (revert to inherit).

---

## Data Models

### Preserved from v0.1.0

#### Project (`model/project.go`)
```go
type Project struct {
    ID                string
    Name              string
    Author            string
    Version           string
    HostDir           string
    CurrentStage      StageName
    Iteration         int
    EnhancementVision string
    Imported          bool
    SummaryReady      bool
    SummaryApproved   bool
    Autonomous        bool
    BaseBranch        string
    DevCommands       []string
    BuildCommands     []string
    RunCommands       []string
    SummaryTokens     int
    StageTokens       map[StageName]int
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

#### Message, ChatHistory, StageActivity, StreamEvent
All unchanged.

### New in v0.2.0

#### Connection (`provider/connection.go`) [NEW]
> Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004

```go
type ProviderType string

const (
    ProviderOllama       ProviderType = "ollama"
    ProviderLMStudio     ProviderType = "lmstudio"
    ProviderAnthropic    ProviderType = "anthropic"
    ProviderOpenAI       ProviderType = "openai"
    ProviderGemini       ProviderType = "gemini"
    ProviderClaudeCLI    ProviderType = "claude_cli"
    ProviderGitHubCopilot ProviderType = "github_copilot" // P3 — not functional in v0.2.0
)

type Connection struct {
    ID           string       `json:"id"`            // UUID
    Name         string       `json:"name"`          // User-defined label
    ProviderType ProviderType `json:"providerType"`
    BaseURL      string       `json:"baseUrl,omitempty"`
    APIKey       string       `json:"apiKey,omitempty"`      // Stored in plain text, file chmod 600
    OrgID        string       `json:"orgId,omitempty"`       // OpenAI only
    ProjectID    string       `json:"projectId,omitempty"`   // OpenAI only
    DefaultModel string       `json:"defaultModel"`
    CreatedAt    time.Time    `json:"createdAt"`
    UpdatedAt    time.Time    `json:"updatedAt"`
}

// ConnectionResponse is the API-facing struct (credentials redacted)
type ConnectionResponse struct {
    ID             string       `json:"id"`
    Name           string       `json:"name"`
    ProviderType   ProviderType `json:"providerType"`
    BaseURL        string       `json:"baseUrl,omitempty"`
    HasCredentials bool         `json:"hasCredentials"`
    OrgID          string       `json:"orgId,omitempty"`
    ProjectID      string       `json:"projectId,omitempty"`
    DefaultModel   string       `json:"defaultModel"`
    CreatedAt      time.Time    `json:"createdAt"`
    UpdatedAt      time.Time    `json:"updatedAt"`
}
```

#### StageAssignment & Config Files [NEW]
> Journeys: JRN-v0.2.0-006, JRN-v0.2.0-007

```go
type StageAssignment struct {
    ConnectionID string `json:"connectionId"`
    Model        string `json:"model"`
}

// GlobalConfig — persisted at ~/.paulette/config.json
type GlobalConfig struct {
    StageDefaults map[model.StageName]StageAssignment `json:"stageDefaults"`
}

// ProjectStageConfig — persisted at <hostDir>/.paulette/stage_config.json
type ProjectStageConfig struct {
    Overrides map[model.StageName]*StageAssignment `json:"overrides"`
}
```

### File-System Persistence Layout

```
~/.paulette/
├── registry.json              # Project registry (unchanged)
├── settings.json              # User settings (unchanged)
├── connections.json           # [NEW] Connection definitions (chmod 600)
├── config.json                # [NEW] Global stage defaults
└── repos/
    └── <project>/
        └── .paulette/
            ├── project.json       # Project metadata (unchanged)
            ├── activity.json      # Stage activity (unchanged)
            ├── sessions.json      # Token sessions (unchanged)
            ├── stage_config.json  # [NEW] Per-project stage overrides
            ├── vision/
            ├── ux/
            ├── architecture/
            ├── build/
            └── iterations/
```

### connections.json Schema
```json
{
  "connections": [
    {
      "id": "a1b2c3d4-...",
      "name": "Local Llama3",
      "providerType": "ollama",
      "baseUrl": "http://localhost:11434",
      "apiKey": "",
      "defaultModel": "llama3:8b",
      "createdAt": "2026-03-25T10:00:00Z",
      "updatedAt": "2026-03-25T10:00:00Z"
    },
    {
      "id": "e5f6g7h8-...",
      "name": "OpenAI GPT-4o",
      "providerType": "openai",
      "baseUrl": "",
      "apiKey": "sk-...",
      "orgId": "org-...",
      "defaultModel": "gpt-4o",
      "createdAt": "2026-03-25T10:05:00Z",
      "updatedAt": "2026-03-25T10:05:00Z"
    }
  ]
}
```

### config.json Schema
```json
{
  "stageDefaults": {
    "vision": {"connectionId": "a1b2c3d4-...", "model": "llama3:8b"},
    "ux": {"connectionId": "a1b2c3d4-...", "model": "llama3:8b"},
    "architecture": {"connectionId": "e5f6g7h8-...", "model": "gpt-4o"},
    "build": {"connectionId": "e5f6g7h8-...", "model": "gpt-4o"},
    "complete": {"connectionId": "a1b2c3d4-...", "model": "llama3:8b"}
  }
}
```

### stage_config.json Schema (per-project)
```json
{
  "overrides": {
    "architecture": {"connectionId": "e5f6g7h8-...", "model": "gpt-4o-mini"},
    "build": null
  }
}
```
Keys present with `null` values or missing keys both mean "inherit from global".

---

## Infrastructure

### Preserved from v0.1.0

| Aspect | Detail |
|---|---|
| Deployment | Single Go binary with embedded frontend; no containers required |
| Persistence | File-system only under `~/.paulette/` |
| CORS | `localhost:5173` (dev) and `localhost:8080` (prod) |
| Process model | Single process; goroutines for SSE streams and autopilot |
| External dependencies | `claude` CLI, `bd` CLI + Dolt, `git` |

### New in v0.2.0

| Aspect | Detail |
|---|---|
| External dependencies (expanded) | `claude` CLI is now **optional** (only required if Claude CLI provider is used); `bd`, `git` remain required |
| Network egress | Provider calls may reach external APIs (api.openai.com, api.anthropic.com, generativelanguage.googleapis.com) or local services (localhost:11434 for Ollama) depending on configured connections |
| Credential security | `connections.json` protected by `chmod 600`; no encryption at rest in v0.2.0 |
| HTTP client timeouts | All provider HTTP clients use a 30-second timeout for connection test, 10-minute timeout for chat streaming |

### Provider API Compatibility Matrix

| Provider | Chat Endpoint | Streaming Format | Auth Header | Model List |
|---|---|---|---|---|
| Ollama | `POST /api/chat` | NDJSON | None | `GET /api/tags` |
| LM Studio | `POST /v1/chat/completions` | SSE (`data:` lines) | None | `GET /v1/models` |
| OpenAI | `POST /v1/chat/completions` | SSE (`data:` lines) | `Authorization: Bearer {key}` | `GET /v1/models` |
| Anthropic | `POST /v1/messages` | SSE (typed events) | `x-api-key: {key}` | Hardcoded list |
| Gemini | `POST /v1beta/models/{model}:streamGenerateContent` | NDJSON array | `?key={key}` query param | `GET /v1beta/models` |
| Claude CLI | subprocess stdio | NDJSON | Local auth | N/A |

### System Prompt Adaptation

The existing system prompts (from `agent/prompts.go`) use XML-envelope response formatting (`<response><discussion>...<artifact>...`). This format must be preserved regardless of provider, as the backend's `agent/parse.go` depends on it. Each provider's `Chat` implementation wraps the same system prompt — the LLM is instructed to respond in the same XML format. No per-provider prompt variation is needed; the abstraction is purely at the transport/API level.

**Note:** Model capability variance (e.g., a small Ollama model may not follow the XML envelope reliably) is the user's responsibility per the vision document. No runtime validation of response format conformance is performed.

---

## Validation Commands

### Dev Commands
- `cd /root/repos/paulette/frontend && npm run dev`
- `cd /root/repos/paulette/backend && go run main.go`

### Build Commands
- `cd /root/repos/paulette/frontend && npm run build`
- `cd /root/repos/paulette/backend && go build -o paulette ./main.go`

### Run Commands
- `cd /root/repos/paulette/backend && ./paulette`