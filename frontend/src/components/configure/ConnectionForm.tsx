/**
 * ConnectionForm — slide-over panel for adding or editing an LLM provider connection.
 *
 * Fields vary by provider type (IACT-007):
 *   - Ollama / LM Studio  → Base URL only
 *   - Anthropic           → API Key + optional Base URL
 *   - OpenAI              → API Key + optional Org ID + Project ID + Base URL
 *   - Gemini              → API Key + optional Base URL
 *   - Claude CLI          → no fields (uses local binary)
 *   - GitHub Copilot      → disabled / coming soon
 *
 * Test Connection simultaneously probes connectivity and attempts model discovery
 * (IACT-008). If the model list is returned, the model field becomes a <select>;
 * otherwise it stays as a free-text <input> with no error shown.
 *
 * Credential warning banner shown whenever the API key field is non-empty (IACT-009).
 *
 * Journey: JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-005
 * Architecture: ARCH-v0.2.0-024, SCR-009
 */

import { useState, useCallback, useEffect } from 'react';
import type { CSSProperties } from 'react';
import type { Connection, ConnectionInput, ModelInfo, ProviderType } from '../../types/provider';
import {
  createConnection,
  updateConnection,
  testConnection,
  testNewConnection,
} from '../../api/connections';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ConnectionFormProps {
  /** If provided, opens in edit mode pre-filled with this connection. */
  connection?: Connection;
  /** Called with the created/updated connection after a successful save. */
  onSaved: (connection: Connection) => void;
  /** Called when the user cancels or closes the panel. */
  onClose: () => void;
}

interface FormData {
  name: string;
  providerType: ProviderType;
  baseUrl: string;
  apiKey: string;
  orgId: string;
  projectId: string;
  model: string;
}

type TestStatus = 'idle' | 'testing' | 'success' | 'error';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const PROVIDER_LABELS: Record<ProviderType, string> = {
  ollama: 'Ollama',
  lmstudio: 'LM Studio',
  anthropic: 'Anthropic API',
  openai: 'OpenAI',
  gemini: 'Gemini',
  claude_cli: 'Claude CLI',
  github_copilot: 'GitHub Copilot (Coming Soon)',
};

