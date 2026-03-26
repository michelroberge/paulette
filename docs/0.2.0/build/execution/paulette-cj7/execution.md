Now I have a full picture. I'll make three targeted fixes:

1. **Move `ConnectionError` to `types/provider.ts`** — canonical home for provider types
2. **Add field-presence validation in `parseConnectionError`** — guard against blank/missing fields
3. **Remove `onDismiss` prop and close button** — spec says banner never auto- or manually dismissesNow rewrite `ConnectionErrorBanner.tsx` — removing the local `ConnectionError` definition, adding the import from `types/provider.ts`, adding field-presence validation, and stripping `onDismiss`:Now verify the TypeScript compiles cleanly:Clean compile. Let me do a quick sanity-check on the final state of both changed files:`onDismiss` and the `×` button are completely gone. Here's a summary of every change made:

---

### Changes applied

**`frontend/src/types/provider.ts`** — added `ConnectionError` interface
The interface (`isConnectionError`, `connectionId`, `connectionName`, `reason`) is now the canonical location in the provider types file, alongside the other provider-related types. Task 5.9 (ChatPanel integration) and any future callers can import it from here without depending on the component file.

**`frontend/src/components/chat/ConnectionErrorBanner.tsx`** — three fixes:

1. **`onDismiss` removed** — the prop, its JSDoc, and the `×` close button are entirely gone. The banner now strictly persists until the user navigates away or retries, matching SCR-012's "does not auto-dismiss" requirement. The `ConnectionError` interface in `types/provider.ts` is now the only definition; the banner re-exports it via `export type { ConnectionError }` so existing callers that imported it from the banner file continue to work without changes.

2. **Field-presence validation added to `parseConnectionError`** — after confirming `isConnectionError === true`, the function now explicitly validates that `connectionId`, `connectionName`, and `reason` are all present, are strings, and are non-empty (`.trim() !== ''`). A backend bug or unexpected payload shape returns `null` rather than an object with blank fields.

3. **`encodeURIComponent` removed from the deep-link** — UUIDs contain no characters that need URL encoding; the URL now matches the spec's literal pattern `/configure?tab=connections&highlight={id}` exactly.