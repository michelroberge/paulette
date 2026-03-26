Now let me read a few more specific files before implementing:Now I have a complete picture. Let me also check the App.tsx to understand the routing and how ProjectStageSettings is currently opened:Now I have everything needed. Let me create the `ConnectionErrorBanner` component:Now let me verify the file compiles correctly by checking for any TypeScript issues:No TypeScript errors. Let me do a quick sanity check on the file contents and confirm it's well-formed:The `ConnectionErrorBanner` component is complete. Here's a summary of what was implemented:

## `ConnectionErrorBanner.tsx` — What was built

**File:** `frontend/src/components/chat/ConnectionErrorBanner.tsx`

### Exports

**`ConnectionError` interface** — Typed structure for connection error data embedded as JSON in a `StreamEvent{type: 'error'}` content string. The backend encodes `isConnectionError: true`, `connectionId`, `connectionName`, and `reason` so the frontend can distinguish provider failures from generic errors.

**`parseConnectionError(content: string): ConnectionError | null`** — Helper that safely tries to JSON-parse an error event's content and returns a `ConnectionError` if `isConnectionError === true`, or `null` for any other error type (plain-text, unknown). This will be used by `useChat` in task 5.9 when wiring the banner into the chat panel.

**`ConnectionErrorBanner` component** — The visual banner itself (SCR-012), matching the spec precisely:

| Spec element | Implementation |
|---|---|
| `bg-red-50 border border-red-200 rounded-lg p-4 flex items-start gap-3 mx-4 my-2` | ✅ Exact Tailwind classes |
| ⚠ warning icon in `text-red-500` | ✅ |
| `text-red-800 text-sm` connection name + `text-red-700` reason | ✅ |
| **"Go to Configure →"** → `/configure?tab=connections&highlight={id}` | ✅ Uses `useNavigate`, URL-encodes the ID |
| **"Change stage connection"** → opens ProjectStageSettings | ✅ Optional `onOpenProjectSettings` callback; omitting it hides the CTA |
| Banner persists until dismissed/navigated | ✅ No auto-dismiss; optional `onDismiss` prop controls the × button |
| IACT-011 deep-link with highlight param | ✅ `?tab=connections&highlight=` encoded param |