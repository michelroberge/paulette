/**
 * ConnectionsTab — list of all defined LLM provider connections with CRUD actions.
 *
 * Features:
 *   - Connections fetched on mount; rendered as cards with provider-type badge,
 *     name, default model, and inline Test / Edit / Delete actions.
 *   - "Add Connection" button opens the ConnectionForm slide-over.
 *   - Edit opens the slide-over pre-filled with the connection's data.
 *   - Delete shows a confirmation dialog; if stage assignments are affected,
 *     a warning lists the impacted stages.
 *   - ?highlight= deep-link: the matching card pulses with a red ring for 3 seconds
 *     then clearHighlight() is called (IACT-011).
 *   - Empty state with a plug icon and friendly message.
 *
 * Journey: JRN-v0.2.0-001, JRN-v0.2.0-002, JRN-v0.2.0-003, JRN-v0.2.0-004
 * Architecture: ARCH-v0.2.0-023, SCR-008
 */

import { useState, useEffect, useCallback, useRef } from 'react';
import type { CSSProperties } from 'react';
import type { Connection, ProviderType, TestResult } from '../../types/provider';
import { listConnections, deleteConnection, testConnection } from '../../api/connections';
import { ConnectionForm } from './ConnectionForm';
import { CredentialWarningToast } from './CredentialWarningToast';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ConnectionsTabProps {
  /**
   * Optional connection ID to highlight on mount (from the ?highlight= query param).
   * Used for deep-linking from inline connection error banners (IACT-011).
   */
  highlight?: string;
  /**
   * Called by ConnectionsTab once the highlight pulse animation has fired so that
   * ConfigurePage can remove the ?highlight= query param from the URL.
   * This prevents the card from re-pulsing every time the user returns to this tab.
   */
  clearHighlight?: () => void;
}

type FormMode =
  | { open: false }
  | { open: true; mode: 'add' }
  | { open: true; mode: 'edit'; connection: Connection };

interface DeleteState {
  connection: Connection;
  /** null = not yet attempted; string[] = cascade result from the API response */
  affectedStages: string[] | null;
  confirming: boolean;
}

