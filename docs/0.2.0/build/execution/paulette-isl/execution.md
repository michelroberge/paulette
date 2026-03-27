Good. The chmod 600 logic is already implemented. Let me read the key files directly to verify the implementation is complete and correct, then run the existing tests.The implementation is already complete. Let me run the tests to verify they all pass.All 12 chmod 0600 tests pass. Let me also run the full provider package test suite to confirm nothing is broken:All tests pass. Here's a summary of what was verified:

## Task Complete — `chmod 600` on `connections.json`

The `chmod 0600` protection was already fully implemented in `backend/internal/provider/connection_store.go` with a belt-and-suspenders approach in the `save()` method:

### Implementation (lines 292–305 of `connection_store.go`)

```go
// chmod before rename — sensitive data is never world-readable, even transiently.
if err := os.Chmod(tmpName, 0600); err != nil {
    return fmt.Errorf("chmod temp file: %w", err)
}

if err := os.Rename(tmpName, s.path); err != nil {
    return fmt.Errorf("rename to %s: %w", s.path, err)
}

committed = true
// Belt-and-suspenders: also chmod the final path in case the rename
// preserved a different mode from the destination directory's umask.
_ = os.Chmod(s.path, 0600)
```

**Two layers of protection:**
1. `chmod 0600` is applied to the **temp file before the atomic rename** — so credentials are never exposed even transiently
2. `chmod 0600` is re-applied to **the final path after rename** — guards against umask or filesystem quirks

### Tests — All 12 pass ✅

| Test | Scenario |
|---|---|
| `Create_FileMode0600` | Basic create |
| `Create_APIKey_FileMode0600` | Create with API key (sensitive) |
| `Create_Multiple_FileMode0600` | 3 sequential creates |
| `Update_FileMode0600` | Update without credentials |
| `Update_AddAPIKey_FileMode0600` | Update adding API key |
| `Delete_FileMode0600` | Delete one of two connections |
| `Delete_LastConnection_FileMode0600` | Delete final connection (empty array remains) |
| `Delete_WithAPIKey_FileMode0600` | Delete when remaining connection has API key |
| `FullLifecycle_FileMode0600` | Create → Update → Delete sequence |
| `ConcurrentCreates_FileMode0600` | 8 concurrent goroutines |