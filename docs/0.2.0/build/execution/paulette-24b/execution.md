Now I understand the issues clearly. Let me fix the `ConfigurePage.tsx`:

1. **URL not updated on tab click** — `setSearchParams` is never called; tab state is local-only
2. **`?highlight=` persists forever** — clearing it when switching tabs requires calling `setSearchParams`
3. **No ARIA role/selected attributes** on tab buttons
4. **Dual state problem** — keeping both local `useState` and URL in sync via `useEffect` is fragile; the URL should be the single source of truthNow replace the `ConfigurePage` function body — remove the dual `useState`/`useEffect` pattern and make the URL the single source of truth, and wire `setSearchParams` to tab clicks:Now I need to check `ConnectionsTab.tsx` to update its props interface to accept the new `clearHighlight` callback:Now let's verify the final state of both files and do a TypeScript build check:TypeScript check passes cleanly. Here's a summary of every change made:

---

## Changes made

### `frontend/src/components/configure/ConfigurePage.tsx`

**1. Removed dual state — URL is now the single source of truth**
- Deleted the `useState<TabId>` and the `useEffect` that tried to keep it in sync with the URL. Those were the root cause of tabs going stale.
- `activeTab` is now a plain `const` derived from `searchParams.get('tab')` on every render — no synchronisation needed, and Back/Forward automatically works because the component just re-reads the URL.

**2. Tab clicks now update the URL via `setSearchParams`**
- `handleTabClick(tab)` calls `setSearchParams({ tab })`, which pushes a new history entry and re-renders with the correct tab.
- The `?highlight=` param is **intentionally omitted** from `setSearchParams({ tab })` — any tab switch drops it, so the pulse card won't re-fire when the user navigates back to Connections.
- A no-op guard (`if (tab === activeTab && !highlightParam) return`) prevents redundant history entries.

**3. `clearHighlight` callback passed to `ConnectionsTab`**
- `ConnectionsTab` receives `clearHighlight={() => setSearchParams({ tab: 'connections' })}` so it can proactively drop `?highlight=` from the URL once the 3-second pulse animation fires, even without the user switching tabs.

**4. Full ARIA tab semantics on `TabButton`**
- Added `role="tab"`, `aria-selected={active}`, `aria-controls={`tabpanel-${id}`}`, and `tabIndex={active ? 0 : -1}`.
- The wrapping `div` is already `role="tablist"`.
- The tab panel `div` now has `role="tabpanel"`, `id={`tabpanel-${activeTab}`}`, and `aria-labelledby={`tab-${activeTab}`}`.

### `frontend/src/components/configure/ConnectionsTab.tsx`

- Added `clearHighlight?: () => void` to `ConnectionsTabProps` (with JSDoc explaining its contract).
- The stub destructs the new prop as `_clearHighlight` so the interface is ready for task 5.2 to wire in without further changes to `ConfigurePage`.