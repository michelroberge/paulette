Add React Router to an existing React + TypeScript app, migrating from `{{existingNavPattern}}` to URL-based routing.

## Installation
```bash
{{packageManager}} install react-router-dom
```
Also install types if needed: `{{packageManager}} install -D @types/react-router-dom`

## Routes to implement
{{routes}}

## Migration Steps

### 1. Wrap the app in a Router
In the entry point (`main.tsx` or `index.tsx`), wrap `<App />` with `<BrowserRouter>`:
```tsx
import { BrowserRouter } from 'react-router-dom'
root.render(<BrowserRouter><App /></BrowserRouter>)
```

### 2. Replace state-based navigation in `App.tsx`
- Remove `{{existingNavPattern}}`
- Use `<Routes>` + `<Route>` to render the correct view component per path
- All existing view components remain unchanged — only their mounting logic changes
- Preserve all props/state that existing components received

### 3. Navigation
- Replace any `onClick` state-setters used for navigation with `useNavigate()` hooks or `<Link>` components
- Back/forward browser buttons must work (this is automatic with React Router if done correctly)

{{#if queryParams}}
### 4. Query Param Parsing
For routes that need query params (`{{queryParams}}`), use:
```typescript
import { useSearchParams } from 'react-router-dom'
const [searchParams] = useSearchParams()
const tab = searchParams.get('tab') // e.g. 'connections' | 'defaults' | null
const highlight = searchParams.get('highlight') // e.g. UUID | null
```
Pass these as props into the relevant child components.
{{/if}}

{{#if deepLinkExample}}
### 5. Deep-link support
Verify that navigating directly to `{{deepLinkExample}}` renders the correct view with the correct tab/highlight state active. This requires the Vite dev server (or production server) to serve `index.html` for all routes — add `historyApiFallback: true` to `vite.config.ts` if not already present.
{{/if}}

## Compatibility checklist
- [ ] All existing routes render identically to before
- [ ] Browser back/forward navigation works
- [ ] No TypeScript errors introduced
- [ ] No prop drilling regressions (state that was previously managed in App still works)

Return the updated `App.tsx` (or minimal diff) and any changes to `main.tsx` / `vite.config.ts`.