interface RowTestState {
  loading: boolean;
  result?: TestResult;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const PROVIDER_LABELS: Record<ProviderType, string> = {
  ollama: 'Ollama',
  lmstudio: 'LM Studio',
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  gemini: 'Gemini',
  claude_cli: 'Claude CLI',
  github_copilot: 'Copilot',
};

/** Badge colours per provider — adapted for dark background. */
const BADGE_STYLES: Record<ProviderType, CSSProperties> = {
  ollama:        { background: 'rgba(139,92,246,0.18)', color: '#c4b5fd' },
  lmstudio:      { background: 'rgba(99,102,241,0.18)', color: '#a5b4fc' },
  anthropic:     { background: 'rgba(251,146,60,0.18)',  color: '#fdba74' },
  openai:        { background: 'rgba(34,197,94,0.18)',   color: '#86efac' },
  gemini:        { background: 'rgba(59,130,246,0.18)',  color: '#93c5fd' },
  claude_cli:    { background: 'rgba(100,116,139,0.18)', color: '#94a3b8' },
  github_copilot:{ background: 'rgba(100,116,139,0.18)', color: '#94a3b8' },
};

// ---------------------------------------------------------------------------
// Keyframe injection
// ---------------------------------------------------------------------------

const KEYFRAMES = `
@keyframes ct-highlight-pulse {
  0%   { box-shadow: 0 0 0 2px #f87171, 0 0 0 4px rgba(248,113,113,0.2); }
  50%  { box-shadow: 0 0 0 2px rgba(248,113,113,0.4), 0 0 0 6px rgba(248,113,113,0.08); }
  100% { box-shadow: 0 0 0 2px #f87171, 0 0 0 4px rgba(248,113,113,0.2); }
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
`;

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Coloured pill badge for the provider type. */
function ProviderBadge({ providerType }: { providerType: ProviderType }) {
  return (
    <span
      style={{
        ...BADGE_STYLES[providerType],
        fontSize: '0.6875rem',
        fontWeight: 600,
        borderRadius: '9999px',
        padding: '0.15rem 0.55rem',
        whiteSpace: 'nowrap',
        flexShrink: 0,
      }}
    >
      {PROVIDER_LABELS[providerType]}
    </span>
  );
}

/** Inline test-result label rendered below a card's action row. */
function TestResultLabel({ state }: { state: RowTestState }) {
  if (state.loading) {
    return (
      <span style={s.testLoading}>
        <span style={s.spinner} /> Testing…
      </span>
    );
  }
  if (!state.result) return null;
  if (state.result.success) {
    return (
      <span style={s.testSuccess}>
        ✓ Connected
        {state.result.models && state.result.models.length > 0
          ? ` · ${state.result.models.length} model${state.result.models.length > 1 ? 's' : ''} found`
          : ''}
      </span>
    );
  }
  return (
    <span style={s.testFailure}>
      ✗ {state.result.error ?? 'Connection failed'}
    </span>
  );
}

/** Plug icon for the empty state. */
function PlugIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      width="40"
      height="40"
      fill="none"
      stroke="#475569"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M18 8h1a4 4 0 0 1 0 8h-1" />
      <path d="M2 8h16v9a4 4 0 0 1-4 4H6a4 4 0 0 1-4-4V8z" />
      <line x1="6" y1="1" x2="6" y2="4" />
      <line x1="10" y1="1" x2="10" y2="4" />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// ConnectionsTab
// ---------------------------------------------------------------------------

export function ConnectionsTab({ highlight, clearHighlight }: ConnectionsTabProps) {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // Form slide-over state
  const [formMode, setFormMode] = useState<FormMode>({ open: false });

  // Delete confirmation state
  const [deleteState, setDeleteState] = useState<DeleteState | null>(null);

  // Per-row test states
  const [testStates, setTestStates] = useState<Record<string, RowTestState>>({});

  // Credential warning toast (IACT-009) — shown after a save that wrote credentials
  const [showCredToast, setShowCredToast] = useState(false);

  // Highlight state — true while the pulse animation is active
  const [highlightActive, setHighlightActive] = useState(!!highlight);
  const highlightRef = useRef<HTMLDivElement | null>(null);
  const clearHighlightCalledRef = useRef(false);

  // ── Load connections ────────────────────────────────────────────────────────

  const loadConnections = useCallback(async () => {
    try {
      const data = await listConnections();
      setConnections(data);
    } catch (err) {
      console.error('[ConnectionsTab] Failed to load connections:', err);
      setLoadError('Failed to load connections. Please refresh and try again.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadConnections();
  }, [loadConnections]);

  // ── Highlight pulse ─────────────────────────────────────────────────────────

  // Once highlight is set, activate the pulse and clear it after 3 seconds.
  useEffect(() => {
    if (!highlight) return;

    setHighlightActive(true);
    clearHighlightCalledRef.current = false;

    const timer = setTimeout(() => {
      setHighlightActive(false);
      if (!clearHighlightCalledRef.current) {
        clearHighlightCalledRef.current = true;
        clearHighlight?.();
      }
    }, 3000);

    return () => clearTimeout(timer);
    // Intentionally not including clearHighlight in deps to avoid re-triggering
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [highlight]);

  // Scroll highlighted card into view once the list has loaded
  useEffect(() => {
    if (highlight && !loading && highlightRef.current) {
      highlightRef.current.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  }, [highlight, loading]);

  // ── Form handlers ───────────────────────────────────────────────────────────

  const openAdd = useCallback(() => {
    setFormMode({ open: true, mode: 'add' });
  }, []);

  const openEdit = useCallback((conn: Connection) => {
    setFormMode({ open: true, mode: 'edit', connection: conn });
  }, []);

  const closeForm = useCallback(() => {
    setFormMode({ open: false });
  }, []);

  const handleSaved = useCallback((saved: Connection) => {
    setConnections(prev => {
      const idx = prev.findIndex(c => c.id === saved.id);
      if (idx >= 0) {
        const next = [...prev];
        next[idx] = saved;
        return next;
      }
      return [...prev, saved];
    });
    closeForm();
    // Direct call-site hook for IACT-009: show the credential warning toast
    // whenever the saved connection carries API credentials.  This fires even
    // if `onCredentialsSaved` in ConnectionForm didn't trigger (e.g. a future
    // refactor), and also covers any save path that ends up here.
    if (saved.hasCredentials) {
      setShowCredToast(true);
    }
  }, [closeForm]);

  // ── Delete handlers ─────────────────────────────────────────────────────────

  const openDelete = useCallback((conn: Connection) => {
    setDeleteState({ connection: conn, affectedStages: null, confirming: false });
  }, []);

  const cancelDelete = useCallback(() => {
    setDeleteState(null);
  }, []);

  const confirmDelete = useCallback(async () => {
    if (!deleteState) return;
    setDeleteState(prev => prev ? { ...prev, confirming: true } : null);
    try {
      const result = await deleteConnection(deleteState.connection.id);
      setConnections(prev => prev.filter(c => c.id !== deleteState.connection.id));
      // Clear test state for the deleted connection
      setTestStates(prev => {
        const next = { ...prev };
        delete next[deleteState.connection.id];
        return next;
      });
      if (result.affectedStages.length > 0) {
        // Show a brief "affected stages" message in the delete state before dismissing
        setDeleteState(prev =>
          prev ? { ...prev, confirming: false, affectedStages: result.affectedStages } : null,
        );
        // Auto-dismiss after 4 seconds
        setTimeout(() => setDeleteState(null), 4000);
      } else {
        setDeleteState(null);
      }
    } catch (err) {
      console.error('[ConnectionsTab] Delete failed:', err);
      setDeleteState(prev =>
        prev ? { ...prev, confirming: false, affectedStages: ['Delete failed — please try again.'] } : null,
      );
    }
  }, [deleteState]);

  // ── Test handlers ───────────────────────────────────────────────────────────

  const handleTest = useCallback(async (conn: Connection) => {
    setTestStates(prev => ({ ...prev, [conn.id]: { loading: true } }));
    try {
      const result = await testConnection(conn.id);
      setTestStates(prev => ({ ...prev, [conn.id]: { loading: false, result } }));
      // Note: we intentionally do NOT merge result.models back onto the connection
      // object.  The backend ConnectionResponse never returns a model list, so
      // `discoveredModels` has no backend backing and adding it as a frontend-only
      // property would be misleading — users must re-run Test Connection inside the
      // Edit form to get the model dropdown populated.
    } catch (err) {
      setTestStates(prev => ({
        ...prev,
        [conn.id]: {
          loading: false,
          result: {
            success: false,
            error: err instanceof Error ? err.message : 'Connection failed',
          },
        },
      }));
    }
  }, []);

  // ── Render ──────────────────────────────────────────────────────────────────

  return (
    <>
      {/* Inject keyframe animations */}
      <style>{KEYFRAMES}</style>

      {/* Top action bar */}
      <div style={s.topBar}>
        <button type="button" onClick={openAdd} style={s.addBtn}>
          + Add Connection
        </button>
      </div>

      {/* Content */}
      {loading ? (
        <p style={s.loadingText}>Loading connections…</p>
      ) : loadError ? (
        <div style={s.errorBox}>
          <span>⚠</span> {loadError}
        </div>
      ) : connections.length === 0 ? (
        <EmptyState onAdd={openAdd} />
      ) : (
        <div style={s.list}>
          {connections.map(conn => {
            const isHighlighted = highlight === conn.id && highlightActive;
            const rowTest = testStates[conn.id];

            return (
              <div
                key={conn.id}
                ref={isHighlighted ? highlightRef : undefined}
                style={{
                  ...s.card,
                  ...(isHighlighted
                    ? {
                        animation: 'ct-highlight-pulse 1s ease-in-out 3',
                        boxShadow: '0 0 0 2px #f87171, 0 0 0 4px rgba(248,113,113,0.15)',
                      }
                    : {}),
                }}
              >
                {/* Card main row */}
                <div style={s.cardMain}>
                  {/* Left: badge + name + model */}
                  <div style={s.cardLeft}>
                    <ProviderBadge providerType={conn.providerType} />
                    <div style={s.cardInfo}>
                      <span style={s.connName}>{conn.name}</span>
                      {conn.defaultModel && (
                        <span style={s.connModel}>{conn.defaultModel}</span>
                      )}
                      {conn.baseUrl && (
                        <span style={s.connUrl}>{conn.baseUrl}</span>
                      )}
                    </div>
                  </div>

                  {/* Right: actions */}
                  <div style={s.cardActions}>
                    <button
                      type="button"
                      onClick={() => handleTest(conn)}
                      disabled={rowTest?.loading}
                      style={{
                        ...s.actionBtn,
                        color: '#60a5fa',
                        opacity: rowTest?.loading ? 0.5 : 1,
                        cursor: rowTest?.loading ? 'not-allowed' : 'pointer',
                      }}
                      aria-label={`Test ${conn.name}`}
                    >
                      Test
                    </button>
                    <span style={s.actionDivider} aria-hidden="true">|</span>
                    <button
                      type="button"
                      onClick={() => openEdit(conn)}
                      style={{ ...s.actionBtn, color: '#94a3b8' }}
                      aria-label={`Edit ${conn.name}`}
                    >
                      Edit
                    </button>
                    <span style={s.actionDivider} aria-hidden="true">|</span>
                    <button
                      type="button"
                      onClick={() => openDelete(conn)}
                      style={{ ...s.actionBtn, color: '#f87171' }}
                      aria-label={`Delete ${conn.name}`}
                    >
                      Delete
                    </button>
                  </div>
                </div>

                {/* Test result row — only shown after a test */}
                {rowTest && (
                  <div style={s.testResultRow}>
                    <TestResultLabel state={rowTest} />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* Connection Form slide-over */}
      {formMode.open && (
        <ConnectionForm
          connection={formMode.mode === 'edit' ? formMode.connection : undefined}
          onSaved={handleSaved}
          onClose={closeForm}
          onCredentialsSaved={() => setShowCredToast(true)}
        />
      )}

      {/* Credential warning toast — shown after a connection save writes API keys to disk */}
      <CredentialWarningToast
        visible={showCredToast}
        onDismiss={() => setShowCredToast(false)}
      />

      {/* Delete confirmation dialog */}
      {deleteState && (
        <DeleteDialog
          state={deleteState}
          onConfirm={confirmDelete}
          onCancel={cancelDelete}
        />
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// EmptyState
// ---------------------------------------------------------------------------

function EmptyState({ onAdd }: { onAdd: () => void }) {
  return (
    <div style={s.emptyState}>
      <PlugIcon />
      <p style={s.emptyTitle}>No connections yet</p>
      <p style={s.emptySubtitle}>
        Add one to start using a custom LLM provider.
      </p>
      <button type="button" onClick={onAdd} style={s.emptyAddBtn}>
        + Add Connection
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// DeleteDialog
// ---------------------------------------------------------------------------

interface DeleteDialogProps {
  state: DeleteState;
  onConfirm: () => void;
  onCancel: () => void;
}

function DeleteDialog({ state, onConfirm, onCancel }: DeleteDialogProps) {
  const { connection, affectedStages, confirming } = state;

  // After delete succeeds, show affected stages warning instead of the prompt
  const showResult = affectedStages !== null && !confirming;

  return (
    <>
      {/* Overlay */}
      <div
        style={s.modalOverlay}
        onClick={onCancel}
        aria-hidden="true"
      />

      {/* Dialog */}
      <div
        style={s.modal}
        role="dialog"
        aria-modal="true"
        aria-label="Delete connection"
      >
        {showResult ? (
          // Post-delete: show cascade result
          <>
            <h3 style={s.modalTitle}>Connection deleted</h3>
            {affectedStages.length > 0 && (
              <div style={s.affectedWarning}>
                ⚠ Stage assignments for{' '}
                <strong>{affectedStages.join(', ')}</strong> were reset to the
                Claude CLI default.
              </div>
            )}
            <div style={s.modalFooter}>
              <button type="button" onClick={onCancel} style={s.modalOkBtn}>
                OK
              </button>
            </div>
          </>
        ) : (
          // Pre-delete: confirmation prompt
          <>
            <h3 style={s.modalTitle}>Delete connection?</h3>
            <p style={s.modalBody}>
              Delete <strong style={{ color: '#f1f5f9' }}>"{connection.name}"</strong>?
              Any stage assignments using this connection will fall back to the
              Claude CLI default.
            </p>
            <div style={s.modalFooter}>
              <button
                type="button"
                onClick={onConfirm}
                disabled={confirming}
                style={{
                  ...s.modalDeleteBtn,
                  opacity: confirming ? 0.6 : 1,
                  cursor: confirming ? 'not-allowed' : 'pointer',
                }}
              >
                {confirming ? 'Deleting…' : 'Delete'}
              </button>
              <button
                type="button"
                onClick={onCancel}
                disabled={confirming}
                style={s.modalCancelBtn}
              >
                Cancel
              </button>
            </div>
          </>
        )}
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

const s: Record<string, CSSProperties> = {
  topBar: {
    display: 'flex',
    justifyContent: 'flex-end',
    marginBottom: '1rem',
  },

  addBtn: {
    background: '#3b82f6',
    color: '#fff',
    border: 'none',
    borderRadius: '8px',
    padding: '0.5rem 1rem',
    fontSize: '0.875rem',
    fontWeight: 500,
    fontFamily: 'inherit',
    cursor: 'pointer',
    transition: 'background 0.15s',
  },

  loadingText: {
    color: '#64748b',
    fontSize: '0.875rem',
    textAlign: 'center',
    paddingTop: '3rem',
  },

  errorBox: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.5rem',
    background: 'rgba(239,68,68,0.1)',
    border: '1px solid rgba(239,68,68,0.3)',
    borderRadius: '8px',
    color: '#fca5a5',
    fontSize: '0.875rem',
    padding: '0.75rem 1rem',
    marginTop: '1rem',
  },

  list: {
    display: 'flex',
    flexDirection: 'column',
    gap: '0.75rem',
  },

  card: {
    background: '#1e293b',
    border: '1px solid #334155',
    borderRadius: '10px',
    padding: '0.875rem 1rem',
    transition: 'box-shadow 0.15s',
  },

  cardMain: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: '1rem',
    flexWrap: 'wrap',
  },

  cardLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.75rem',
    minWidth: 0,
    flex: 1,
  },

  cardInfo: {
    display: 'flex',
    flexDirection: 'column',
    gap: '0.125rem',
    minWidth: 0,
  },

  connName: {
    fontSize: '0.9375rem',
    fontWeight: 600,
    color: '#f1f5f9',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },

  connModel: {
    fontSize: '0.8125rem',
    color: '#64748b',
  },

  connUrl: {
    fontSize: '0.75rem',
    color: '#475569',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },

  cardActions: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.125rem',
    flexShrink: 0,
  },

  actionBtn: {
    background: 'none',
    border: 'none',
    cursor: 'pointer',
    fontSize: '0.8125rem',
    padding: '0.25rem 0.5rem',
    fontFamily: 'inherit',
    borderRadius: '4px',
    transition: 'opacity 0.1s',
    textDecoration: 'none',
  },

  actionDivider: {
    color: '#334155',
    fontSize: '0.75rem',
    userSelect: 'none',
    pointerEvents: 'none',
  },

  testResultRow: {
    marginTop: '0.5rem',
    paddingTop: '0.5rem',
    borderTop: '1px solid #1e2d3d',
    display: 'flex',
    alignItems: 'center',
    gap: '0.375rem',
  },

  testLoading: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '0.375rem',
    fontSize: '0.8125rem',
    color: '#64748b',
  },

  testSuccess: {
    fontSize: '0.8125rem',
    fontWeight: 500,
    color: '#4ade80',
  },

  testFailure: {
    fontSize: '0.8125rem',
    fontWeight: 500,
    color: '#f87171',
  },

  spinner: {
    display: 'inline-block',
    width: '0.75rem',
    height: '0.75rem',
    border: '2px solid rgba(100,116,139,0.3)',
    borderTopColor: '#64748b',
    borderRadius: '50%',
    animation: 'spin 0.7s linear infinite',
  },

  // ── Empty state ──────────────────────────────────────────────────────────

  emptyState: {
    textAlign: 'center',
    padding: '4rem 2rem',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: '0.625rem',
  },

  emptyTitle: {
    fontSize: '1rem',
    fontWeight: 600,
    color: '#cbd5e1',
    margin: '0.5rem 0 0',
  },

  emptySubtitle: {
    fontSize: '0.875rem',
    color: '#475569',
    margin: 0,
    maxWidth: '22rem',
  },

  emptyAddBtn: {
    marginTop: '0.75rem',
    background: '#3b82f6',
    color: '#fff',
    border: 'none',
    borderRadius: '8px',
    padding: '0.5rem 1.25rem',
    fontSize: '0.875rem',
    fontWeight: 500,
    fontFamily: 'inherit',
    cursor: 'pointer',
    transition: 'background 0.15s',
  },

  // ── Delete modal ─────────────────────────────────────────────────────────

  modalOverlay: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(0,0,0,0.55)',
    zIndex: 40,
  },

  modal: {
    position: 'fixed',
    top: '50%',
    left: '50%',
    transform: 'translate(-50%, -50%)',
    background: '#1e293b',
    border: '1px solid #334155',
    borderRadius: '12px',
    padding: '1.5rem',
    width: 'min(90vw, 26rem)',
    zIndex: 50,
    boxShadow: '0 20px 60px rgba(0,0,0,0.5)',
  },

  modalTitle: {
    fontSize: '1rem',
    fontWeight: 600,
    color: '#f1f5f9',
    margin: '0 0 0.75rem',
  },

  modalBody: {
    fontSize: '0.875rem',
    color: '#94a3b8',
    lineHeight: 1.6,
    margin: '0 0 1.25rem',
  },

  modalFooter: {
    display: 'flex',
    gap: '0.625rem',
    justifyContent: 'flex-end',
  },

  modalDeleteBtn: {
    background: '#ef4444',
    color: '#fff',
    border: 'none',
    borderRadius: '8px',
    padding: '0.5rem 1.125rem',
    fontSize: '0.875rem',
    fontWeight: 500,
    fontFamily: 'inherit',
    transition: 'background 0.15s',
  },

  modalCancelBtn: {
    background: 'transparent',
    color: '#94a3b8',
    border: '1px solid #334155',
    borderRadius: '8px',
    padding: '0.5rem 1.125rem',
    fontSize: '0.875rem',
    fontFamily: 'inherit',
    cursor: 'pointer',
    transition: 'background 0.15s',
  },

  modalOkBtn: {
    background: '#3b82f6',
    color: '#fff',
    border: 'none',
    borderRadius: '8px',
    padding: '0.5rem 1.5rem',
    fontSize: '0.875rem',
    fontWeight: 500,
    fontFamily: 'inherit',
    cursor: 'pointer',
    transition: 'background 0.15s',
  },

  affectedWarning: {
    background: 'rgba(251,191,36,0.08)',
    border: '1px solid rgba(251,191,36,0.25)',
    borderRadius: '6px',
    padding: '0.625rem 0.75rem',
    fontSize: '0.8125rem',
    color: '#fbbf24',
    lineHeight: 1.5,
    marginBottom: '1.25rem',
  },
};
