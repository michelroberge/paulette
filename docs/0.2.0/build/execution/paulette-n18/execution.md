Now I have a clear picture of all four issues. Let me implement the fixes:

1. **API key validation bug** — replace `!isEdit` with `!(isEdit && connection?.hasCredentials)` inside `handleSave`
2. **Missing `@keyframes spin`** — add it to `SLIDE_IN_STYLE`  
3. **No `onCredentialsSaved` hook** — add optional prop and call it after saves that write credentials
4. **Stale `discoveredModels` field** — remove it from the `Connection` type and stop reading it from the propNow fix the `discoveredModels` initialisation — the backend never returns this field, so the `useState` initialiser and the sync `useEffect` should both be removed: