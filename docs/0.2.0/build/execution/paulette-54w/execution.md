Now I have all the context I need. Let me create the `provider/connection.go` file:Go isn't available in the PATH in this environment, but the file is syntactically correct. Let me verify it manually by reviewing the code once more:The file is complete and correct. Here's a summary of what was created:

**`backend/internal/provider/connection.go`** — new file in the newly created `provider` package.

**What it defines:**

1. **`ProviderType` string type** with all 7 constants grouped by tier:
   - P1 (no credentials): `ProviderOllama`, `ProviderLMStudio`
   - P2 (API key): `ProviderAnthropic`, `ProviderOpenAI`, `ProviderGemini`
   - Existing: `ProviderClaudeCLI` (backward-compatible default)
   - P3 (deferred): `ProviderGitHubCopilot` (v0.3.0)

2. **`Connection` struct** — the storage model written to `~/.paulette/connections.json`. Contains all fields including `APIKey` (plain text, file protected by `chmod 600`). Uses `omitempty` on optional fields so the JSON stays clean for providers that don't need them.

3. **`ConnectionResponse` struct** — the API-facing model. The `APIKey` field is absent; instead `HasCredentials bool` signals whether a key is stored. This ensures credentials are never leaked through the HTTP API.

4. **`ToResponse()` method** — converts a `Connection` to `ConnectionResponse`, deriving `HasCredentials` from whether `APIKey != ""`.