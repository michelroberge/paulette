Now I have all the information I need. I'll create `frontend/src/types/provider.ts` with types that match the backend API contract from `connection.go` and the build plan specification.Now let me verify the file compiles correctly by checking the TypeScript:Clean — no TypeScript errors. Let me do a quick review of the file to confirm everything is correct:The file is created and TypeScript compilation passes cleanly. Here's a summary of what was implemented in `frontend/src/types/provider.ts`:

## Types Created

| Type | Description |
|------|-------------|
| `ProviderType` | String union of all 7 provider identifiers (`ollama`, `lmstudio`, `anthropic`, `openai`, `gemini`, `claude_cli`, `github_copilot`) — matches backend `ProviderType` constants |
| `Connection` | API response shape with `hasCredentials: boolean` — never exposes `apiKey`; matches backend `ConnectionResponse` struct |
| `ConnectionInput` | Create/update request body with optional `apiKey?: string` for writing credentials to the server |
| `ModelInfo` | Discovered model with `id` + `name`, populated from provider model-list endpoints (e.g. Ollama `/api/tags`, OpenAI `/v1/models`) |
| `TestResult` | Connection probe result with `success`, optional `error` reason, and optional `models[]` from opportunistic discovery |
| `DeleteResult` | Delete response including `affectedStages: StageName[]` so the UI can warn about cascade-cleared assignments |
| `StageAssignment` | `{ connectionId, model }` — the per-stage routing record used in both global and project configs |
| `GlobalStageConfig` | `Partial<Record<StageName, StageAssignment>>` — global defaults stored in `~/.paulette/config.json` |
| `ProjectStageConfig` | Per-project overrides with `null`-able assignments to express "inherit from global" |

Key design contracts honoured:
- `Connection` → `hasCredentials` (redacted), `ConnectionInput` → `apiKey` (write-only)  
- `ProjectStageConfig.overrides` uses `StageAssignment | null` where `null` = inherit global
- `StageName` is imported from the existing `types/index.ts` (no duplication)