const DEFAULT_BASE_URLS: Partial<Record<ProviderType, string>> = {
  ollama: 'http://localhost:11434',
  lmstudio: 'http://localhost:1234',
  openai: 'https://api.openai.com',
  anthropic: 'https://api.anthropic.com',
  gemini: 'https://generativelanguage.googleapis.com',
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function buildInput(form: FormData): ConnectionInput {
  const input: ConnectionInput = {
    name: form.name.trim(),
    providerType: form.providerType,
    defaultModel: form.model.trim(),
  };
  if (form.baseUrl.trim()) input.baseUrl = form.baseUrl.trim();
  if (form.apiKey.trim()) input.apiKey = form.apiKey.trim();
  if (form.orgId.trim()) input.orgId = form.orgId.trim();
  if (form.projectId.trim()) input.projectId = form.projectId.trim();
  return input;
}

function providerNeedsUrl(type: ProviderType): boolean {
  return type === 'ollama' || type === 'lmstudio';
}

function providerNeedsApiKey(type: ProviderType): boolean {
  return type === 'anthropic' || type === 'openai' || type === 'gemini';
}

function providerHasOptionalUrl(type: ProviderType): boolean {
  return type === 'anthropic' || type === 'openai' || type === 'gemini';
}

function providerHasOrgFields(type: ProviderType): boolean {
  return type === 'openai';
}

// ---------------------------------------------------------------------------
// ConnectionForm
// ---------------------------------------------------------------------------

export function ConnectionForm({ connection, onSaved, onClose }: ConnectionFormProps) {
  const isEdit = Boolean(connection);

  const [form, setForm] = useState<FormData>(() => ({
    name: connection?.name ?? '',
    providerType: connection?.providerType ?? 'ollama',
    baseUrl: connection?.baseUrl ?? '',
    apiKey: '', // never pre-filled — server never returns raw keys
    orgId: connection?.orgId ?? '',
    projectId: connection?.projectId ?? '',
    model: connection?.defaultModel ?? '',
  }));

  const [testStatus, setTestStatus] = useState<TestStatus>('idle');
  const [testError, setTestError] = useState('');
  const [discoveredModels, setDiscoveredModels] = useState<ModelInfo[]>(
    connection?.discoveredModels ?? [],
  );

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // When provider type changes, clear fields that don't apply to the new type
  const handleProviderChange = useCallback((newType: ProviderType) => {
    setForm(prev => ({
      ...prev,
      providerType: newType,
      // Clear URL when switching to a type that doesn't need it (and isn't optional)
      baseUrl: providerNeedsUrl(newType) || providerHasOptionalUrl(newType)
        ? (newType === prev.providerType ? prev.baseUrl : '')
        : '',
      apiKey: '',
      orgId: '',
      projectId: '',
    }));
    setTestStatus('idle');
    setTestError('');
    setDiscoveredModels([]);
  }, []);

  // Sync discoveredModels from connection prop on first render (edit mode)
  useEffect(() => {
    if (connection?.discoveredModels?.length) {
      setDiscoveredModels(connection.discoveredModels);
    }
  }, [connection]);

  // ── Test Connection ─────────────────────────────────────────────────────────

  const handleTest = useCallback(async () => {
    setTestStatus('testing');
    setTestError('');
    try {
      let result;
      // If editing an existing connection and the user hasn't entered a new API key,
      // test the already-saved credentials via the connection ID endpoint.
      if (isEdit && connection && !form.apiKey.trim()) {
        result = await testConnection(connection.id);
      } else {
        result = await testNewConnection(buildInput(form));
      }

      if (result.success) {
        setTestStatus('success');
        if (result.models && result.models.length > 0) {
          setDiscoveredModels(result.models);
          // Auto-select first model if none is set
          setForm(prev =>
            prev.model ? prev : { ...prev, model: result.models![0].id },
          );
        }
      } else {
        setTestStatus('error');
        setTestError(result.error ?? 'Connection failed');
      }
    } catch (err) {
      setTestStatus('error');
      setTestError(err instanceof Error ? err.message : 'Connection failed');
    }
  }, [form, isEdit, connection]);

  // ── Save ────────────────────────────────────────────────────────────────────

  const handleSave = useCallback(async () => {
    if (!form.name.trim()) {
      setSaveError('Connection name is required.');
      return;
    }
    if (providerNeedsUrl(form.providerType) && !form.baseUrl.trim()) {
      setSaveError('Base URL is required for this provider.');
      return;
    }
    if (providerNeedsApiKey(form.providerType) && !isEdit && !form.apiKey.trim()) {
      setSaveError('API key is required for this provider.');
      return;
    }

    setSaving(true);
    setSaveError(null);
    try {
      const input = buildInput(form);
      const saved = isEdit && connection
        ? await updateConnection(connection.id, input)
        : await createConnection(input);
      onSaved(saved);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save connection.');
    } finally {
      setSaving(false);
    }
  }, [form, isEdit, connection, onSaved]);

  // ── Field helpers ───────────────────────────────────────────────────────────

  const setField = useCallback(<K extends keyof FormData>(key: K, value: FormData[K]) => {
    setForm(prev => ({ ...prev, [key]: value }));
  }, []);

  const showCredentialWarning = providerNeedsApiKey(form.providerType) && form.apiKey.trim().length > 0;
  const hasExistingCredentials = isEdit && connection?.hasCredentials && !form.apiKey.trim();

  // ── Render ──────────────────────────────────────────────────────────────────

  const type = form.providerType;
  const isComingSoon = type === 'github_copilot';

  return (
    <>
      {/* Inject keyframes and animation class */}
      <style>{SLIDE_IN_STYLE}</style>

      {/* Backdrop */}
      <div
        style={s.backdrop}
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Panel */}
      <div
        style={s.panel}
        role="dialog"
        aria-modal="true"
        aria-label={isEdit ? 'Edit Connection' : 'Add Connection'}
      >
        {/* Header */}
        <div style={s.header}>
          <h2 style={s.headerTitle}>{isEdit ? 'Edit Connection' : 'Add Connection'}</h2>
          <button
            style={s.closeBtn}
            onClick={onClose}
            type="button"
            aria-label="Close"
          >
            ✕
          </button>
        </div>

        {/* Scrollable body */}
        <div style={s.body}>
          {/* Name */}
          <Field label="Connection Name">
            <input
              type="text"
              value={form.name}
              onChange={e => setField('name', e.target.value)}
              placeholder="e.g. Local Llama3"
              style={s.input}
              autoFocus
            />
          </Field>

          {/* Provider Type */}
          <Field label="Provider Type">
            <select
              value={form.providerType}
              onChange={e => handleProviderChange(e.target.value as ProviderType)}
              style={s.select}
            >
              {(Object.keys(PROVIDER_LABELS) as ProviderType[]).map(pt => (
                <option
                  key={pt}
                  value={pt}
                  disabled={pt === 'github_copilot'}
                >
                  {PROVIDER_LABELS[pt]}
                </option>
              ))}
            </select>
          </Field>

          {/* Coming soon note */}
          {isComingSoon && (
            <div style={s.comingSoonNote}>
              GitHub Copilot OAuth support is planned for v0.3.0.
            </div>
          )}

          {/* Claude CLI info note */}
          {type === 'claude_cli' && (
            <div style={s.infoNote}>
              Uses the local <code style={s.code}>claude</code> binary and its
              existing authentication. No additional configuration needed.
            </div>
          )}

          {/* Base URL — required for Ollama/LM Studio */}
          {(providerNeedsUrl(type) || providerHasOptionalUrl(type)) && !isComingSoon && (
            <Field
              label="Base URL"
              hint={providerNeedsUrl(type) ? 'Required' : 'Optional override'}
            >
              <input
                type="url"
                value={form.baseUrl}
                onChange={e => setField('baseUrl', e.target.value)}
                placeholder={DEFAULT_BASE_URLS[type] ?? 'https://…'}
                style={s.input}
              />
            </Field>
          )}

          {/* API Key */}
          {providerNeedsApiKey(type) && (
            <Field
              label="API Key"
              hint={hasExistingCredentials ? 'Leave blank to keep existing key' : undefined}
            >
              <input
                type="password"
                value={form.apiKey}
                onChange={e => setField('apiKey', e.target.value)}
                placeholder={hasExistingCredentials ? '••••••••••••' : 'sk-…'}
                style={s.input}
                autoComplete="off"
              />
            </Field>
          )}

          {/* OpenAI-specific Org ID / Project ID */}
          {providerHasOrgFields(type) && (
            <>
              <Field label="Organization ID" hint="Optional">
                <input
                  type="text"
                  value={form.orgId}
                  onChange={e => setField('orgId', e.target.value)}
                  placeholder="org-…"
                  style={s.input}
                />
              </Field>
              <Field label="Project ID" hint="Optional">
                <input
                  type="text"
                  value={form.projectId}
                  onChange={e => setField('projectId', e.target.value)}
                  placeholder="proj_…"
                  style={s.input}
                />
              </Field>
            </>
          )}

          {/* Credential warning */}
          {showCredentialWarning && (
            <div style={s.credWarning}>
              ⚠ Credentials are saved in plain text to{' '}
              <code style={s.code}>~/.paulette/connections.json</code> (user-readable only).
              No keychain integration in v0.2.0.
            </div>
          )}

          {/* Test Connection */}
          {type !== 'github_copilot' && (
            <div style={{ marginTop: '0.25rem' }}>
              <button
                type="button"
                onClick={handleTest}
                disabled={testStatus === 'testing'}
                style={{
                  ...s.testBtn,
                  opacity: testStatus === 'testing' ? 0.7 : 1,
                  cursor: testStatus === 'testing' ? 'not-allowed' : 'pointer',
                }}
              >
                {testStatus === 'testing' ? (
                  <span style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', justifyContent: 'center' }}>
                    <span style={s.spinner} />
                    Testing…
                  </span>
                ) : (
                  'Test Connection'
                )}
              </button>

              {testStatus === 'success' && (
                <p style={s.testSuccess}>✓ Connected</p>
              )}
              {testStatus === 'success' && discoveredModels.length > 0 && (
                <p style={s.testDiscovery}>Models fetched ✓</p>
              )}
              {testStatus === 'error' && (
                <p style={s.testFailure}>✗ {testError || 'Connection failed'}</p>
              )}
            </div>
          )}

          {/* Default Model */}
          {type !== 'github_copilot' && type !== 'claude_cli' && (
            <Field label="Default Model">
              {discoveredModels.length > 0 ? (
                <>
                  <select
                    value={form.model}
                    onChange={e => setField('model', e.target.value)}
                    style={s.select}
                  >
                    {!discoveredModels.some(m => m.id === form.model) && form.model && (
                      <option value={form.model}>{form.model}</option>
                    )}
                    <option value="">— Select a model —</option>
                    {discoveredModels.map(m => (
                      <option key={m.id} value={m.id}>
                        {m.name || m.id}
                      </option>
                    ))}
                  </select>
                  <p style={s.discoveryHint}>Models fetched from provider</p>
                </>
              ) : (
                <>
                  <input
                    type="text"
                    value={form.model}
                    onChange={e => setField('model', e.target.value)}
                    placeholder="e.g. llama3:8b"
                    style={s.input}
                  />
                  <p style={s.manualModelHint}>Enter model name manually</p>
                </>
              )}
            </Field>
          )}

          {/* Save error */}
          {saveError && (
            <div style={s.saveError}>
              ⚠ {saveError}
            </div>
          )}
        </div>

        {/* Footer */}
        <div style={s.footer}>
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || isComingSoon}
            style={{
              ...s.saveBtn,
              opacity: saving || isComingSoon ? 0.6 : 1,
              cursor: saving || isComingSoon ? 'not-allowed' : 'pointer',
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button
            type="button"
            onClick={onClose}
            style={s.cancelBtn}
          >
            Cancel
          </button>
        </div>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Field wrapper
// ---------------------------------------------------------------------------

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '0.375rem' }}>
      <label style={s.fieldLabel}>
        {label}
        {hint && <span style={s.fieldHint}> — {hint}</span>}
      </label>
      {children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Slide-in keyframes (injected once)
// ---------------------------------------------------------------------------

const SLIDE_IN_STYLE = `
@keyframes cf-slide-in {
  from { transform: translateX(100%); }
  to   { transform: translateX(0); }
}
`;

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

const s: Record<string, CSSProperties> = {
  backdrop: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(0,0,0,0.45)',
    zIndex: 40,
  },

  panel: {
    position: 'fixed',
    top: 0,
    right: 0,
    bottom: 0,
    width: '24rem',       // w-96
    background: '#1e293b',
    boxShadow: '-4px 0 24px rgba(0,0,0,0.4)',
    zIndex: 50,
    display: 'flex',
    flexDirection: 'column',
    animation: 'cf-slide-in 0.2s ease-out',
  },

  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '1rem 1.25rem',
    borderBottom: '1px solid #334155',
    flexShrink: 0,
  },

  headerTitle: {
    fontSize: '1rem',
    fontWeight: 600,
    color: '#f8fafc',
    margin: 0,
  },

  closeBtn: {
    background: 'none',
    border: 'none',
    color: '#94a3b8',
    cursor: 'pointer',
    fontSize: '1rem',
    padding: '0.25rem',
    lineHeight: 1,
    borderRadius: '4px',
    fontFamily: 'inherit',
    transition: 'color 0.15s',
  },

  body: {
    flex: 1,
    overflowY: 'auto',
    padding: '1.25rem',
    display: 'flex',
    flexDirection: 'column',
    gap: '1rem',
  },

  footer: {
    display: 'flex',
    gap: '0.75rem',
    padding: '1rem 1.25rem',
    borderTop: '1px solid #334155',
    flexShrink: 0,
  },

  fieldLabel: {
    fontSize: '0.8125rem',
    fontWeight: 500,
    color: '#cbd5e1',
  },

  fieldHint: {
    fontWeight: 400,
    color: '#64748b',
    fontSize: '0.75rem',
  },

  input: {
    width: '100%',
    background: '#0f172a',
    color: '#e2e8f0',
    border: '1px solid #334155',
    borderRadius: '6px',
    padding: '0.4375rem 0.625rem',
    fontSize: '0.875rem',
    outline: 'none',
    fontFamily: 'inherit',
    boxSizing: 'border-box',
  },

  select: {
    width: '100%',
    background: '#0f172a',
    color: '#e2e8f0',
    border: '1px solid #334155',
    borderRadius: '6px',
    padding: '0.4375rem 0.625rem',
    fontSize: '0.875rem',
    outline: 'none',
    fontFamily: 'inherit',
    cursor: 'pointer',
    appearance: 'auto',
    boxSizing: 'border-box',
  },

  testBtn: {
    width: '100%',
    background: 'transparent',
    border: '1px solid #3b82f6',
    color: '#60a5fa',
    borderRadius: '8px',
    padding: '0.5rem',
    fontSize: '0.875rem',
    fontWeight: 500,
    fontFamily: 'inherit',
    transition: 'background 0.15s',
  },

  testSuccess: {
    margin: '0.375rem 0 0',
    fontSize: '0.8125rem',
    fontWeight: 500,
    color: '#4ade80',
  },

  testDiscovery: {
    margin: '0.125rem 0 0',
    fontSize: '0.75rem',
    color: '#4ade80',
  },

  testFailure: {
    margin: '0.375rem 0 0',
    fontSize: '0.8125rem',
    fontWeight: 500,
    color: '#f87171',
  },

  discoveryHint: {
    margin: '0.25rem 0 0',
    fontSize: '0.6875rem',
    color: '#4ade80',
  },

  manualModelHint: {
    margin: '0.25rem 0 0',
    fontSize: '0.6875rem',
    color: '#475569',
  },

  infoNote: {
    background: 'rgba(59, 130, 246, 0.08)',
    border: '1px solid rgba(59, 130, 246, 0.2)',
    borderRadius: '6px',
    padding: '0.625rem 0.75rem',
    fontSize: '0.8125rem',
    color: '#93c5fd',
    lineHeight: 1.5,
  },

  comingSoonNote: {
    background: 'rgba(100, 116, 139, 0.1)',
    border: '1px solid rgba(100, 116, 139, 0.2)',
    borderRadius: '6px',
    padding: '0.625rem 0.75rem',
    fontSize: '0.8125rem',
    color: '#64748b',
    lineHeight: 1.5,
  },

  credWarning: {
    background: 'rgba(251, 191, 36, 0.08)',
    border: '1px solid rgba(251, 191, 36, 0.25)',
    borderRadius: '6px',
    padding: '0.625rem 0.75rem',
    fontSize: '0.75rem',
    color: '#fbbf24',
    lineHeight: 1.5,
  },

  code: {
    background: 'rgba(0,0,0,0.3)',
    borderRadius: '3px',
    padding: '0 3px',
    fontFamily: 'monospace',
    fontSize: '0.8em',
  },

  saveError: {
    background: 'rgba(239, 68, 68, 0.1)',
    border: '1px solid rgba(239, 68, 68, 0.3)',
    borderRadius: '6px',
    padding: '0.625rem 0.75rem',
    fontSize: '0.8125rem',
    color: '#fca5a5',
  },

  saveBtn: {
    flex: 1,
    background: '#3b82f6',
    color: '#fff',
    border: 'none',
    borderRadius: '8px',
    padding: '0.5625rem 1rem',
    fontSize: '0.875rem',
    fontWeight: 500,
    fontFamily: 'inherit',
    transition: 'background 0.15s',
  },

  cancelBtn: {
    flex: 1,
    background: 'transparent',
    color: '#94a3b8',
    border: '1px solid #334155',
    borderRadius: '8px',
    padding: '0.5625rem 1rem',
    fontSize: '0.875rem',
    fontWeight: 400,
    fontFamily: 'inherit',
    cursor: 'pointer',
    transition: 'background 0.15s',
  },

  spinner: {
    display: 'inline-block',
    width: '0.875rem',
    height: '0.875rem',
    border: '2px solid rgba(96, 165, 250, 0.3)',
    borderTopColor: '#60a5fa',
    borderRadius: '50%',
    animation: 'spin 0.7s linear infinite',
  },
};
