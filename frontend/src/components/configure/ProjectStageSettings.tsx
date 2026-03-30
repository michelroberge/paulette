/**
 * ProjectStageSettings — slide-over panel for per-project stage connection overrides.
 *
 * Allows users to override the global stage defaults on a per-project basis.
 * Each pipeline stage has an "Inherit" toggle:
 *   ON  → uses the global default (read-only display of global connection + model)
 *   OFF → editable override (connection picker + model field auto-saved on change/blur)
 *
 * Override rows are tinted yellow to signal a custom assignment.
 * "Reset all to global defaults" reverts every stage to inherited in one action (with confirmation).
 *
 * Journey: JRN-v0.2.0-007
 * Architecture: ARCH-v0.2.0-026, SCR-011, IACT-010, IACT-012
 */

import { useState, useEffect, useCallback } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import type { StageName } from '../../types';
import type { Connection, GlobalStageConfig, ModelInfo } from '../../types/provider';
import { listConnections } from '../../api/connections';
import {
  getGlobalDefaults,
  getProjectOverrides,
  setProjectStageOverride,
  resetProjectOverrides,
} from '../../api/stageConfig';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STAGES: StageName[] = ['vision', 'ux', 'architecture', 'build', 'complete'];

const STAGE_LABELS: Record<StageName, string> = {
  vision: 'Vision',
  ux: 'UX Design',
  ui: 'UI Framework',
  architecture: 'Architecture',
  build: 'Build',
  complete: 'Complete',
};

/** Stage-specific pip colours to match the sidebar. */
const STAGE_COLORS: Record<StageName, string> = {
  vision: '#3b82f6',
  ux: '#8b5cf6',
  ui: '#ec4899',
  architecture: '#f59e0b',
  build: '#22c55e',
  complete: '#14b8a6',
};

/**
 * Sentinel connection ID meaning "use built-in Claude CLI default".
 * An empty string is intentional — the backend interprets an absent/empty connectionId
 * as falling back to the Claude CLI.
 */
const CLAUDE_CLI_ID = '';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error';

interface RowState {
  /** When true the stage inherits the global default; fields are read-only. */
  inherit: boolean;
  /** Selected connection ID ('' = Claude CLI default). */
  connectionId: string;
  /** Selected model string. */
  model: string;
  /** Visual indicator for the auto-save operation on this row. */
  saveStatus: SaveStatus;
}

type RowStates = Record<StageName, RowState>;

