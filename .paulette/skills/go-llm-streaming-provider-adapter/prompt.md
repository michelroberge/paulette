Create a Go `{{providerName}}Provider` in the `{{packageName}}` package that implements the `Provider` interface.

## Provider Struct
```go
type {{providerName}}Provider struct {
    baseURL    string // default: "{{defaultBaseURL}}"
    httpClient *http.Client
    {{extraFields}}
}
```

## Interface Methods

### `Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)`
1. Build the provider-specific request body from `ChatRequest` (convert `History` + `UserMessage` to the provider's message array format)
2. Set auth: **{{authMechanism}}** — {{#if authHeaderOrParam}}using `{{authHeaderOrParam}}`{{/if}}
3. POST to `{{chatEndpoint}}` with a 10-minute timeout context
4. Launch a goroutine that reads the response body and converts to `StreamEvent`:

**Streaming format: `{{streamingFormat}}`**
{{#if (eq streamingFormat 'ndjson')}}
- Read line-by-line with `bufio.Scanner`
- Unmarshal each JSON line
- Map content field → `StreamEvent{Type: "chunk", Content: ...}`
- On done sentinel → `StreamEvent{Type: "done"}`
{{/if}}
{{#if (eq streamingFormat 'openai_sse')}}
- Read SSE lines (`data: {...}` prefix)
- Skip `data: [DONE]` lines with a `done` event
- Unmarshal JSON, extract `choices[0].delta.content` → `StreamEvent{Type: "chunk"}`
{{/if}}
{{#if (eq streamingFormat 'anthropic_sse')}}
- Parse typed SSE events (`event:` line followed by `data:` line)
- `content_block_delta` event → `StreamEvent{Type: "chunk", Content: delta.text}`
- `message_stop` event → `StreamEvent{Type: "done"}`
{{/if}}
{{#if (eq streamingFormat 'gemini_ndjson_array')}}
- Read line-by-line; buffer to handle chunked JSON array entries
- Extract `candidates[0].content.parts[0].text` → `StreamEvent{Type: "chunk"}`
- Empty line or array close → `StreamEvent{Type: "done"}`
{{/if}}
5. On any HTTP or parse error, send `StreamEvent{Type: "error", Content: err.Error()}` and close the channel

### `TestConnection(ctx context.Context) error`
- Use a 30-second timeout
- Send a minimal probe to verify connectivity and auth (prefer a cheap endpoint like model list or a 1-token message)
- Return nil on 2xx, descriptive error otherwise

### `ListModels(ctx context.Context) ([]ModelInfo, error)`
{{#if (eq modelListEndpoint 'unsupported')}}
- Return `nil, ErrModelListUnsupported`
{{else if (eq modelListEndpoint 'hardcoded')}}
- Return a hardcoded `[]ModelInfo` slice of known models for this provider
{{else}}
- GET `{{modelListEndpoint}}` with auth
- Parse response → `[]ModelInfo{ID: ..., Name: ...}`
{{/if}}

## Error handling
- Never panic; all errors are returned or sent as error StreamEvents
- Wrap HTTP errors with provider name for context: `fmt.Errorf("{{providerName}}: %w", err)`
- Close the StreamEvent channel in a `defer` inside the goroutine

Include all necessary imports (`net/http`, `bufio`, `encoding/json`, `context`, `fmt`, `io`, `strings`).