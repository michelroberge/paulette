Now I have enough context to implement the Gemini provider. Let me write it:Now let's verify it compiles cleanly:Go isn't available in the current shell environment. Let me verify the code correctness by reviewing it for any issues:The implementation looks correct. The file is well-structured and uses only stdlib imports (`bufio`, `bytes`, `context`, `encoding/json`, `fmt`, `io`, `net/http`, `strings`, `time`) — no new dependencies introduced.

Here's a summary of what was implemented in `backend/internal/provider/gemini.go`:

### `GeminiProvider` — Key design decisions

**Struct & constructor** (`NewGeminiProvider`):
- Accepts `baseURL` (defaults to `https://generativelanguage.googleapis.com`) and `apiKey`
- Trims trailing slashes from base URL for clean URL construction
- HTTP client set to 10-minute timeout (for streaming); test/model-list operations override this with a 30s context deadline

**`Chat()`**:
- Builds a Gemini-format request body: `contents[]` (with `"user"`/`"model"` roles — `"assistant"` is mapped to `"model"`) and an optional `systemInstruction` field for the stage prompt
- Appends `?key={apiKey}&alt=sse` to force SSE streaming format
- Spawns a goroutine that calls `streamResponse()` and closes the channel on completion

**`streamResponse()`**:
- Reads SSE lines (`data: {...}`) from the response body via a buffered scanner (1 MB buffer for large chunks)
- Parses each `data:` line as a `geminiStreamChunk` and extracts `candidates[0].content.parts[0].text`
- Emits `StreamEvent{Type: "chunk", Content: text}` for each non-empty text segment
- Checks context cancellation between lines; emits an `"error"` event and returns early if cancelled
- Emits `StreamEvent{Type: "done"}` when the stream ends normally

**`TestConnection()`** and **`ListModels()`**:
- Both use `GET /v1beta/models?key={apiKey}` with a 30s deadline
- `ListModels` strips the `"models/"` prefix from returned names (e.g. `"models/gemini-1.5-pro"` → `"gemini-1.5-pro"`) to produce clean IDs usable in the chat URL path