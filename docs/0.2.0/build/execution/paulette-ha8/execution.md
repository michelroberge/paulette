Now I have a clear picture of all the errors. Let me fix them all. The `bead.go` handler still references the old file-backed graph API (`ReadBeadGraph`, `WriteBeadGraph`, `beadGraphLock`) which was replaced by the new bd-CLI-based API in `beads.go`. I need to surgically replace all those references.Now let me make all the fixes systematically:Now I have the full picture. Let me make all the fixes:Good progress. Now let me fix the remaining issues with one script:Now fix the `GetDetail` handler to use `ReadSingleBead`:Now fix the remaining `ReadBeadGraph` calls in `BeadChat`, `GetBeadFiles`, `GetBeadDiff`, and `siblingBeads`:Now let's verify the build:There are more files with the same issues. Let me fix those too:Now I'll fix `instruct.go` and `pipeline.go`:Now check if `pipeline.go` already imports `context`:The backend compiles cleanly. Now let's check if there are any unused imports that need cleanup:The `go vet` warnings about `pipeline.go:117` and the test file are pre-existing (not caused by my changes). Let me verify that the `sync.Mutex` import is still needed in `bead.go`:Good - `sync.WaitGroup` is still used, so the import is correct. Now let's run the tests to confirm everything passes:All tests pass. Let me also verify the frontend still builds:Both backend and frontend build cleanly. Let me do a final comprehensive check:All tests pass. Here's a summary of what was fixed:

## Summary

The backend had a **partially-applied migration** where `repository/fs/beads.go` was fully updated to use the bd-CLI-based API (replacing the file-backed JSON graph), but several handler files still referenced the old, now-deleted functions. This caused 14+ compile errors.

### Root Cause
The old API (`ReadBeadGraph`, `WriteBeadGraph`, `beadGraphLock`, `updateGraphBeadFields`, `updateGraphStatus`) was removed from `beads.go` during migration to bd-based storage, but callers in 4 files were not updated.

### Files Fixed
1. **`handler/bead.go`** — The largest set of changes:
   - Replaced all `fsrepo.ReadBeadGraph()` calls with `fsrepo.ReadSingleBead(ctx, ...)` or `fsrepo.ReadBdBeadGraph(ctx, ...)` 
   - Removed all `beadGraphLock()` references and the now-unnecessary `mu *sync.Mutex` parameters
   - Replaced `updateGraphBeadFields(mu, ...)` with `updateBeadMeta(ctx, ...)` (persists `PostExecutionCommit` to bd notes)
   - Removed `updateGraphStatus(mu, ...)` calls (bd CLI is the authoritative status source)
   - Added new `updateBeadMeta()` helper using `ReadBeadMeta`/`WriteBeadMeta`
   - Renamed `enrichBeadFromGraph()` → `enrichBeadFromBd()` using `ReadSingleBead`
   - Updated `siblingBeads()` to accept a `context.Context` and use `ReadBdBeadGraph`

2. **`handler/instruct.go`** — Replaced `ReadBeadGraph`, removed `beadGraphLock`/`updateGraphBeadFields`, replaced metadata persistence with `WriteBeadMeta`

3. **`handler/pipeline.go`** — Replaced `ReadBeadGraph` with `ReadBdBeadGraph(context.Background(), ...)`, added `context` import

4. **`autopilot/orchestrator.go`** — Replaced `ReadBeadGraph` with `ReadBdBeadGraph(ctx, ...)`