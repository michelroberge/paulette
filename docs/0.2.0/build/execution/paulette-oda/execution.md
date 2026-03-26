Now I have a clear picture. The fix is simple: add `discoveredModels?: ModelInfo[]` to the `Connection` interface, and add explicit type annotations for `m` in `ProjectStageSettings.tsx`.Now fix the implicit `any` type errors in `ProjectStageSettings.tsx`:Now check if `ModelInfo` is imported in `ProjectStageSettings.tsx`:Now run the build again to verify all TypeScript errors are fixed:The build now succeeds. Let me also verify the backend still builds and run the `npm run dev` check:Both issues are now fixed. Let me provide a summary:

## Summary

Two issues were fixed:

### 1. TypeScript compilation errors (prevented `npm run build`)
Three type errors in the frontend source code caused `tsc` to fail before vite even ran:

- **`ConnectionForm.tsx`** and **`ProjectStageSettings.tsx`** referenced `connection.discoveredModels` — a field present in the architecture spec but missing from the `Connection` interface in `types/provider.ts`.
- **`ProjectStageSettings.tsx`** had two `m` callback parameters inferred as `any` since TypeScript couldn't infer the type from the missing field.

**Fixes:**
- Added `discoveredModels?: ModelInfo[]` to the `Connection` interface in `src/types/provider.ts`
- Added `ModelInfo` to the import in `ProjectStageSettings.tsx`
- Added explicit `(m: ModelInfo)` type annotations on the two callback parameters

### 2. Rolldown native binding failure (prevented `npm run build` at runtime)
Vite 8 depends on `rolldown`, which ships platform-specific native `.node` binaries as optional npm packages. npm had cached a broken install where `@rolldown/binding-linux-x64-gnu` was absent.

**Fix:** Removed `node_modules/` and `package-lock.json`, then ran a clean `npm install`. npm correctly resolved and installed the native binding on the fresh install.

Both `npm run build` and `go build` now complete successfully with exit code 0.