export interface Props {
  projectId: string;
  projectName: string;
  onClose: () => void;
  /**
   * Optional callback invoked whenever an override is saved or cleared,
   * so the parent (e.g. stage navigator) can re-render pip accents.
   */
  onOverridesChange?: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/** Returns a default RowState (inherit=true, Claude CLI sentinel). */
function defaultRow(): RowState {
  return { inherit: true, connectionId: CLAUDE_CLI_ID, model: '', saveStatus: 'idle' };
}

/** Build initial RowStates from all stages — all inheriting. */
function buildDefaultRows(): RowStates {
  return Object.fromEntries(STAGES.map(s => [s, defaultRow()])) as RowStates;
}

export function ProjectStageSettings({ projectId, projectName, onClose, onOverridesChange }: Props) {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [globalDefaults, setGlobalDefaults] = useState<GlobalStageConfig>({ stageDefaults: {} });
  const [rows, setRows] = useState<RowStates>(buildDefaultRows);
  const [loading, setLoading] = useState(true);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const [resetting, setResetting] = useState(false);

  // ── Data loading ──────────────────────────────────────────────────────────

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const [conns, defaults, overrides] = await Promise.all([
          listConnections(),
          getGlobalDefaults(),
          getProjectOverrides(projectId),
        ]);
        if (cancelled) return;

        setConnections(conns);
        setGlobalDefaults(defaults);

        // Build row states from the fetched overrides.
        // A null or absent override entry means "inherit".
        const newRows: Partial<RowStates> = {};
        for (const stage of STAGES) {
          const override = overrides.overrides[stage];
          if (!override) {
            // Inherit: show global default values in muted read-only view.
            const global = defaults.stageDefaults[stage];
            newRows[stage] = {
              inherit: true,
              connectionId: global?.connectionId ?? CLAUDE_CLI_ID,
              model: global?.model ?? '',
              saveStatus: 'idle',
            };
          } else {
            // Project override active.
            newRows[stage] = {
              inherit: false,
              connectionId: override.connectionId,
              model: override.model,
              saveStatus: 'idle',
            };
          }
        }
        setRows(newRows as RowStates);
      } catch (err) {
        console.error('[ProjectStageSettings] Failed to load settings:', err);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    return () => { cancelled = true; };
  }, [projectId]);

  // ── Row state helpers ─────────────────────────────────────────────────────

  const updateRow = useCallback((stage: StageName, patch: Partial<RowState>) => {
    setRows(prev => ({ ...prev, [stage]: { ...prev[stage], ...patch } }));
  }, []);

  /**
   * Persist a project-level override for a stage and update the save-status indicator.
   * A "Saved ✓" label fades out automatically after 2 seconds.
   */
  const saveRow = useCallback(
    async (stage: StageName, connectionId: string, model: string) => {
      updateRow(stage, { saveStatus: 'saving' });
      try {
        await setProjectStageOverride(projectId, stage, { connectionId, model });
        updateRow(stage, { saveStatus: 'saved' });
        onOverridesChange?.();
        setTimeout(() => updateRow(stage, { saveStatus: 'idle' }), 2000);
      } catch {
        updateRow(stage, { saveStatus: 'error' });
      }
    },
    [projectId, updateRow, onOverridesChange],
  );

  // ── Inherit toggle ────────────────────────────────────────────────────────

  /**
   * Toggle a stage between inherited (ON) and overridden (OFF).
   *
   * ON → OFF: pre-populate fields from the global default then immediately save
   *           so a concrete override record exists in the project config.
   * OFF → ON: send null to the API to delete the override, revert display to global.
   */
  const handleToggleInherit = useCallback(
    async (stage: StageName) => {
      const row = rows[stage];

      if (row.inherit) {
        // Switching to override — pre-populate from global defaults.
        const global = globalDefaults.stageDefaults[stage];
        const connectionId = global?.connectionId ?? CLAUDE_CLI_ID;
        const model = global?.model ?? '';
        updateRow(stage, { inherit: false, connectionId, model, saveStatus: 'idle' });
        // Save immediately to create the override record.
        await saveRow(stage, connectionId, model);
      } else {
        // Reverting to inherit — delete the override.
        updateRow(stage, { saveStatus: 'saving' });
        try {
          await setProjectStageOverride(projectId, stage, null);
          const global = globalDefaults.stageDefaults[stage];
          updateRow(stage, {
            inherit: true,
            connectionId: global?.connectionId ?? CLAUDE_CLI_ID,
            model: global?.model ?? '',
            saveStatus: 'saved',
          });
          onOverridesChange?.();
          setTimeout(() => updateRow(stage, { saveStatus: 'idle' }), 2000);
        } catch {
          updateRow(stage, { saveStatus: 'error' });
        }
      }
    },
    [rows, globalDefaults, projectId, updateRow, saveRow, onOverridesChange],
  );

  // ── Connection / model change handlers ───────────────────────────────────

  /** When the user selects a new connection, update model to that connection's defaultModel. */
  const handleConnectionChange = useCallback(
    async (stage: StageName, connectionId: string) => {
      const conn = connections.find(c => c.id === connectionId);
      const newModel = conn?.defaultModel ?? '';
      updateRow(stage, { connectionId, model: newModel });
      await saveRow(stage, connectionId, newModel);
    },
    [connections, updateRow, saveRow],
  );

  /** For the model dropdown (when discovered models are available): save immediately on change. */
  const handleModelSelect = useCallback(
    async (stage: StageName, model: string) => {
      const row = rows[stage];
      updateRow(stage, { model });
      await saveRow(stage, row.connectionId, model);
    },
    [rows, updateRow, saveRow],
  );

  /** For the free-text model input: only save on blur (not every keystroke). */
  const handleModelInput = useCallback(
    (stage: StageName, model: string) => updateRow(stage, { model }),
    [updateRow],
  );

  const handleModelBlur = useCallback(
    async (stage: StageName) => {
      const row = rows[stage];
      if (!row.inherit) {
        await saveRow(stage, row.connectionId, row.model);
      }
    },
    [rows, saveRow],
  );

  // ── Reset all ─────────────────────────────────────────────────────────────

  const handleResetAll = useCallback(async () => {
    setResetting(true);
    try {
      await resetProjectOverrides(projectId);
      // Revert all rows to inherit using fresh global defaults.
      const newRows: Partial<RowStates> = {};
      for (const stage of STAGES) {
        const global = globalDefaults.stageDefaults[stage];
        newRows[stage] = {
          inherit: true,
          connectionId: global?.connectionId ?? CLAUDE_CLI_ID,
          model: global?.model ?? '',
          saveStatus: 'idle',
        };
      }
      setRows(newRows as RowStates);
      onOverridesChange?.();
    } catch (err) {
      console.error('[ProjectStageSettings] Reset failed:', err);
    } finally {
      setResetting(false);
      setShowResetConfirm(false);
    }
  }, [projectId, globalDefaults, onOverridesChange]);

  // ── Display helpers ───────────────────────────────────────────────────────

  const getConnectionLabel = useCallback(
    (connectionId: string) => {
      if (!connectionId) return 'Claude CLI (default)';
      return connections.find(c => c.id === connectionId)?.name ?? 'Unknown connection';
    },
    [connections],
  );

  const getGlobalDisplay = useCallback(
    (stage: StageName) => {
      const def = globalDefaults.stageDefaults[stage];
      return {
        connectionName: getConnectionLabel(def?.connectionId ?? ''),
        model: def?.model || '—',
      };
    },
    [globalDefaults, getConnectionLabel],
  );

  const hasAnyOverride = STAGES.some(s => !rows[s].inherit);

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <>
      {/* Backdrop */}
      <div
        aria-hidden
        onClick={onClose}
        style={{
          position: 'fixed',
          inset: 0,
          background: 'rgba(0, 0, 0, 0.4)',
          zIndex: 49,
        }}
      />

      {/* Slide-over panel */}
      <aside
        aria-label="Stage connection settings"
        style={{
          position: 'fixed',
          top: 0,
          right: 0,
          bottom: 0,
          width: '420px',
          background: '#1e293b',
          borderLeft: '1px solid #334155',
          boxShadow: '-4px 0 32px rgba(0, 0, 0, 0.6)',
          zIndex: 50,
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        {/* ── Header ── */}
        <div
          style={{
            display: 'flex',
            alignItems: 'flex-start',
            justifyContent: 'space-between',
            padding: '1.125rem 1.25rem 1rem',
            borderBottom: '1px solid #334155',
            flexShrink: 0,
          }}
        >
          <div>
            <h2 style={{ fontSize: '0.9375rem', fontWeight: 600, color: '#e2e8f0', margin: 0 }}>
              Stage Connection Settings
            </h2>
            <p style={{ fontSize: '0.75rem', color: '#64748b', margin: '0.2rem 0 0' }}>
              {projectName}
            </p>
          </div>
          <button
            onClick={onClose}
            title="Close"
            aria-label="Close panel"
            style={{
              background: 'none',
              border: 'none',
              color: '#64748b',
              cursor: 'pointer',
              fontSize: '1.375rem',
              lineHeight: 1,
              padding: '0.125rem 0.25rem',
              borderRadius: '4px',
              marginTop: '-0.125rem',
            }}
          >
            ×
          </button>
        </div>

        {/* ── Body ── */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '1rem 1.25rem' }}>
          {loading ? (
            <p style={{ color: '#64748b', fontSize: '0.875rem', textAlign: 'center', paddingTop: '2.5rem' }}>
              Loading settings…
            </p>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.625rem' }}>
              {STAGES.map(stage => {
                const row = rows[stage];
                const isOverriding = !row.inherit;
                const globalDisplay = getGlobalDisplay(stage);
                const selectedConn = connections.find(c => c.id === row.connectionId);
                const discoveredModels = selectedConn?.discoveredModels ?? [];
                const hasDiscoveredModels = discoveredModels.length > 0;

                return (
                  <div
                    key={stage}
                    style={{
                      background: isOverriding ? 'rgba(234, 179, 8, 0.07)' : 'rgba(15, 23, 42, 0.6)',
                      border: `1px solid ${isOverriding ? 'rgba(234, 179, 8, 0.25)' : '#1e293b'}`,
                      borderRadius: '8px',
                      padding: '0.875rem 1rem',
                      transition: 'background 0.15s, border-color 0.15s',
                    }}
                  >
                    {/* Row header */}
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: '0.5rem',
                        marginBottom: isOverriding ? '0.625rem' : '0.5rem',
                      }}
                    >
                      {/* Stage pip */}
                      <span
                        style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          background: STAGE_COLORS[stage],
                          flexShrink: 0,
                          display: 'inline-block',
                        }}
                      />

                      {/* Stage label */}
                      <span
                        style={{
                          fontSize: '0.875rem',
                          fontWeight: 500,
                          color: '#e2e8f0',
                          flex: 1,
                        }}
                      >
                        {STAGE_LABELS[stage]}
                      </span>

                      {/* Save status indicator */}
                      <SaveStatusLabel
                        status={row.saveStatus}
                        onRetry={
                          row.saveStatus === 'error'
                            ? () => saveRow(stage, row.connectionId, row.model)
                            : undefined
                        }
                      />

                      {/* Inherit toggle */}
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.375rem', flexShrink: 0 }}>
                        <span style={{ fontSize: '0.6875rem', color: '#64748b' }}>Inherit</span>
                        <InheritToggle
                          on={row.inherit}
                          onToggle={() => handleToggleInherit(stage)}
                          label={`${STAGE_LABELS[stage]} inherit`}
                        />
                      </div>
                    </div>

                    {/* Fields */}
                    {row.inherit ? (
                      /* Read-only: shows global default values in muted style */
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '0.3rem' }}>
                        <FieldRow label="Connection">
                          <span style={{ fontSize: '0.75rem', color: '#475569', fontStyle: 'italic' }}>
                            {globalDisplay.connectionName}
                          </span>
                        </FieldRow>
                        <FieldRow label="Model">
                          <span style={{ fontSize: '0.75rem', color: '#475569', fontStyle: 'italic' }}>
                            {globalDisplay.model}
                          </span>
                        </FieldRow>
                      </div>
                    ) : (
                      /* Editable: connection picker + model field */
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                        {/* Connection picker */}
                        <FieldRow label="Connection">
                          <select
                            value={row.connectionId}
                            onChange={e => handleConnectionChange(stage, e.target.value)}
                            style={selectStyle}
                          >
                            <option value="">Claude CLI (default)</option>
                            {connections.map(c => (
                              <option key={c.id} value={c.id}>
                                {c.name}
                              </option>
                            ))}
                          </select>
                        </FieldRow>

                        {/* Model field */}
                        <FieldRow label="Model">
                          {hasDiscoveredModels ? (
                            <select
                              value={row.model}
                              onChange={e => handleModelSelect(stage, e.target.value)}
                              style={selectStyle}
                            >
                              {!discoveredModels.some((m: ModelInfo) => m.id === row.model) && row.model && (
                                <option value={row.model}>{row.model}</option>
                              )}
                              {discoveredModels.map((m: ModelInfo) => (
                                <option key={m.id} value={m.id}>
                                  {m.name || m.id}
                                </option>
                              ))}
                            </select>
                          ) : (
                            <input
                              type="text"
                              value={row.model}
                              onChange={e => handleModelInput(stage, e.target.value)}
                              onBlur={() => handleModelBlur(stage)}
                              placeholder="e.g. llama3:8b"
                              style={inputStyle}
                            />
                          )}
                        </FieldRow>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* ── Footer ── */}
        {/*
         * Per SCR-011 / JRN-v0.2.0-007 the "Reset all" link is always
         * present in the footer — it must not be swapped out for a different
         * element, so the footer remains stable and predictable regardless of
         * override state.  We mute it visually when there is nothing to reset.
         */}
        <div
          style={{
            borderTop: '1px solid #334155',
            padding: '0.875rem 1.25rem',
            flexShrink: 0,
          }}
        >
          <button
            onClick={() => setShowResetConfirm(true)}
            disabled={!hasAnyOverride}
            title={!hasAnyOverride ? 'All stages already inherit global defaults' : undefined}
            style={{
              background: 'none',
              border: 'none',
              color: hasAnyOverride ? '#ef4444' : '#475569',
              cursor: hasAnyOverride ? 'pointer' : 'default',
              fontSize: '0.8125rem',
              textDecoration: hasAnyOverride ? 'underline' : 'none',
              padding: 0,
              transition: 'color 0.2s',
            }}
          >
            Reset all to global defaults
          </button>
        </div>
      </aside>

      {/* ── Reset-all confirmation dialog ── */}
      {showResetConfirm && (
        <div
          aria-modal
          role="dialog"
          aria-labelledby="reset-confirm-title"
          onClick={() => setShowResetConfirm(false)}
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0, 0, 0, 0.65)',
            zIndex: 60,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '1rem',
          }}
        >
          <div
            onClick={e => e.stopPropagation()}
            style={{
              background: '#1e293b',
              border: '1px solid #334155',
              borderRadius: '12px',
              padding: '1.5rem',
              width: '380px',
              maxWidth: '100%',
              boxShadow: '0 8px 40px rgba(0, 0, 0, 0.6)',
            }}
          >
            <h3
              id="reset-confirm-title"
              style={{
                fontSize: '1rem',
                fontWeight: 600,
                color: '#e2e8f0',
                margin: '0 0 0.75rem',
              }}
            >
              Reset all stage overrides?
            </h3>
            <p
              style={{
                fontSize: '0.875rem',
                color: '#94a3b8',
                margin: '0 0 1.25rem',
                lineHeight: 1.55,
              }}
            >
              Reset all stage overrides for{' '}
              <strong style={{ color: '#e2e8f0' }}>{projectName}</strong>? Every stage will
              revert to the global defaults. This cannot be undone.
            </p>
            <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end' }}>
              <button
                onClick={() => setShowResetConfirm(false)}
                style={{
                  background: 'none',
                  border: '1px solid #334155',
                  color: '#94a3b8',
                  borderRadius: '6px',
                  padding: '0.5rem 1rem',
                  cursor: 'pointer',
                  fontSize: '0.875rem',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleResetAll}
                disabled={resetting}
                style={{
                  background: '#ef4444',
                  border: 'none',
                  color: '#fff',
                  borderRadius: '6px',
                  padding: '0.5rem 1rem',
                  cursor: resetting ? 'not-allowed' : 'pointer',
                  fontSize: '0.875rem',
                  fontWeight: 500,
                  opacity: resetting ? 0.7 : 1,
                }}
              >
                {resetting ? 'Resetting…' : 'Reset All'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Toggle button implementing the Inherit ON/OFF interaction. */
function InheritToggle({
  on,
  onToggle,
  label,
}: {
  on: boolean;
  onToggle: () => void;
  label: string;
}) {
  return (
    <button
      role="switch"
      aria-checked={on}
      aria-label={label}
      onClick={onToggle}
      title={on ? 'Using global default — click to override for this project' : 'Using project override — click to inherit global default'}
      style={{
        position: 'relative',
        width: '36px',
        height: '20px',
        borderRadius: '10px',
        background: on ? '#3b82f6' : '#374151',
        border: 'none',
        cursor: 'pointer',
        flexShrink: 0,
        transition: 'background 0.15s',
        padding: 0,
      }}
    >
      <span
        style={{
          position: 'absolute',
          top: '2px',
          left: on ? '18px' : '2px',
          width: '16px',
          height: '16px',
          borderRadius: '50%',
          background: '#fff',
          boxShadow: '0 1px 3px rgba(0, 0, 0, 0.35)',
          transition: 'left 0.15s',
          display: 'block',
        }}
      />
    </button>
  );
}

/** Inline label to the left of a form field inside a stage row. */
function FieldRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
      <span
        style={{
          fontSize: '0.6875rem',
          color: '#64748b',
          width: '72px',
          flexShrink: 0,
          textAlign: 'right',
        }}
      >
        {label}
      </span>
      <div style={{ flex: 1, minWidth: 0 }}>{children}</div>
    </div>
  );
}

/**
 * Fading "Saved ✓" / "Saving…" / "Save failed — retry?" label per row.
 *
 * The element is always present in the DOM so the CSS opacity transition
 * (transition-opacity duration-500 equivalent) plays correctly both when
 * a status appears (idle → saved) and when it fades out (saved → idle).
 * Returning null when idle would prevent the fade-out animation.
 *
 * Per IACT-010: on error, a "retry?" link is shown so the user can
 * re-trigger the save without starting over.
 */
function SaveStatusLabel({
  status,
  onRetry,
}: {
  status: SaveStatus;
  onRetry?: () => void;
}) {
  const isVisible = status !== 'idle';
  const colour =
    status === 'saved' ? '#22c55e' : status === 'saving' ? '#64748b' : '#ef4444';

  return (
    <span
      style={{
        fontSize: '0.6875rem',
        color: colour,
        flexShrink: 0,
        opacity: isVisible ? 1 : 0,
        transition: 'opacity 500ms',
        pointerEvents: isVisible ? 'auto' : 'none',
        whiteSpace: 'nowrap',
        // Reserve space so the row height doesn't shift during animation.
        minWidth: '4.5rem',
        display: 'inline-block',
        textAlign: 'right',
      }}
      aria-live="polite"
    >
      {status === 'saved' && 'Saved ✓'}
      {status === 'saving' && 'Saving…'}
      {status === 'error' && (
        <>
          Save failed
          {onRetry && (
            <>
              {' — '}
              <button
                onClick={onRetry}
                style={{
                  background: 'none',
                  border: 'none',
                  color: '#ef4444',
                  cursor: 'pointer',
                  fontSize: '0.6875rem',
                  textDecoration: 'underline',
                  padding: 0,
                }}
              >
                retry?
              </button>
            </>
          )}
        </>
      )}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Shared input / select styles
// ---------------------------------------------------------------------------

const baseFieldStyle: CSSProperties = {
  width: '100%',
  background: '#0f172a',
  color: '#e2e8f0',
  border: '1px solid #334155',
  borderRadius: '6px',
  padding: '0.3125rem 0.5rem',
  fontSize: '0.75rem',
  outline: 'none',
};

const selectStyle: CSSProperties = {
  ...baseFieldStyle,
  cursor: 'pointer',
  appearance: 'auto',
};

const inputStyle: CSSProperties = {
  ...baseFieldStyle,
};
