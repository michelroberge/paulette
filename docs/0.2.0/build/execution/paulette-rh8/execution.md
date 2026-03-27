Now let me read the provider types and how the ChatPanel is used in context:Let me look at how these components are used and how the App.tsx is set up:Let me check the ProjectStageSettings component and the ProjectHeader to see what's already wired up:Now I have a complete picture. Let me implement the three changes needed:

1. **`useChat.ts`** — track `connectionError` state, parse connection errors from stream events
2. **`ChatPanel.tsx`** — render `ConnectionErrorBanner` in place of the streaming indicator  
3. **`App.tsx`** — wire the new state/props through to `ChatPanel`Now update the `error` case in `handleEvent` to distinguish connection errors from generic errors:Now update `send` and `resume` to clear the banner on retry, and expose `connectionError` in the return value:Now update `ChatPanel.tsx` to accept the new props and render the banner:Now replace the streaming indicator block to handle both cases (connection error banner vs. normal streaming):The textarea is already disabled only during `streaming`. But we should also allow sending when there's a connection error (so user can retry after fixing). The current `disabled={streaming}` is already correct — when a connection error occurs, `streaming` is false, so the input stays enabled. However, let me also make the submit button behavior consistent when a connection error is present vs. during streaming:Good — the textarea is already enabled when `streaming` is false (which it will be during a connection error). Now wire up `App.tsx`:Now add the `showStageSettings` state alongside the other modal state variables:Update the `useChat` destructuring to include `connectionError`:Pass `connectionError` and `onOpenProjectSettings` to `ChatPanel`:Add the `ProjectStageSettings` slide-over alongside the existing modals at the bottom of the JSX:Now let's verify the TypeScript build to make sure everything compiles cleanly:Build passes with zero TypeScript errors. Let me do a quick review of all the changed files to confirm correctness:Everything looks correct. Here's a summary of all the changes made:

---

## What was implemented

### `frontend/src/hooks/useChat.ts`
- **Imported** `ConnectionError` type and `parseConnectionError` helper from the existing `ConnectionErrorBanner` component
- **Added** `connectionError: ConnectionError | null` state (initially `null`)
- **Modified** the `error` event handler with two paths:
  - *Connection error* (structured JSON with `isConnectionError: true`): sets `connectionError` state, does **not** add a message to the chat history — the banner is the sole UI for this error type
  - *Generic error* (plain text, malformed JSON, missing fields): existing behavior unchanged — appends an error message to the messages list
- **`send()`**: clears `connectionError` before submitting so the banner dismisses automatically when the user retries
- **`resume()`**: same clear on reconnect attempt
- **Return value**: exposes `connectionError` alongside the existing fields

### `frontend/src/components/chat/ChatPanel.tsx`
- **Imported** `ConnectionErrorBanner` and `ConnectionError`
- **Added** optional props `connectionError` and `onOpenProjectSettings`
- **Replaced** the streaming indicator block with a three-way render:
  1. `connectionError` set → `ConnectionErrorBanner` (with deep-link "Go to Configure →" and "Change stage connection" CTA)
  2. `streaming` true → existing animated streaming indicator
  3. Neither → `null`
- The textarea remains **enabled** during a connection error (since `disabled={streaming}` and `streaming` is false after the error), so the user can type and send after fixing the connection

### `frontend/src/App.tsx`
- **Imported** `ProjectStageSettings`
- **Added** `showStageSettings` state
- **Destructured** `connectionError` from `useChat`
- **Passed** `connectionError` and `onOpenProjectSettings={() => setShowStageSettings(true)}` to `ChatPanel`
- **Rendered** `ProjectStageSettings` slide-over when `showStageSettings` is true, enabling the "Change stage connection" shortcut from the error banner to work without navigating away from the project