Now I have a clear picture of all the issues. Let me fix them all in `gemini.go`:

1. **Fix misleading struct docstring** — update to accurately describe the SSE format (`&alt=sse`)
2. **Add empty model guard** in `Chat()` 
3. **Read error body** in `Chat()` non-200 path (same as `fetchModels`)
4. **Signal malformed JSON** instead of silently continuing
5. **Send `done` after `error` on context cancellation** so callers draining until `done` don't blockNow fix the `streamResponse` method — malformed JSON should emit an error event, and context cancellation should send `done` after `error`:Now verify the final file looks correct:Now let me also fix the `Chat()` method's docstring which still says "NDJSON response stream":Now let's verify it compiles cleanly:Go isn't installed in this environment, but the code changes are syntactically straightforward. Let me do a final review of the file to confirm everything is correct:All five issues are fixed. Here's a summary of every change made to `backend/internal/provider/gemini.go`:

---

### Fixes applied

**1. Struct docstring corrected** (was misleading, mentioned NDJSON array format)  
The `GeminiProvider` type comment now accurately documents `&alt=sse` as the streaming mechanism and shows the actual SSE line format (`data: {...}`). References to array punctuation characters `[`, `,` are gone.

**2. Empty `req.Model` guard added** (was silent 404)  
`Chat()` now returns an explicit `fmt.Errorf("gemini: model must not be empty")` before building the request. No more silent `...models/:streamGenerateContent` 404 from the API.

**3. Error response body read in `Chat()`** (was opaque "unexpected status 400")  
Non-200 responses now read up to 4 KB of the body — matching the pattern already used in `fetchModels` — so auth failures and quota errors surface the API's own error message.

**4. Malformed JSON emits a diagnostic error event** (was silent `continue`)  
When `json.Unmarshal` fails for a `data:` chunk, `streamResponse` now sends `StreamEvent{Type: "error", Content: "gemini: malformed chunk: ..."}` before continuing. The stream is not aborted (remaining lines still processed), but the caller has diagnostic info.

**5. Context cancellation always sends `done` after `error`** (was potential channel drain deadlock)  
Both the `ctx.Done()` early-exit path and the `scanner.Err()` error path now emit `done` immediately after any `error` event. A caller blocking on `done` will never hang, matching the convention the rest of the codebase expects.