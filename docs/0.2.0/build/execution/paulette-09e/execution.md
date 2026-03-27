Now let me check if lmstudio.go and openai.go already exist and what they contain:Now I have everything I need. Let me create the `openai_compat.go` file:Now let me verify the file compiles correctly within the package:Go isn't installed in this environment, so I can't run a compilation check directly. Let me do a thorough manual review to ensure correctness by cross-checking the imports and types against the existing code:Let me do a final review of the created file to ensure everything is correct:The implementation is complete. Here's a summary of what was created in `backend/internal/provider/openai_compat.go`:

## What was implemented

**`openai_compat.go`** — a shared OpenAI-compatible base used by both the LM Studio and OpenAI providers (tasks 2.3 and 2.4). It contains:

### Types
- `openAIMessage` / `openAIChatRequest` — request body for `POST /v1/chat/completions`
- `openAIStreamChunk` / `openAIChoice` / `openAIDelta` — SSE payload decoding
- `openAIError` — structured error from both SSE stream and non-2xx bodies
- `openAIModelsResponse` / `openAIModelEntry` — `GET /v1/models` response

### HTTP client factories
- `newOpenAICompatChatClient()` — 10-minute timeout for long-running SSE streams
- `newOpenAICompatTestClient()` — 30-second timeout for probe requests

### Core shared functions
- **`buildOpenAIMessages(req ChatRequest) []openAIMessage`** — converts a `ChatRequest` (system prompt + history + user message) into the OpenAI messages array, preserving the XML envelope system prompt that `agent/parse.go` depends on
- **`openAICompatChat(ctx, httpClient, baseURL, authHeaders, req)`** — marshals request, sets `Content-Type: application/json` + `Accept: text/event-stream`, applies optional auth headers, starts SSE stream in a goroutine; surfaces HTTP errors as structured messages
- **`openAICompatStreamSSE(ctx, body, ch)`** — line-by-line SSE parser that strips `data: ` prefix, handles the `[DONE]` sentinel, skips empty/comment/non-data lines, extracts `choices[0].delta.content`, and always emits a terminal `"done"` event
- **`openAICompatListModels(ctx, baseURL, authHeaders)`** — `GET /v1/models` with 30s timeout, parses `data[].id` array
- **`openAICompatTestConnection(ctx, baseURL, authHeaders)`** — delegates to `openAICompatListModels` as a connectivity + auth health check