Create a Go HTTP handler group for `{{entityName}}` in the `{{packageName}}` package, mounted at `{{routePrefix}}`.

## Handler Struct
```go
type {{entityName}}Handler struct {
    store {{storeName}}
    // add other dependencies as needed
}

func New{{entityName}}Handler(store {{storeName}}) *{{entityName}}Handler
```

## Standard CRUD Endpoints
Implement each as a method on `*{{entityName}}Handler`:

| Method | Path | Handler | Notes |
|---|---|---|---|
| GET | `{{routePrefix}}` | `List` | Return array; use `{{responseType}}` if credential redaction needed |
| POST | `{{routePrefix}}` | `Create` | Decode body, delegate to store, return 201 |
| GET | `{{routePrefix}}/{id}` | `Get` | Return single record or 404 |
| PUT | `{{routePrefix}}/{id}` | `Update` | Decode body, delegate to store, return 200 |
| DELETE | `{{routePrefix}}/{id}` | `Delete` | Delegate to store, return JSON result (may include affected references) |

{{#if extraActions}}
## Extra Action Endpoints
Also implement: `{{extraActions}}`
- For each action, add a method and a brief comment describing its behaviour.
{{/if}}

## Conventions
- All responses are `Content-Type: application/json`
- Errors use a helper: `jsonError(w, status, message string)` → `{"error": "message"}`
- Path params read via `chi.URLParam(r, "id")`
- Request bodies decoded with `json.NewDecoder(r.Body).Decode(&input)`; return 400 on decode error
- Store `not found` errors → 404; other errors → 500
- `List` and `Get` must never return raw credentials — use `{{responseType}}` or map to a safe struct
- Include a `RegisterRoutes(r chi.Router)` method that mounts all routes on the provided router

## Error response format
```json
{"error": "human-readable message"}
```

Include all necessary imports (`net/http`, `encoding/json`, `github.com/go-chi/chi/v5`, `fmt`).