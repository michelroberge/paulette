All the provider implementations already exist. Let me check the test files and verify the code compiles correctly.All tests pass. Let me do a final full build verification of the backend:All provider implementations are already complete and fully functional. Here's a summary of what exists in the `backend/internal/provider/` package:

## Provider Implementations — Status: ✅ Complete

All 6 active provider backends are implemented:

| File | Provider | Streaming Format | Auth |
|---|---|---|---|
| `claude_cli.go` | **Claude CLI** | Delegates to `agent.ChatWithBin()` subprocess | Local auth (existing) |
| `ollama.go` | **Ollama** | NDJSON (`/api/chat`) → `StreamEvent` | None (local service) |
| `lmstudio.go` | **LM Studio** | SSE via `openai_compat` (`/v1/chat/completions`) | None (local service) |
| `openai.go` | **OpenAI** | SSE via `openai_compat` | `Authorization: Bearer {key}` + optional org/project headers |
| `anthropic.go` | **Anthropic API** | Typed SSE (`content_block_delta`, `message_stop`) | `x-api-key` header |
| `gemini.go` | **Gemini** | SSE via `&alt=sse` query param | `?key=` query param |
| `openai_compat.go` | **Shared base** | SSE parsing for OpenAI-compatible APIs | Configurable auth headers |

### Key Implementation Details
- **Timeouts**: 10-minute client timeout for chat streams; 30-second context timeout for `TestConnection` and `ListModels`
- **Context cancellation**: All stream loops check `ctx.Done()` between lines — no silent hangs
- **Always-done guarantee**: Every provider emits a final `{Type: "done"}` event, even on error paths, so callers draining the channel never block indefinitely
- **Model discovery**: Ollama (`/api/tags`), LM Studio + OpenAI (`/v1/models`), Gemini (`/v1beta/models`), Anthropic (hardcoded list), Claude CLI (`ErrModelListUnsupported`)
- **GitHub Copilot**: Returns a clear "not yet available" error (P3 — v0.3.0)

All **59 unit tests** pass with `ok github.com/michelroberge/paulette/backend/internal/provider`.