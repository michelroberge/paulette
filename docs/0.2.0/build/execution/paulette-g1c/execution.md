The `ConfigurePage.tsx` already exists with a complete implementation. Let me verify the other components it depends on to make sure everything integrates correctly.The `ConfigurePage.tsx` is already fully implemented from the prior iteration. Let me verify it builds correctly by checking for any TypeScript issues.There's a TypeScript import error. Let me fix it:Build succeeds. The `ConfigurePage.tsx` was already implemented in the prior iteration — I fixed the one TypeScript import error (`CSSProperties` needed a type-only import due to `verbatimModuleSyntax` being enabled).

Here's a summary of what `ConfigurePage.tsx` implements:

**File: `frontend/src/components/configure/ConfigurePage.tsx`**

- **Two-tab layout** — "Connections" and "Stage Defaults" tabs with active state tracked in React state
- **`?tab=` query param support** — reads `connections` or `defaults` from the URL on mount; updates when the URL changes (back/forward navigation)
- **`?highlight=` query param support** — extracts the connection ID and forwards it to `ConnectionsTab` as a `highlight` prop for deep-link pulsing (IACT-011)
- **`← Home` breadcrumb** — `useNavigate()` to `/` with accessible `aria-label`
- **`max-w-4xl` centered layout** — `maxWidth: '56rem'` with auto margin and `2rem 1.5rem` padding
- **Tab strip styling** — active tab gets `border-bottom: 2px solid #3b82f6` and blue text; inactive tabs have transparent border and muted color; active tab margin overlaps the strip border for flush rendering
- **Dark theme** — matches the rest of Paulette's dark UI (`#0f172a` background)
- Integrates `ConnectionsTab` and `StageDefaultsTab` (currently stubs, to be fully implemented in tasks 5.2 and 5.4)