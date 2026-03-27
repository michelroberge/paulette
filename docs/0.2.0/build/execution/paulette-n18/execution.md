Now I have a clear picture of all four issues. Let me implement the fixes:Now fix the `discoveredModels` stale references in `ConnectionForm.tsx` — remove the `connection?.discoveredModels` init and the effect that tries to sync from it (the backend never returns this field):Now remove the unused `useEffect` import since we no longer need it in `ConnectionForm.tsx` — first let me check if `useEffect` is still used elsewhere in the file:`useEffect` is now unused. Remove it from the import:Now fix `ConnectionsTab.tsx` — two changes:
1. Add `if (saved.hasCredentials) setShowCredToast(true)` to `handleSaved` (direct call-site hook).
2. Remove the `discoveredModels` mutation on the connection object (stale field with no backend backing).Now remove the `discoveredModels` mutation in the test handler (stale field not in `Connection` type):Now verify the final state of both files compiles cleanly:Clean — zero TypeScript errors. Let me do a final review of the two changed files to confirm all fixes landed correctly:All four fixes are in place. Here's a summary of every change made:

---

### Fixes applied

**1. Edit-mode API key validation bug — `ConnectionForm.tsx` line ~214**

Replaced `!isEdit` guard with `!hasExistingCreds` (computed inline inside `handleSave`):

```diff
- if (providerNeedsApiKey(form.providerType) && !isEdit && !form.apiKey.trim()) {
+ const hasExistingCreds = isEdit && connection?.hasCredentials && !form.apiKey.trim();
+ if (providerNeedsApiKey(form.providerType) && !hasExistingCreds && !form.apiKey.trim()) {
```

The old `!isEdit` was too broad — it allowed saving a connection that had its provider type switched from Ollama (no credentials) to OpenAI/Anthropic/Gemini without ever entering an API key. The new guard correctly exempts only connections that *already have credentials stored on disk* and whose key field was left blank.

---

**2. Spinner `@keyframes spin` not self-contained — `ConnectionForm.tsx`**

Added `@keyframes spin` to the `SLIDE_IN_STYLE` string that the component injects into the DOM. The spinner element's `animation: 'spin 0.7s linear infinite'` now has its keyframe defined in the same scope rather than relying on a global rule in `App.css`.

---

**3. No `handleSaved` call-site hook for credential toast — `ConnectionsTab.tsx`**

Added `if (saved.hasCredentials) setShowCredToast(true)` directly inside `handleSaved`. This is the call-site integration the review found missing: the toast is now fired via the `onSaved` return value, acting as a defense-in-depth layer alongside the existing `onCredentialsSaved` prop wiring.

---

**4. Stale `discoveredModels` field with no backend backing — `ConnectionForm.tsx` + `ConnectionsTab.tsx`**

- **`ConnectionForm.tsx`**: Replaced `useState<ModelInfo[]>(connection?.discoveredModels ?? [])` with a plain `useState<ModelInfo[]>([])` and removed the `useEffect` that tried to sync from `connection.discoveredModels`. Removed the unused `useEffect` import. Added a comment explaining why the prop is not consulted (backend `ConnectionResponse` never returns model lists).
- **`ConnectionsTab.tsx`**: Removed the `setConnections` mutation that was spreading `discoveredModels: result.models` back onto the connection object after a test. Replaced it with a comment explaining the intentional omission. This eliminates the confusing phantom property that only existed in frontend memory and could never survive a page reload.