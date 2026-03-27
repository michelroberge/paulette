Create a TypeScript API client module at `api/{{moduleName}}.ts`.

## Imports
```typescript
import { {{importTypes}} } from '../types'
import { {{fetchHelper}} } from './client'
```

## Endpoints to implement
{{endpoints}}

For each endpoint, create an exported async function following this pattern:
```typescript
// GET list example
export async function list{{EntityName}}s(): Promise<ResponseType[]> {
  return {{fetchHelper}}<ResponseType[]>('GET', '{{baseRoute}}')
}

// POST with body example
export async function create{{EntityName}}(input: InputType): Promise<ResponseType> {
  return {{fetchHelper}}<ResponseType>('POST', '{{baseRoute}}', input)
}

// DELETE with result example
export async function delete{{EntityName}}(id: string): Promise<DeleteResult> {
  return {{fetchHelper}}<DeleteResult>('DELETE', `{{baseRoute}}/${id}`)
}

// Sending null to clear/reset
export async function clearOverride(id: string): Promise<void> {
  return {{fetchHelper}}<void>('PUT', `{{baseRoute}}/${id}`, null)
}
```

## Conventions
- Every function is `export async function`
- Path params are template literals: `` `{{baseRoute}}/${id}` ``
- No inline `fetch` calls — always delegate to `{{fetchHelper}}`
- Return type is always explicit — no `any`
- Functions that return nothing use `Promise<void>`
- All error handling is left to the caller (do not catch inside the module)
- Add a JSDoc comment on each function describing what it does and any notable behaviour (e.g. "Send null assignment to clear the override and revert to inherit")

Produce the complete file with all functions.