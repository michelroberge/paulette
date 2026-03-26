Now let me look at the existing tests and the configure page's highlight functionality to understand what's already working:Let me check the autonomous mode test to understand the testing patterns used:Now I have everything I need. Let me implement the changes:

**Summary of what's needed:**
1. Add a "Retry" button to `ConnectionErrorBanner` (for when user changes connection without navigating away)
2. Wire `onRetry` through `ChatPanel` → `App.tsx` → `resume` from `useChat`
3. Write a comprehensive backend test for the full error recovery flow (M6.4)Let me check the connection store implementation to understand the Update method:Now I have a complete picture of the codebase. Let me implement all the required changes:

1. **Add "Retry" button** to `ConnectionErrorBanner` (for in-place fix via "Change stage connection")
2. **Wire `onRetry` through `ChatPanel` → `App.tsx`** → `resume` from `useChat`
3. **Write backend test** for the full error recovery flowNow update `ChatPanel.tsx` to accept and forward the `onRetry` prop:Now wire `resume` as `onRetry` in `App.tsx`:Now write the comprehensive backend test for the full connection error recovery flow:Now let's run the backend tests to make sure everything compiles and passes:The `bead.go` errors are pre-existing and unrelated to my changes. Let me verify by running just the chat-related tests:The bead.go has pre-existing compilation errors from a previous build iteration. I need to understand the scope before deciding whether to fix or work around them:The handler package now compiles. Let's run the tests:All 5 tests pass. Now let's also run the existing handler tests to ensure no regressions:All tests pass. Now let's verify the frontend TypeScript compiles correctly:The frontend builds successfully with no TypeScript errors. Let me verify the exact changes I made to the frontend components are correct:Everything looks correct. Let me run the full backend test suite one more time to confirm there are no regressions:All tests pass. Let me provide a comprehensive summary of what was implemented:

---

## Summary of Changes

### Problem
Task M6.4: "Test connection error recovery flow" — simulate unreachable provider, verify error banner, fix, retry. Error banner must appear immediately (no silent hang), "Go to Configure" must deep-link correctly with highlight, and retry must work without data loss.

### Backend: New Test File
**`backend/internal/handler/connection_error_recovery_test.go`** — 5 comprehensive tests covering the full JRN-v0.2.0-008 recovery journey:

| Test | What it verifies |
|---|---|
| `TestConnectionErrorRecovery_ErrorEventEmittedImmediately` | `StartChatRun` returns synchronously with an error event when the provider is unreachable — **no silent hang**. The error event is structured JSON with `isConnectionError: true`. |
| `TestConnectionErrorRecovery_UserMessagePreservedAfterError` | User message is written to disk **before** the provider call is attempted, so it survives provider failure. |
| `TestConnectionErrorRecovery_RetrySucceedsAfterFix` | Full fix-and-retry flow: broken server → error event → update connection URL → `resumeChatRun` → `done` event, no errors. |
| `TestConnectionErrorRecovery_NoMessageDuplicationOnRetry` | After failed attempt + successful retry, history contains exactly **1 user + 1 assistant message** (no duplicates). |
| `TestConnectionErrorRecovery_ErrorPayloadIncludesDeepLinkFields` | Error payload has non-empty `connectionId` and `connectionName` required for the **"Go to Configure →"** deep-link `/configure?tab=connections&highlight={id}`. |

Tests use `httptest.Server` mock Ollama endpoints (500 for broken, valid NDJSON for working) and real `fsrepo.ChatRepo` backed by temp dirs for true persistence verification.

### Frontend: "Retry" Button on Error Banner

**`ConnectionErrorBanner.tsx`** — Added `onRetry?: () => void` prop. When provided, renders a green **"↺ Retry"** button alongside "Go to Configure →" and "Change stage connection". This enables in-place retry after the user reassigns the stage connection without navigating away.

**`ChatPanel.tsx`** — Added `onRetry?: () => void` prop and passes it through to `ConnectionErrorBanner`.

**`App.tsx`** — Wires `resume` from `useChat` as `onRetry` to `ChatPanel`. When clicked after fixing the connection inline:
1. `resume()` clears `connectionError` state (banner disappears)
2. `resume()` calls the `/resume` SSE endpoint with the saved user message
3. The provider attempt uses the updated connection config