Now I have a complete picture of all 5 issues. Let me fix them systematically:Now fix the `SaveStatusLabel` to use CSS opacity transitions and add the retry link:Now wire the `onRetry` callback into the `SaveStatusLabel` usage in the stage row render, and fix the footer to always show "Reset all":Now fix the footer to always show the "Reset all" button (removing the conditional swap):Now fix the two `React.CSSProperties` usages at the bottom of the file:Now fix the `FieldRow` component that uses `React.ReactNode`:Now verify the final file looks correct end-to-end:The file looks correct. Let me do a quick TypeScript build check to confirm there are no compilation errors:Clean compile. Here's a summary of every change made and the reasoning behind each:

---

### Fixes applied to `ProjectStageSettings.tsx`

**1. `transition-opacity` fade on `SaveStatusLabel` (IACT-010 / SCR-011)**
The component is now **always rendered** (never returns `null`), with `opacity: isVisible ? 1 : 0` and `transition: 'opacity 500ms'` on the inline style. This is the CSS-equivalent of `transition-opacity duration-500`. The 2-second `setTimeout` still triggers the `idle` transition, which causes the browser to smoothly fade the element out — something that was impossible when the component unmounted instantly. A `minWidth: '4.5rem'` reserves layout space so no row-height jitter occurs during the animation.

**2. Retry link on save failure (IACT-010)**
`SaveStatusLabel` now accepts an `onRetry?: () => void` prop. When `status === 'error'`, it renders `"Save failed — retry?"` where "retry?" is an underlined inline `<button>`. At the call site, `onRetry` is passed as `() => saveRow(stage, row.connectionId, row.model)` — the current row already holds the last-attempted values, so no extra state is needed. The retry only appears when `onRetry` is provided and `status === 'error'`, so `'saving'` and `'saved'` states are unaffected.

**3. "Reset all to global defaults" always present in footer (SCR-011)**
The `hasAnyOverride ? <button/> : <p/>` conditional is replaced with an unconditional `<button>`. When there are no overrides, the button is `disabled` and visually muted (`color: '#475569'`, no underline, `cursor: default`, with a tooltip "All stages already inherit global defaults"). The footer DOM structure is now stable — no layout shift after a successful reset. The `transition: 'color 0.2s'` smooths the colour change as overrides are added/removed.

**4. `React.CSSProperties` / `React.ReactNode` without `import React` (fragile namespace)**
Added `import type { CSSProperties, ReactNode } from 'react'` and replaced all three `React.CSSProperties` usages (`baseFieldStyle`, `selectStyle`, `inputStyle`) and the one `React.ReactNode` usage (`FieldRow.children`) with the directly-imported types. This is robust regardless of `jsx` compiler mode or any future tsconfig change.