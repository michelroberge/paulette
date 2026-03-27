Now let me check the correct model IDs used in the rest of the codebase:The pattern is clear — the entire codebase uses `claude-sonnet-4-6` and `claude-opus-4-6`. Now I'll fix the three issues in `provider.go`:

1. **Model ID mismatch**: Update `stageModels` map and the fallback to use `-4-6` versions
2. **Dead `projectRepo` parameter**: Remove the unused `repository.ProjectRepo` parameter (and its import)
3. **Comment fix**: Update the `ChatRequest.Model` comment which mentions `claude-sonnet-4-5`Now remove the unused `projectRepo` parameter from `ResolveForStage` and update its signature and doc comment:Now fix the fallback model in the `!ok` case at the bottom of `ResolveForStage`:Now let me check if `ResolveForStage` is called anywhere else in the codebase so we can update those call sites to drop the removed `projectRepo` argument:Good — `ResolveForStage` isn't called elsewhere yet (the handler modification is a separate task). Let me do a final read of the updated file to verify correctness:The file looks correct. All four review findings have been addressed:

---

**Summary of changes made to `backend/internal/provider/provider.go`:**

1. **Model ID mismatch (bug) — fixed:** Updated all entries in `stageModels` from `"claude-sonnet-4-5"` / `"claude-opus-4-5"` to `"claude-sonnet-4-6"` / `"claude-opus-4-6"`, matching the values used consistently throughout `handler/chat.go`, `agent/beads.go`, `agent/import.go`, `agent/skills.go`, and `agent/summary.go`. The same fix was applied to the `!ok` fallback default at the bottom of `ResolveForStage`. Also updated the example in the `ChatRequest.Model` doc comment.

2. **Dead `projectRepo` parameter removed:** Removed `projectRepo repository.ProjectRepo` from `ResolveForStage`'s signature (it was unused per the body comment "available for future use"). This also allowed removing the `repository` package import entirely, eliminating a cross-package import dependency with no current value. Callers will simply omit the argument when they're implemented in task 3.5/3.6.

3. **`repository` import removed:** The import block now only contains `context`, `errors`, `fmt`, `agent`, and `model` — all of which are actually used.

4. **Doc comment cleaned up:** The `projectRepo` line was removed from the `ResolveForStage` godoc since the parameter no longer exists. The three-level fallback documentation remains intact and accurate.