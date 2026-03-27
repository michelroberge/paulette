The `ForConnection()` method is already implemented. Let me verify all the referenced provider constructors actually exist:All provider files exist. Let me verify each constructor signature to confirm they match what `providerForConnection` calls:I notice `NewLMStudioProvider` returns only one value but `providerForConnection` tries to use it as `(Provider, error)`. Let me check the lmstudio.go file and try to build:I can see the issue. `NewLMStudioProvider` returns a single `*LMStudioProvider` value, but `providerForConnection` returns `(Provider, error)`. The `return NewLMStudioProvider(conn.BaseURL)` call on line 129 is missing the `nil` error return and won't compile. I need to fix this.Now let me verify the complete `providerForConnection` method looks correct end-to-end:The factory method is complete and correct. Let me also do a quick cross-check of all provider constructors to confirm the return types are compatible:All return types are now correctly handled in `providerForConnection`:

| Provider | Constructor returns | Factory call |
|---|---|---|
| `claude_cli` | `*ClaudeCLIProvider` | `return NewClaudeCLIProvider(), nil` ✓ |
| `ollama` | `*OllamaProvider` | `return NewOllamaProvider(...), nil` ✓ |
| `lmstudio` | `*LMStudioProvider` | `return NewLMStudioProvider(...), nil` ✓ (fixed) |
| `openai` | `(*OpenAIProvider, error)` | `return NewOpenAIProvider(...)` ✓ (multi-return passthrough) |
| `anthropic` | `*AnthropicProvider` | `return NewAnthropicProvider(...), nil` ✓ |
| `gemini` | `*GeminiProvider` | `return NewGeminiProvider(...), nil` ✓ |
| `github_copilot` | — | `return nil, fmt.Errorf("...not yet supported")` ✓ |
| unknown | — | `return nil, fmt.Errorf("unknown provider type: %q", ...)` ✓ |

The fix was a single-line correction: `return NewLMStudioProvider(conn.BaseURL)` → `return NewLMStudioProvider(conn.BaseURL), nil`. The `LMStudioProvider` constructor returns only `*LMStudioProvider` (no error), so the `, nil` was needed to satisfy the `(Provider, error)` return signature of `providerForConnection`.

The `Registry.ForConnection()` and `providerForConnection()` factory method are now fully wired:
- All 6 active provider types (`claude_cli`, `ollama`, `lmstudio`, `openai`, `anthropic`, `gemini`) instantiate correctly
- `github_copilot` returns a clear "not yet supported" error message pointing users to v0.3.0
- Unknown provider types return a descriptive error with the unrecognised type quoted
- `ollama` and `lmstudio` guard against empty `BaseURL` before constructing
- `ResolveForStage` implements the 3-level fallback (project override → global default → Claude CLI hardcoded) intact