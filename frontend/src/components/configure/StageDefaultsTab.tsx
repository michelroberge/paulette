/**
 * StageDefaultsTab — global per-stage connection and model assignment table.
 *
 * Displays a table with one row per pipeline stage (Vision → Complete).
 * Each row lets the user assign a named connection and model that will be
 * used as the global default for that stage in all new projects.
 *
 * Behaviour:
 *   - Changes auto-save on dropdown change or text-input blur (IACT-010).
 *   - A "Saved ✓" label fades in next to the changed field and fades out
 *     after 2 seconds; errors show "Save failed — retry?".
 *   - When a connection is changed the model field pre-populates with the
 *     connection's defaultModel so the user doesn't start from scratch.
 *   - When the selected connection has discoveredModels the model column
 *     renders a <select>; otherwise it falls back to a free-text <input>.
 *   - A note at the bottom reminds users these apply to new projects only,
 *     and that per-project overrides are set from the ⚙ icon.
 *
 * Journey: JRN-v0.2.0-006
 * Architecture: ARCH-v0.2.0-025, SCR-010, IACT-010
 */

import { useState, useEffect, useCallback } from 'react';
import type { CSSProperties } from 'react';
import type { StageName } from '../../types';
import type { Connection, StageAssignment } from '../../types/provider';
import { listConnections } from '../../api/connections';
import { getGlobalDefaults, setGlobalStageDefault } from '../../api/stageConfig';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STAGES: StageName[] = ['vision', 'ux', 'architecture', 'build', 'complete'];

const STAGE_LABELS: Record<StageName, string> = {
  vision: 'Vision',
  ux: 'UX Design',
  architecture: 'Architecture',
  build: 'Build',
  complete: 'Complete',
};

/** Stage pip colours matching the sidebar navigator. */
const STAGE_COLORS: Record<StageName, string> = {
  vision: '#3b82f6',
  ux: '#8b5cf6',
  architecture: '#f59e0b',
  build: '#22c55e',
  complete: '#14b8a6',
};

/**
 * Sentinel connection ID meaning "use the built-in Claude CLI default".
 * An empty string is stored — the backend interprets an absent / empty
 * connectionId as falling back to the Claude CLI subprocess.
 */
const CLAUDE_CLI_ID = '';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error';

interface RowState {
  /** Selected connection ID. '' = Claude CLI fallback. */
  connectionId: string;
  /** Selected model string. */
  model: string;
  /** Visual auto-save indicator for this row. */
  saveStatus: SaveStatus;
}

type RowStates = Record<StageName, RowState>;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function defaultRow(): RowState {
  return { connectionId: CLAUDE_CLI_ID, model: '', saveStatus: 'idle' };
}

function buildDefaultRows(): RowStates {
  return Object.fromEntries(STAGES.map(s => [s, defaultRow()])) as RowStates;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function StageDefaultsTab() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [rows, setRows] = useState<RowStates>(buildDefaultRows);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // ── Data loading ───────────────────────────────────────────────────────────

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const [conns, globalConfig] = await Promise.all([
          listConnections(),
          getGlobalDefaults(),
        ]);
        if (cancelled) return;

        setConnections(conns);

        const newRows: Partial<RowStates> = {};
        for (const stage of STAGES) {
          const assignment = globalConfig.stageDefaults[stage];
          newRows[stage] = {
            connectionId: assignment?.connectionId ?? CLAUDE_CLI_ID,
            model: assignment?.model ?? '',
            saveStatus: 'idle',
          };
        }
        setRows(newRows as RowStates);
      } catch (err) {
        if (!cancelled) {
          console.error('[StageDefaultsTab] Failed to load settings:', err);
          setLoadError('Failed to load settings. Please refresh and try again.');
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    return () => { cancelled = true; };
  }, []);

  // ── Row state helpers ──────────────────────────────────────────────────────

  const updateRow = useCallback((stage: StageName, patch: Partial<RowState>) => {
    setRows(prev => ({ ...prev, [stage]: { ...prev[stage], ...patch } }));
  }, []);

  /**
   * Persist the global default for a stage and animate the save-status label.
   * The "Saved ✓" label fades out automatically after 2 seconds (IACT-010).
   */
  const saveRow = useCallback(
    async (stage: StageName, connectionId: string, model: string) => {
      updateRow(stage, { saveStatus: 'saving' });
      const assignment: StageAssignment = { connectionId, model };
      try {
        await setGlobalStageDefault(stage, assignment);
        updateRow(stage, { saveStatus: 'saved' });
        setTimeout(() => updateRow(stage, { saveStatus: 'idle' }), 2000);
      } catch (err) {
        console.error(`[StageDefaultsTab] Save failed for stage ${stage}:`, err);
        updateRow(stage, { saveStatus: 'error' });
      }
    },
    [updateRow],
  );

  // ── Field change handlers ──────────────────────────────────────────────────

  /**
   * When the user picks a new connection, update the model to the connection's
   * defaultModel (so the row is immediately sensible) and save right away.
   */
  const handleConnectionChange = useCallback(
    async (stage: StageName, connectionId: string) => {
      const conn = connections.find(c => c.id === connectionId);
      const newModel = conn?.defaultModel ?? '';
      updateRow(stage, { connectionId, model: newModel });
      await saveRow(stage, connectionId, newModel);
    },
    [connections, updateRow, saveRow],
  );

  /** For a model <select> (discovered models): save immediately on change. */
  const handleModelSelect = useCallback(
    async (stage: StageName, model: string) => {
      const row = rows[stage];
      updateRow(stage, { model });
      await saveRow(stage, row.connectionId, model);
    },
    [rows, updateRow, saveRow],
  );

  /** For a free-text model <input>: update state on every keystroke, save on blur. */
  const handleModelInput = useCallback(
    (stage: StageName, model: string) => updateRow(stage, { model }),
    [updateRow],
  );

  const handleModelBlur = useCallback(
    async (stage: StageName) => {
      const row = rows[stage];
      await saveRow(stage, row.connectionId, row.model);
    },
    [rows, saveRow],
  );

  // ── Render ─────────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <p style={styles.loadingText}>Loading stage defaults…</p>
    );
  }

  if (loadError) {
    return (
      <div style={styles.errorBox}>
        <span>⚠</span> {loadError}
      </div>
    );
  }

  return (
    <div>
      {/* Stage defaults table */}
      <div style={styles.table} role="table" aria-label="Global stage defaults">
        {/* Table header */}
        <div role="rowgroup">
          <div role="row" style={styles.headerRow}>
            <span role="columnheader" style={{ ...styles.headerCell, ...styles.colStage }}>
              Stage
            </span>
            <span role="columnheader" style={{ ...styles.headerCell, ...styles.colConnection }}>
              Connection
            </span>
            <span role="columnheader" style={{ ...styles.headerCell, ...styles.colModel }}>
              Model
            </span>
            {/* Status column — empty header, reserved for auto-save indicator */}
            <span role="columnheader" style={{ ...styles.headerCell, ...styles.colStatus }} />
          </div>
        </div>

        {/* Table body */}
        <div role="rowgroup">
          {STAGES.map((stage, idx) => {
            const row = rows[stage];
            const selectedConn = connections.find(c => c.id === row.connectionId);
            const discoveredModels = selectedConn?.discoveredModels ?? [];
            const hasDiscoveredModels = discoveredModels.length > 0;
            const isLast = idx === STAGES.length - 1;

            return (
              <div
                key={stage}
                role="row"
                style={{
                  ...styles.bodyRow,
                  borderBottom: isLast ? 'none' : '1px solid #1e293b',
                }}
              >
                {/* Stage column — pip + label */}
                <div role="cell" style={{ ...styles.cell, ...styles.colStage }}>
                  <span
                    aria-hidden
                    style={{
                      display: 'inline-block',
                      width: '8px',
                      height: '8px',
                      borderRadius: '50%',
                      background: STAGE_COLORS[stage],
                      flexShrink: 0,
                    }}
                  />
                  <span style={styles.stageLabel}>{STAGE_LABELS[stage]}</span>
                </div>

                {/* Connection column — dropdown */}
                <div role="cell" style={{ ...styles.cell, ...styles.colConnection }}>
                  <select
                    value={row.connectionId}
                    onChange={e => handleConnectionChange(stage, e.target.value)}
                    aria-label={`Connection for ${STAGE_LABELS[stage]}`}
                    style={selectStyle}
                  >
                    <option value="">Claude CLI (default)</option>
                    {connections.map(c => (
                      <option key={c.id} value={c.id}>
                        {c.name}
                      </option>
                    ))}
                  </select>
                </div>

                {/* Model column — dropdown or free-text */}
                <div role="cell" style={{ ...styles.cell, ...styles.colModel }}>
                  {hasDiscoveredModels ? (
                    <select
                      value={row.model}
                      onChange={e => handleModelSelect(stage, e.target.value)}
                      aria-label={`Model for ${STAGE_LABELS[stage]}`}
                      style={selectStyle}
                    >
                      {/* Keep current value selectable even if not in discovered list */}
                      {!discoveredModels.some(m => m.id === row.model) && row.model && (
                        <option value={row.model}>{row.model}</option>
                      )}
                      {discoveredModels.map(m => (
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
                      aria-label={`Model for ${STAGE_LABELS[stage]}`}
                      style={inputStyle}
                    />
                  )}
                </div>

                {/* Status column — fading save indicator */}
                <div role="cell" style={{ ...styles.cell, ...styles.colStatus }}>
                  <SaveStatusLabel
                    status={row.saveStatus}
                    onRetry={
                      row.saveStatus === 'error'
                        ? () => saveRow(stage, row.connectionId, row.model)
                        : undefined
                    }
                  />
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Scope note */}
      <p style={styles.scopeNote}>
        These defaults apply to all new projects. Existing projects can override
        settings per-stage from the{' '}
        <span style={styles.gearHint}>⚙</span> icon in the project header.
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/**
 * Fading "Saved ✓" / "Saving…" / "Save failed — retry?" label.
 *
 * Always rendered (never null) so the opacity transition fires correctly in
 * both directions: fade-in when status changes from idle → saved, and
 * fade-out when it reverts from saved → idle (IACT-010).
 */
function SaveStatusLabel({
  status,
  onRetry,
}: {
  status: SaveStatus;
  onRetry?: () => void;
}) {
  const visible = status !== 'idle';
  const colour =
    status === 'saved'
      ? '#22c55e'
      : status === 'saving'
      ? '#64748b'
      : '#ef4444'; // error

  return (
    <span
      aria-live="polite"
      style={{
        fontSize: '0.6875rem',
        color: colour,
        opacity: visible ? 1 : 0,
        transition: 'opacity 500ms',
        pointerEvents: visible ? 'auto' : 'none',
        whiteSpace: 'nowrap',
        display: 'inline-block',
        minWidth: '4.5rem',
        textAlign: 'right',
      }}
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
                  fontFamily: 'inherit',
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
// Styles
// ---------------------------------------------------------------------------

const styles: Record<string, CSSProperties> = {
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
    background: 'rgba(239, 68, 68, 0.1)',
    border: '1px solid rgba(239, 68, 68, 0.3)',
    borderRadius: '8px',
    color: '#fca5a5',
    fontSize: '0.875rem',
    padding: '0.75rem 1rem',
    marginTop: '1rem',
  },

  table: {
    width: '100%',
    background: '#1e293b',
    border: '1px solid #334155',
    borderRadius: '10px',
    overflow: 'hidden',
  },

  headerRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.75rem',
    padding: '0.625rem 1rem',
    borderBottom: '1px solid #334155',
    background: 'rgba(15, 23, 42, 0.5)',
  },

  headerCell: {
    fontSize: '0.6875rem',
    fontWeight: 600,
    textTransform: 'uppercase' as const,
    letterSpacing: '0.04em',
    color: '#64748b',
  },

  bodyRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.75rem',
    padding: '0.75rem 1rem',
    transition: 'background 0.1s',
  },

  cell: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.5rem',
    minWidth: 0,
  },

  colStage: {
    width: '140px',
    flexShrink: 0,
  },

  colConnection: {
    flex: '0 0 220px',
    minWidth: 0,
  },

  colModel: {
    flex: 1,
    minWidth: 0,
  },

  colStatus: {
    width: '5rem',
    flexShrink: 0,
    justifyContent: 'flex-end',
  },

  stageLabel: {
    fontSize: '0.875rem',
    fontWeight: 500,
    color: '#e2e8f0',
  },

  scopeNote: {
    fontSize: '0.8125rem',
    color: '#64748b',
    marginTop: '1rem',
    lineHeight: 1.55,
  },

  gearHint: {
    fontStyle: 'normal' as const,
  },
};

// ---------------------------------------------------------------------------
// Shared field styles
// ---------------------------------------------------------------------------

const baseFieldStyle: CSSProperties = {
  width: '100%',
  background: '#0f172a',
  color: '#e2e8f0',
  border: '1px solid #334155',
  borderRadius: '6px',
  padding: '0.3125rem 0.5rem',
  fontSize: '0.8125rem',
  outline: 'none',
  fontFamily: 'inherit',
};

const selectStyle: CSSProperties = {
  ...baseFieldStyle,
  cursor: 'pointer',
  appearance: 'auto',
};

const inputStyle: CSSProperties = {
  ...baseFieldStyle,
};
