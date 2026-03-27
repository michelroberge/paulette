Now let me read more context about the existing components to understand patterns:Let me also check the ConnectionErrorBanner for patterns and check if a ConnectionForm exists:Now I have everything I need. Let me implement the full `ConnectionsTab.tsx` and also create `ConnectionForm.tsx` since it's required for ConnectionsTab to be functional:Now let me implement the full `ConnectionsTab.tsx`:Now let's verify the files compile correctly by running a TypeScript check:No TypeScript errors. Let me also do a quick sanity check to make sure the files look right:The build succeeds cleanly. Let me verify the two key files were created correctly:Both files are fully implemented. Here's a summary of what was built:

## `ConnectionsTab.tsx` — Full implementation replacing the stub

**Features delivered:**

- **Connection list** fetched on mount via `listConnections()`, rendered as dark-themed cards with provider-type badge, connection name, default model, and optional base URL
- **Provider badges** colour-coded by type (purple for Ollama, indigo for LM Studio, orange for Anthropic, green for OpenAI, blue for Gemini, grey for Claude CLI)
- **Three inline actions per card**: Test (blue), Edit (grey), Delete (red)
- **Test Connection**: calls the saved connection's test endpoint, renders inline ✓ / ✗ result below the card with model count if discovery succeeded
- **Edit**: opens `ConnectionForm` slide-over pre-filled with the connection's data
- **Delete**: shows a centred confirmation dialog; on confirm calls the API, then surfaces a warning toast listing any affected stage assignments (cascade result from `DeleteResult.affectedStages`)
- **`?highlight=` deep-link**: matching card gets a 3-second red pulsing ring animation (`ct-highlight-pulse` keyframe), scrolls into view, then `clearHighlight()` is called via a ref-guarded timer
- **Empty state**: plug icon SVG + friendly message + "Add Connection" shortcut button

## `ConnectionForm.tsx` — New companion file (required by ConnectionsTab)

**Features delivered:**

- **Slide-over panel** (w-96, fixed right side) with backdrop, slide-in animation, and × close button
- **Provider-type-driven conditional fields** (IACT-007): selecting a type synchronously shows/hides Base URL, API Key, Org ID, Project ID without any round-trip
- **GitHub Copilot** shown as disabled option with "Coming Soon" label
- **Test Connection** button: uses `testConnection(id)` for saved connections with unchanged key, `testNewConnection(input)` otherwise; populates model dropdown on success (IACT-008)
- **Model field**: switches between `<select>` (discovered models) and free-text `<input>` (fallback) 
- **Credential warning banner** shown whenever the API key field is non-empty
- **Save/Cancel footer** with validation (name required, URL required for Ollama/LM Studio, API key required on create for cloud providers)