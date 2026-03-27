Now I have enough context. Let me create the `CredentialWarningToast.tsx` component:Now let me verify the component integrates correctly with the existing `ConnectionForm.tsx` which already references `onCredentialsSaved`:The `handleSave` function doesn't yet call `onCredentialsSaved`. I need to fix that, and also check how `ConnectionsTab` uses the form so the toast can be wired up at the tab level:Now I need to do two things:
1. Wire `onCredentialsSaved` call inside `ConnectionForm.tsx`'s `handleSave`
2. Wire up `CredentialWarningToast` in `ConnectionsTab.tsx`Now I have the full picture. Let me make the two surgical edits:

**1. Fix `ConnectionForm.tsx` — call `onCredentialsSaved` after a successful save when credentials were involved:****2. Wire `CredentialWarningToast` into `ConnectionsTab.tsx`:**Now let's verify the build compiles cleanly:Build succeeds with no TypeScript errors. Here's a summary of what was implemented:

## `CredentialWarningToast.tsx` — Implemented

**New file:** `frontend/src/components/configure/CredentialWarningToast.tsx`

The component implements the full credential warning toast (IACT-009 / ARCH-v0.2.0-028):

- **Fixed `bottom-4 right-4` positioning** with `z-50` — always above other content
- **Yellow warning styling** matching the spec (`bg-yellow-50`, `border-yellow-300`, `text-yellow-800`, `shadow-lg`)
- **Auto-dismisses after 6 seconds** with a 300ms CSS `opacity` fade-out transition  
- **Manual dismiss button** (×) for immediate closure, also with fade-out
- **`visible` / `onDismiss` prop interface** — parent controls when to show, component manages its own animation lifecycle via `useRef` timers and `requestAnimationFrame` for smooth CSS transitions
- **`role="status"` + `aria-live="polite"`** for accessibility
- Warning message clearly states credentials are saved to `~/.paulette/connections.json` (user-readable only), as specified

**Wired into `ConnectionsTab.tsx`:**
- Added `showCredToast` state; passes `onCredentialsSaved={() => setShowCredToast(true)}` to `ConnectionForm`
- Renders `<CredentialWarningToast>` at the tab root

**Fixed `ConnectionForm.tsx`:**
- `handleSave` now calls `onCredentialsSaved?.()` after a successful save when the provider requires API keys (`anthropic`, `openai`, `gemini`) and a key was entered — never fires for Ollama, LM Studio, or Claude CLI