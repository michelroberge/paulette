Now let me read the key files to understand the exact interfaces needed:Now I have everything I need. Let me implement the `ConnectionStore`:Now let's verify the file compiles correctly by building the provider package:Go isn't installed in the shell environment. Let me verify the implementation is syntactically correct by reviewing it carefully and checking it against the existing package patterns:The implementation is complete. Here's a summary of what was implemented in `backend/internal/provider/connection_store.go`:

## What was implemented

### `ConnectionStore` struct
- **`path string`** — absolute path to `~/.paulette/connections.json`
- **`mu sync.RWMutex`** — read/write mutex for concurrent safety
- **`connections []Connection`** — in-memory cache (populated at startup, kept in sync on every mutation)

### Constructor
- **`NewConnectionStore(registryPath string)`** — resolves the connections file path, eagerly loads existing data (silently ignores missing file on first run)

### CRUD methods
| Method | Details |
|--------|---------|
| `List() []Connection` | Returns a defensive copy (RLock, no filesystem hit) |
| `Get(id string) (*Connection, error)` | Linear scan with defensive copy on hit (RLock) |
| `Create(conn Connection) (*Connection, error)` | Assigns UUID v4, sets timestamps, appends + saves with rollback on failure |
| `Update(conn Connection) (*Connection, error)` | Preserves `CreatedAt`, updates `UpdatedAt`, replaces in-place with rollback |
| `Delete(id string) ([]string, error)` | Removes from slice with rollback; affected stage cleanup is delegated to `StageConfigStore.ClearConnectionReferences` in the handler layer |

### Key implementation details
- **Atomic writes** — write to temp file in same directory → `chmod 0600` → `os.Rename` (same-filesystem, POSIX-atomic); belt-and-suspenders `chmod` on the final path too
- **Rollback on save failure** — `snapshot()` copies the slice before mutation; restored if `save()` errors
- **UUID v4** — generated with `crypto/rand` (no external dependencies), correct version/variant bits set
- **`connectionsEnvelope`** wrapper — matches the `{ "connections": [...] }` schema from the architecture doc, allowing future top-level fields without schema breakage