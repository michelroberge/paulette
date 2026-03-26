/**
 * API client for connection management — CRUD, testing, and model discovery.
 *
 * Connections are named, reusable records that describe how to reach an LLM backend
 * (Ollama, LM Studio, OpenAI, Anthropic, Gemini, or the local Claude CLI). They are
 * persisted to ~/.paulette/connections.json on the server (chmod 600).
 *
 * Key design rules:
 *   - `Connection` responses never include the raw API key — only `hasCredentials: boolean`.
 *   - `ConnectionInput` (create/update) includes `apiKey` so the frontend can write credentials.
 *   - Test endpoints simultaneously probe connectivity AND attempt model discovery (IACT-008).
 *
 * Journeys: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004, JRN-v0.2.0-005
 * Architecture: ARCH-v0.2.0-029
 */

import type {
  Connection,
  ConnectionInput,
  ModelInfo,
  TestResult,
  DeleteResult,
} from '../types/provider';
import { apiFetch } from './client';

/**
 * Retrieve all saved connections.
 * Credentials are redacted — each record exposes `hasCredentials` instead of the raw key.
 */
export function listConnections(): Promise<Connection[]> {
  return apiFetch<Connection[]>('/connections');
}

/**
 * Create a new named connection.
 * The server assigns a UUID, applies timestamps, and writes the record to
 * ~/.paulette/connections.json with chmod 600.
 *
 * @param input  Full connection configuration including any API key.
 * @returns The newly created connection (credentials redacted).
 */
export function createConnection(input: ConnectionInput): Promise<Connection> {
  return apiFetch<Connection>('/connections', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

/**
 * Retrieve a single connection by its UUID.
 * Credentials are redacted — `hasCredentials` indicates whether a key is stored.
 *
 * @param id  The connection UUID.
 */
export function getConnection(id: string): Promise<Connection> {
  return apiFetch<Connection>(`/connections/${id}`);
}

/**
 * Update an existing connection.
 * Only fields present in `input` are changed; omitting `apiKey` leaves the stored key untouched.
 *
 * @param id     The connection UUID.
 * @param input  Updated connection fields.
 * @returns The updated connection (credentials redacted).
 */
export function updateConnection(id: string, input: ConnectionInput): Promise<Connection> {
  return apiFetch<Connection>(`/connections/${id}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
}

/**
 * Delete a connection by its UUID.
 *
 * Any global or per-project stage assignments that referenced the deleted connection are
 * automatically cleared. The response lists the affected stage names so the UI can warn
 * the user (e.g. "Stage assignments for Vision, UX were reset to default.").
 *
 * @param id  The connection UUID.
 * @returns Object containing the list of stage names whose assignments were cleared.
 */
export function deleteConnection(id: string): Promise<DeleteResult> {
  return apiFetch<DeleteResult>(`/connections/${id}`, {
    method: 'DELETE',
  });
}

/**
 * Test an existing saved connection.
 *
 * Sends a minimal probe request to verify connectivity and authentication. Simultaneously
 * attempts to fetch the provider's model list (opportunistic — IACT-008). On success,
 * `models` is populated when the provider exposes a model-list endpoint; it is absent or
 * empty when model discovery is unsupported.
 *
 * @param id  The connection UUID.
 * @returns Test outcome with optional discovered models.
 */
export function testConnection(id: string): Promise<TestResult> {
  return apiFetch<TestResult>(`/connections/${id}/test`, {
    method: 'POST',
  });
}

/**
 * Test an unsaved connection configuration without persisting it first.
 *
 * Used in the Connection Form (SCR-009) so the user can validate settings before saving.
 * Behaves identically to `testConnection` but accepts a full `ConnectionInput` body
 * instead of a connection ID.
 *
 * @param input  The connection configuration to probe (not yet saved to disk).
 * @returns Test outcome with optional discovered models.
 */
export function testNewConnection(input: ConnectionInput): Promise<TestResult> {
  return apiFetch<TestResult>('/connections/test', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

/**
 * Fetch the list of models available from a saved connection's provider.
 *
 * Returns the same model list that would be populated during a successful Test Connection
 * call. Useful for re-fetching models after the connection's base URL or credentials change.
 *
 * @param id  The connection UUID.
 * @returns Array of discovered models. Throws if the provider does not support model listing.
 */
export function listModels(id: string): Promise<ModelInfo[]> {
  return apiFetch<ModelInfo[]>(`/connections/${id}/models`);
}

/**
 * Fetch the list of models from an unsaved connection configuration.
 *
 * Allows the Connection Form (SCR-009) to populate the model dropdown immediately after
 * a successful test without requiring the user to save first.
 *
 * @param input  The connection configuration to query (not yet saved to disk).
 * @returns Array of discovered models. Throws if the provider does not support model listing.
 */
export function listModelsForNew(input: ConnectionInput): Promise<ModelInfo[]> {
  return apiFetch<ModelInfo[]>('/connections/models', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}
