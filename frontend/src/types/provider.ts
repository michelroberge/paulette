/**
 * TypeScript types for the pluggable LLM provider layer (Paulette v0.2.0).
 *
 * These types mirror the backend API contract defined in
 * backend/internal/provider/connection.go and backend/internal/provider/stage_config.go.
 *
 * Key design rules:
 *   - `Connection` (API response) uses `hasCredentials: boolean` — never exposes the raw key.
 *   - `ConnectionInput` (create / update request) includes `apiKey?: string` for the actual secret.
 *   - `StageAssignment` uses `null` to represent "inherit global default" in project overrides.
 */

import type { StageName } from './index';

// ---------------------------------------------------------------------------
// Provider types
// ---------------------------------------------------------------------------

/**
 * All supported LLM backend provider types.
 * `github_copilot` is defined but disabled ("coming soon") in v0.2.0.
 */
export type ProviderType =
  | 'ollama'
  | 'lmstudio'
  | 'anthropic'
  | 'openai'
  | 'gemini'
  | 'claude_cli'
  | 'github_copilot';

// ---------------------------------------------------------------------------
// Connection — API response shape (credentials redacted)
// ---------------------------------------------------------------------------

/**
 * A named, reusable connection record as returned by the API.
 * Credentials are never included — `hasCredentials` signals whether a key is stored.
 */
export interface Connection {
  /** UUID assigned by the server on creation. */
  id: string;
  /** User-defined label, e.g. "Local Llama3" or "Work Copilot". */
  name: string;
  /** Which LLM backend this connection targets. */
  providerType: ProviderType;
  /** Required for Ollama and LM Studio; optional base URL override for cloud providers. */
  baseUrl?: string;
  /**
   * True when an API key is stored for this connection on disk.
   * The actual key is never returned by the API.
   */
  hasCredentials: boolean;
  /** OpenAI-specific optional organization identifier. */
  orgId?: string;
  /** OpenAI-specific optional project identifier. */
  projectId?: string;
  /** Default model used when no per-stage model override is set. */
  defaultModel: string;
  createdAt: string;
  updatedAt: string;
}

// ---------------------------------------------------------------------------
// ConnectionInput — request body for create / update
// ---------------------------------------------------------------------------

/**
 * Payload sent to create or update a connection.
 * Includes `apiKey` so the frontend can write credentials; the server stores
 * the key and never returns it again (only `hasCredentials` is exposed).
 */
export interface ConnectionInput {
  /** User-defined label. */
  name: string;
  providerType: ProviderType;
  /** Required for Ollama / LM Studio; optional override for cloud providers. */
  baseUrl?: string;
  /** API key for cloud providers (Anthropic, OpenAI, Gemini). Omit for Ollama, LM Studio, Claude CLI. */
  apiKey?: string;
  /** OpenAI-specific optional organization identifier. */
  orgId?: string;
  /** OpenAI-specific optional project identifier. */
  projectId?: string;
  /** Default model for this connection. */
  defaultModel: string;
}

// ---------------------------------------------------------------------------
// ModelInfo — discovered model from provider
// ---------------------------------------------------------------------------

/**
 * A model returned by the provider's model-list endpoint (e.g. GET /api/tags for Ollama,
 * GET /v1/models for OpenAI-compatible APIs).
 */
export interface ModelInfo {
  /** Model identifier as understood by the provider (e.g. "llama3:8b", "gpt-4o"). */
  id: string;
  /** Human-readable display name, if provided by the API. Falls back to `id` if absent. */
  name: string;
}

// ---------------------------------------------------------------------------
// TestResult — result of a connection probe
// ---------------------------------------------------------------------------

/**
 * Returned by POST /api/connections/:id/test and POST /api/connections/test.
 *
 * On success, `models` is populated if the provider exposes a model-list endpoint.
 * On failure, `error` contains a short human-readable reason (e.g. "Connection refused").
 */
export interface TestResult {
  /** Whether the connectivity and auth probe succeeded. */
  success: boolean;
  /** Short error description when `success` is false. */
  error?: string;
  /**
   * Discovered models, populated when `success` is true and the provider exposes a
   * model-list endpoint. Empty array (or absent) when model discovery is not supported.
   */
  models?: ModelInfo[];
}

// ---------------------------------------------------------------------------
// DeleteResult — result of deleting a connection
// ---------------------------------------------------------------------------

/**
 * Returned by DELETE /api/connections/:id.
 *
 * When the deleted connection was referenced by stage assignments (global defaults or
 * project overrides), those assignments are cleared and the affected stage names are
 * listed here so the frontend can warn the user.
 */
export interface DeleteResult {
  /**
   * Stage names whose assignments were cleared because they referenced the deleted
   * connection. Empty array when no assignments were affected.
   *
   * Typed as `string[]` (not `StageName[]`) to faithfully mirror the backend API
   * contract — the server returns raw strings, not a validated union.
   */
  affectedStages: string[];
}

// ---------------------------------------------------------------------------
// ConnectionError — structured payload in StreamEvent{type: 'error'} content
// ---------------------------------------------------------------------------

/**
 * Structured data embedded in a StreamEvent{type: 'error'} `content` field
 * when the error originates from a failing provider connection.
 *
 * The backend JSON-encodes this object into the SSE content string so that
 * the frontend can distinguish connection errors from generic errors and
 * render the actionable ConnectionErrorBanner (SCR-012 / IACT-011).
 */
export interface ConnectionError {
  /** Discriminant — always `true` for connection errors. */
  isConnectionError: true;
  /** UUID of the failing connection. */
  connectionId: string;
  /** Human-readable name of the failing connection, e.g. "Local Llama3". */
  connectionName: string;
  /** Short error reason, e.g. "Connection refused at http://localhost:11434". */
  reason: string;
}

// ---------------------------------------------------------------------------
// Stage assignment & config
// ---------------------------------------------------------------------------

/**
 * Assigns a specific connection and model to a single pipeline stage.
 * Used in both global defaults and per-project overrides.
 */
export interface StageAssignment {
  /** ID of the `Connection` to use for this stage. */
  connectionId: string;
  /**
   * Model to use for this stage. May differ from the connection's `defaultModel`
   * when the user selects a different model at the stage level.
   */
  model: string;
}

/**
 * Global stage defaults stored in ~/.paulette/config.json.
 *
 * The backend wraps the per-stage map in a `stageDefaults` key (e.g.
 * `GET /api/config/stages/:stage` returns the full object for the stage).
 * Absent stage entries fall back to the built-in Claude CLI default.
 *
 * Shape matches the backend `GlobalConfig` struct:
 *   { "stageDefaults": { "vision": { "connectionId": "…", "model": "…" }, … } }
 */
export interface GlobalStageConfig {
  stageDefaults: Partial<Record<StageName, StageAssignment>>;
}

/**
 * Per-project stage configuration stored in <hostDir>/.paulette/stage_config.json.
 *
 * `overrides` maps each stage to its project-specific assignment, or `null` to
 * signal "inherit from global defaults". Absent keys are treated as `null` (inherit).
 */
export interface ProjectStageConfig {
  /** Per-stage overrides. A `null` value means "use the global default for this stage". */
  overrides: Partial<Record<StageName, StageAssignment | null>>;
}
