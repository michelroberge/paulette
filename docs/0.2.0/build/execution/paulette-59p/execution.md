The frontend already has significant structure. Let me read the key files to understand exactly what exists and what needs to be added or modified.Let me check the remaining key files:Let me now check the remaining configure components to understand the full state of the implementation:The codebase is already quite complete. Let me run a build to verify everything compiles correctly:The build passes cleanly with no TypeScript errors. Let me do a final audit of each task in the milestone to confirm all acceptance criteria are met:All five tasks in Milestone 4 (Frontend Foundation & Routing) are **fully implemented** in the existing codebase from the prior iteration. Here's the complete verification:

---

## ✅ Task 4.1 — React Router Setup
- `react-router-dom` v7.13.2 is installed in `package.json`
- `BrowserRouter` wraps the app in `main.tsx`
- All three routes are defined in `App.tsx`:
  - `/` → `ProjectListPage`
  - `/projects/:id` → `ProjectDetailPage`
  - `/configure` → `ConfigurePage`
- `?tab=` and `?highlight=` query params are fully parsed in `ConfigurePage.tsx` via `useSearchParams()`

## ✅ Task 4.2 — TypeScript Types (`types/provider.ts`)
All required types are defined:
- `ProviderType` union (7 variants including `github_copilot`)
- `Connection` with `hasCredentials: boolean` (never exposes raw key)
- `ConnectionInput` with `apiKey?: string` for create/update
- `ModelInfo`, `TestResult`, `DeleteResult`
- `StageAssignment`, `GlobalStageConfig`, `ProjectStageConfig`
- Bonus: `ConnectionError` for structured error events from SSE

## ✅ Task 4.3 — API Client: Connections (`api/connections.ts`)
All 8 functions implemented using `apiFetch` helper:
- `listConnections`, `createConnection`, `getConnection`, `updateConnection`, `deleteConnection`
- `testConnection`, `testNewConnection`
- `listModels`, `listModelsForNew`

## ✅ Task 4.4 — API Client: Stage Config (`api/stageConfig.ts`)
All 5 functions implemented:
- `getGlobalDefaults`, `setGlobalStageDefault`
- `getProjectOverrides`, `setProjectStageOverride` (sends `null` to clear override)
- `resetProjectOverrides`

## ✅ Task 4.5 — Home Page "Configure Paulette" Button
- Button appears in the project grid as a card alongside "New Project" and "Import Repo"
- Also exposed as a cog icon in the banner header
- `App.tsx` passes `onConfigure={() => navigate('/configure')}` to `ProjectList`

## ✅ Build Verification
```
✓ built in 2.70s
```
TypeScript compiles clean — no type errors. The chunk size warning is expected due to Monaco Editor and is not an error.