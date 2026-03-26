Now let me look at the gemini.go and utils.go for additional patterns (NDJSON parsing specifically):Now I have a clear picture of all the patterns. Let me write the `ollama.go` file:Now verify the file compiles correctly:Go isn't installed in this environment, but the implementation is complete. Let me do a static review of the code to verify correctness:`backend/internal/provider/ollama.go` is complete. Here's a summary of what was implemented:

## `provider/ollama.go` — Implementation Summary

**Type:** New file, fully implements `ARCH-v0.2.0-012`.

### `OllamaProvider` struct
- Fields: `baseURL string`, `httpClient *http.Client`
- HTTP client configured with a **10-minute timeout** (for streaming chat), per architecture spec
- Constructor `NewOllamaProvider(baseURL string)` — strips trailing slashes for URL consistency

### `Provider` interface methods

| Method | Implementation |
|---|---|
| `Chat` | `POST /api/chat` with `stream:true`; goroutine reads NDJSON body via `streamResponse` |
| `TestConnection` | Delegates to `fetchModels` with a **30-second context deadline** |
| `ListModels` | Delegates to `fetchModels` with a **30-second context deadline** |

### NDJSON stream parsing (`streamResponse`)
- Line-by-line `bufio.Scanner` with a 1 MiB buffer
- Each line unmarshalled as `ollamaStreamChunk{Message, Done, Error}`
- `done:false` + non-empty `message.content` → `StreamEvent{Type:"chunk"}`
- `done:true` → `StreamEvent{Type:"done"}` and returns immediately
- Non-empty `error` field → `StreamEvent{Type:"error"}` + `StreamEvent{Type:"done"}` (early exit)
- Context cancellation checked between lines — never hangs
- Fallback `"done"` sentinel emitted if stream ends without an explicit `done:true` line

### Message format (`buildChatRequest`)
- System prompt prepended as `{"role":"system","content":"..."}` (Ollama supports this natively)
- History messages appended verbatim (`"user"` / `"assistant"` roles match directly)
- New user turn appended last

### Model discovery (`fetchModels` / `ListModels`)
- `GET /api/tags` → parses `{"models":[{"name":"llama3:8b"},...]}`
- Both `ID` and `Name` set to `m.Name` (Ollama model names are already human-readable)
- No auth headers sent anywhere — Ollama is a credential-free local service