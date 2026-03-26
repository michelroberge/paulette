All **50+ provider tests pass** with zero failures. The LLM provider implementations from Milestone 2 are fully complete and working. Here's a summary of what exists:

## Milestone 2 Status: ✅ Complete

All P1 and P2 provider backends are fully implemented in `/backend/internal/provider/`:

### Provider Implementations

| File | Provider | Streaming Format | Auth |
|---|---|---|---|
| `ollama.go` | **Ollama** | NDJSON via `POST /api/chat` | None (local service) |
| `openai_compat.go` | **Shared OpenAI-compatible base** | SSE `data: {...}` lines | Configurable headers |
| `lmstudio.go` | **LM Studio** | OpenAI-compatible SSE | None (delegates to compat layer) |
| `openai.go` | **OpenAI** | OpenAI SSE | `Authorization: Bearer {key}`, optional Org/Project headers |
| `anthropic.go` | **Anthropic API** | Typed SSE events (`content_block_delta`, `message_stop`) | `x-api-key` + `anthropic-version` |
| `gemini.go` | **Gemini** | SSE via `&alt=sse` on `streamGenerateContent` | `?key=` query param |
| `claude_cli.go` | **Claude CLI** (fallback) | Subprocess NDJSON | Local auth |

### Key Design Details
- **`openai_compat.go`**: Shared implementation for OpenAI and LM Studio — handles SSE parsing (`data: [DONE]` sentinel, `delta.content` extraction), model list (`GET /v1/models`), and connection testing
- **Anthropic**: Hardcoded model list (API has no model-discovery endpoint); parses typed SSE events; 1-token probe for connection test
- **Gemini**: API key as query param; `candidates[0].content.parts[0].text` extraction; model names stripped of `models/` prefix
- **HTTP timeouts**: 30s for `TestConnection`/`ListModels`, 10 min for streaming `Chat`
- **Registry wiring**: `ForConnection()` and `ResolveForStage()` correctly instantiate all 6 active providers with 3-level fallback (project override → global default → Claude CLI)
- **`chmod 600`**: Applied on every `connections.json` write