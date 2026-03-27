Now let me check where `NewOpenAIProvider` is called in the registry:Let me check the current build state:Now I have a complete picture of all three issues. Let me fix them all:

**Issue 1**: Remove the redundant `context.WithTimeout` wrappers in `TestConnection` and `ListModels` — `openAICompatListModels` already applies its own 30-second timeout internally.

**Issue 2**: Add `httpClient *http.Client` as a struct field (initialized once in `NewOpenAIProvider`), reuse it in `Chat()` instead of creating a new client per call.

**Issue 3**: Change `NewOpenAIProvider` to return `(*OpenAIProvider, error)` and validate that `apiKey` is non-empty at construction time.

I also need to fix the call site in `provider.go` — and while there, fix an existing compile error on the `NewLMStudioProvider` call (which already returns two values but is called as `return NewLMStudioProvider(...), nil`).Now fix `Chat()` to use the pooled `p.httpClient`, and remove the redundant `context.WithTimeout` from `TestConnection` and `ListModels`:Now update the call site in `provider.go` — fix both the `NewOpenAIProvider` call (new error return) and the `NewLMStudioProvider` call (existing compile error):Now verify the final state of `openai.go` looks correct end-to-end:All three fixes are in place. Here's a summary of every change made:

---

### `backend/internal/provider/openai.go`

**Fix 1 — Remove double timeout** (`TestConnection` and `ListModels`):
Both methods previously wrapped the caller's context with `context.WithTimeout(ctx, 30*time.Second)` before passing it down. But `openAICompatListModels` (the function both ultimately call) already applies its own `context.WithTimeout(ctx, 30*time.Second)` at line 273 of `openai_compat.go`. The outer wrappers were redundant — and misleading, since changing the callee's timeout would silently leave the caller's override in effect. Both are now removed; the single enforcement point is in `openai_compat.go`.

**Fix 2 — `httpClient` as struct field** (`OpenAIProvider` struct + `Chat`):
Added `httpClient *http.Client` to the struct and initialised it once in `NewOpenAIProvider` via `newOpenAICompatChatClient()`. `Chat()` now passes `p.httpClient` to `openAICompatChat` instead of calling `newOpenAICompatChatClient()` on every invocation. This preserves TCP keep-alive and connection pooling across sequential streaming calls to the same provider.

**Fix 3 — `apiKey` validated at construction time** (`NewOpenAIProvider`):
Constructor signature changed from `func(...) *OpenAIProvider` to `func(...) (*OpenAIProvider, error)`. An empty `apiKey` now returns `fmt.Errorf("openai: apiKey is required")` immediately, consistent with `NewLMStudioProvider`'s `baseURL` validation — rather than deferring the failure to a confusing `401 Unauthorized` at request time. `"time"` import removed (no longer needed).

---

### `backend/internal/provider/provider.go`

**Two call-site fixes** in `providerForConnection`:
- `NewOpenAIProvider(...)` — updated from `return NewOpenAIProvider(...), nil` to `return NewOpenAIProvider(...)` to propagate the new error return.
- `NewLMStudioProvider(...)` — same correction for a pre-existing compile error (`NewLMStudioProvider` already returned two values but the call site had the incorrect `, nil` suffix).