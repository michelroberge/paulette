Now let me look at some more files to understand the existing structure:Now I have a clear picture of the codebase. Let me install `react-router-dom` and restructure the app for routing:Now let me update `main.tsx` to add `BrowserRouter`:Now let me create a minimal `ConfigurePage` placeholder (the full implementation comes in task 5.1):Now let me do the major restructure of `App.tsx` — splitting it into a `ProjectDetailPage` component and a top-level routing `App`:Now `ProjectList` needs an `onConfigure` prop. Let me check its current interface and add it:Now let me add the "Configure Paulette" button to the project grid in `ProjectList`. Let me find the right location:Now let's verify the build compiles correctly:The build succeeds with no TypeScript errors. Let me also verify the key parts of the implementation are correct:Everything looks correct. Here's a summary of what was implemented:

## Summary

### Changes Made

**`frontend/package.json`** — Added `react-router-dom@^7.13.2` as a dependency (installed via npm).

**`frontend/src/main.tsx`** — Wrapped the app in `<BrowserRouter>` so React Router has a history context throughout the component tree.

**`frontend/src/App.tsx`** — Restructured into three components:
- **`ProjectListPage`** — wraps `<ProjectList>` and uses `useNavigate` to push `/projects/:id` on selection and `/configure` for the configure button.
- **`ProjectDetailPage`** — contains all the existing project detail logic (state, effects, handlers) but now uses `useParams` to get `id` from the URL, loads the project via `getProject(id)`, and uses `navigate('/')` instead of `setProject(null)` for back navigation. Includes loading and error states for the async project fetch.
- **`App`** (default export) — a simple `<Routes>` wrapper with three routes: `/`, `/projects/:id`, `/configure`.

**`frontend/src/components/configure/ConfigurePage.tsx`** *(new file)* — Placeholder component for the `/configure` route. Parses `?tab=` and `?highlight=` query params (used by deep-linking from connection error banners per IACT-011) so those URLs resolve correctly even before the full implementation is wired in task 5.1.

**`frontend/src/components/project/ProjectList.tsx`** — Added optional `onConfigure?: () => void` prop; renders a "Configure Paulette ⚙" card in the project grid when the prop is provided.