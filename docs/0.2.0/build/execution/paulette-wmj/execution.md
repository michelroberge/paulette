Now let me look at the sibling providers to understand the correct patterns:Now I have a clear picture of all three issues. Let me fix `lmstudio.go`:

1. **Constructor signature**: Change `NewLMStudioProvider` to return `*LMStudioProvider` only (no error), since the empty-URL guard is already in `provider.go` before the call.
2. **`req.Model` guard**: Add an early empty-model check in `Chat()` with a clear provider-specific error.
3. **`httpClient` field**: Add the field to the struct and allocate it once in the constructor (matching the `OllamaProvider` pattern), then use `p.httpClient` in `Chat()`.Now verify the backend compiles with the fixed file:Go doesn't appear to be installed in this environment. Let me do a careful manual code review to verify correctness against the call sites:The call site in `provider.go` (line 129) expects a single value from `NewLMStudioProvider` — `return NewLMStudioProvider(conn.BaseURL), nil` — and the constructor now returns `*LMStudioProvider` (one value). ✓

Let me do a final read of the fixed file to confirm everything is correct:All three review findings are resolved. Here's a summary of every change made to `backend/internal/provider/lmstudio.go`:

---

### Fix 1 — Constructor signature mismatch (compile error)

**Before:** `func NewLMStudioProvider(baseURL string) (*LMStudioProvider, error)` — two return values, making `return NewLMStudioProvider(conn.BaseURL), nil` in `provider.go` a compile error.

**After:** `func NewLMStudioProvider(baseURL string) *LMStudioProvider` — single return value, matching every sibling constructor (`NewOllamaProvider`, `NewOpenAIProvider`, etc.) and the existing call site in `provider.go`. The empty-URL guard is already done in `providerForConnection()` before this constructor is reached, so the redundant error return is removed.

---

### Fix 2 — Missing `req.Model` guard in `Chat()`

**Added** an early guard at the top of `Chat()`:
```go
if req.Model == "" {
    return nil, fmt.Errorf("lmstudio: model must not be empty")
}
```
This gives a clear, provider-prefixed local error instead of forwarding an empty model string to the LM Studio API and getting an opaque upstream error — consistent with `OllamaProvider` and `OpenAIProvider`.

---

### Fix 3 — Missing `httpClient` field (per-request allocation)

**Before:** Struct had only `baseURL string`; `Chat()` called `newOpenAICompatChatClient()` on every invocation, creating a new `*http.Client` per request and bypassing connection pooling.

**After:** Struct gains `httpClient *http.Client` (matching the ARCH-v0.2.0-013 spec and the `OllamaProvider` pattern). The constructor allocates one client with a 10-minute timeout. `Chat()` now passes `p.httpClient` to `openAICompatChat`, reusing the connection pool across calls. `TestConnection` and `ListModels` still use the helpers' own short-lived clients (30-second context deadlines) — correct, since those operations need a tight timeout, not the 10-minute streaming timeout.