Create a Go file-persisted store for `{{entityName}}` records in the `{{packageName}}` package.

## Requirements

### Struct
```go
type {{entityName}}Store struct {
    path  string           // {{filePath}}
    mu    sync.RWMutex
    items []{{entityName}}
}

func New{{entityName}}Store(basePath string) *{{entityName}}Store
```

### CRUD Methods
- `List() []{{entityName}}` — returns a copy of all records (read lock)
- `Get(id string) (*{{entityName}}, error)` — returns a single record by `{{idField}}`
- `Create(item {{entityName}}) (*{{entityName}}, error)` — assigns a new UUID to `{{idField}}`, sets `CreatedAt`/`UpdatedAt`, appends, saves
- `Update(item {{entityName}}) (*{{entityName}}, error)` — finds by `{{idField}}`, replaces, sets `UpdatedAt`, saves
- `Delete(id string) ({{#if extraDeleteReturn}}{{extraDeleteReturn}}, {{/if}}error)` — removes by `{{idField}}`{{#if extraDeleteReturn}}, returns `{{extraDeleteReturn}}` representing any references that were cleaned up{{/if}}, saves

### File Persistence (`save()` — private)
1. Marshal the in-memory slice to JSON with `json.MarshalIndent`
2. Write to a temp file in the same directory (`*.tmp`)
3. `os.Rename(tmp, path)` for atomic swap
4. `os.Chmod(path, {{filePermissions}})` after rename
5. Return any error from these steps

### Loading
On `New{{entityName}}Store`, attempt to read and unmarshal the file. If the file does not exist (`os.IsNotExist`), start with an empty slice — do not return an error.

### Concurrency
Use `sync.RWMutex`: `RLock/RUnlock` for reads (`List`, `Get`), `Lock/Unlock` for writes (`Create`, `Update`, `Delete`, `save`).

### Error handling
- Return a descriptive `fmt.Errorf` when record is not found
- Wrap file I/O errors with context
- Panic is never acceptable; all errors are returned

Include all necessary imports (`sync`, `os`, `encoding/json`, `path/filepath`, `time`, `fmt`, `github.com/google/uuid` or `crypto/rand` for UUID generation).