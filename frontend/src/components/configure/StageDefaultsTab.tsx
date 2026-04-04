/**
 * StageDefaultsTab — global per-stage connection and model assignment table,
 * with expandable sub-operation rows for UX and Build stages.
 *
 * UX expands to: Chat, Mock Generation
 * Build expands to: Generate Plan, Review, Execute Beads
 *
 * Sub-operations inherit from their parent stage by default.
 * When overridden, they save via the operation-level API endpoints.
 */

import { useState, useEffect, useCallback, useRef } from 'react';
import type { CSSProperties } from 'react';
import type { StageName } from '../../types';
import type { Connection, ModelInfo, StageAssignment, OperationKey, BuildPoolSlot } from '../../types/provider';
import { listConnections, listModels } from '../../api/connections';
import {
  getGlobalDefaults,
  setGlobalStageDefault,
  setGlobalOperationDefault,
  deleteGlobalOperationDefault,
  getBuildPool,
  setBuildPool,
  deleteBuildPool,
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

const STAGE_COLORS: Record<StageName, string> = {
  vision: '#3b82f6',
  ux: '#8b5cf6',
  ui: '#ec4899',
  architecture: '#f59e0b',
  build: '#22c55e',
  complete: '#14b8a6',
};

/** Sub-operations for stages that have them. */
interface SubOperation {
  key: OperationKey;
  label: string;
}

const STAGE_OPERATIONS: Partial<Record<StageName, SubOperation[]>> = {
  ux: [
    { key: 'ux.chat', label: 'Chat' },
    { key: 'ux.mock', label: 'Mock Generation' },
  ],
  build: [
    { key: 'build.generate', label: 'Generate Plan' },
    { key: 'build.review', label: 'Review' },
    { key: 'build.execute', label: 'Execute Beads' },
  ],
};

const CLAUDE_CLI_ID = '';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error';

interface RowState {
  connectionId: string;
  model: string;
  saveStatus: SaveStatus;
}

type RowStates = Record<string, RowState>; // keyed by stage name or operation key

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function defaultRow(): RowState {
  return { connectionId: CLAUDE_CLI_ID, model: '', saveStatus: 'idle' };
}

function buildDefaultRows(): RowStates {
  const rows: RowStates = {};
  for (const stage of STAGES) {
    rows[stage] = defaultRow();
    const ops = STAGE_OPERATIONS[stage];
    if (ops) {
      for (const op of ops) {
        rows[op.key] = defaultRow();
      }
    }
  }
  return rows;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function StageDefaultsTab() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [rows, setRows] = useState<RowStates>(buildDefaultRows);
  /** Which operations have an explicit override (not inherited from stage). */
  const [opOverridden, setOpOverridden] = useState<Record<string, boolean>>({});
  const [expandedStages, setExpandedStages] = useState<Record<StageName, boolean>>({} as Record<StageName, boolean>);
  const [stageModels, setStageModels] = useState<Record<string, ModelInfo[]>>({});
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const saveSeqRef = useRef<Record<string, number>>({});

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

        const newRows: RowStates = buildDefaultRows();
        const overridden: Record<string, boolean> = {};

        // Stage-level defaults
        for (const stage of STAGES) {
          const assignment = globalConfig.stageDefaults[stage];
          if (assignment) {
            newRows[stage] = {
              connectionId: assignment.connectionId ?? CLAUDE_CLI_ID,
              model: assignment.model ?? '',
              saveStatus: 'idle',
            };
          }
        }

        // Operation-level defaults
        if (globalConfig.operationDefaults) {
          for (const [opKey, assignment] of Object.entries(globalConfig.operationDefaults)) {
            if (assignment) {
              newRows[opKey] = {
                connectionId: assignment.connectionId ?? CLAUDE_CLI_ID,
                model: assignment.model ?? '',
                saveStatus: 'idle',
              };
              overridden[opKey] = true;
            }
          }
        }

        setRows(newRows);
        setOpOverridden(overridden);
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

  const updateRow = useCallback((key: string, patch: Partial<RowState>) => {
    setRows(prev => ({ ...prev, [key]: { ...prev[key], ...patch } }));
  }, []);

  const saveRow = useCallback(
    async (key: string, connectionId: string, model: string, isOperation: boolean) => {
      if (!saveSeqRef.current[key]) saveSeqRef.current[key] = 0;
      saveSeqRef.current[key]++;
      const mySeq = saveSeqRef.current[key];

      updateRow(key, { saveStatus: 'saving' });
      const assignment: StageAssignment = { connectionId, model };
      try {
        if (isOperation) {
          await setGlobalOperationDefault(key as OperationKey, assignment);
        } else {
          await setGlobalStageDefault(key as StageName, assignment);
        }
        if (saveSeqRef.current[key] !== mySeq) return;
        updateRow(key, { saveStatus: 'saved' });
        setTimeout(() => {
          if (saveSeqRef.current[key] === mySeq) {
            updateRow(key, { saveStatus: 'idle' });
          }
        }, 2000);
      } catch (err) {
        if (saveSeqRef.current[key] !== mySeq) return;
        console.error(`[StageDefaultsTab] Save failed for ${key}:`, err);
        updateRow(key, { saveStatus: 'error' });
      }
    },
    [updateRow],
  );

  // ── Field change handlers ──────────────────────────────────────────────────

  const handleConnectionChange = useCallback(
    async (key: string, connectionId: string, isOperation: boolean) => {
      const conn = connections.find(c => c.id === connectionId);
      const newModel = conn?.defaultModel ?? '';
      updateRow(key, { connectionId, model: newModel });
      setStageModels(prev => ({ ...prev, [key]: [] }));

      if (connectionId) {
        listModels(connectionId)
          .then(models => setStageModels(prev => ({ ...prev, [key]: models })))
          .catch(() => {});
      }

      if (isOperation) {
        setOpOverridden(prev => ({ ...prev, [key]: true }));
      }
      await saveRow(key, connectionId, newModel, isOperation);
    },
    [connections, updateRow, saveRow],
  );

  const handleModelSelect = useCallback(
    async (key: string, model: string, isOperation: boolean) => {
      const row = rows[key];
      updateRow(key, { model });
      if (isOperation) {
        setOpOverridden(prev => ({ ...prev, [key]: true }));
      }
      await saveRow(key, row.connectionId, model, isOperation);
    },
    [rows, updateRow, saveRow],
  );

  const handleModelInput = useCallback(
    (key: string, model: string) => updateRow(key, { model }),
    [updateRow],
  );

  const handleModelBlur = useCallback(
    async (key: string, isOperation: boolean) => {
      const row = rows[key];
      if (isOperation) {
        setOpOverridden(prev => ({ ...prev, [key]: true }));
      }
      await saveRow(key, row.connectionId, row.model, isOperation);
    },
    [rows, saveRow],
  );

  const handleClearOperation = useCallback(
    async (opKey: OperationKey) => {
      try {
        await deleteGlobalOperationDefault(opKey);
        setOpOverridden(prev => {
          const next = { ...prev };
          delete next[opKey];
          return next;
        });
        updateRow(opKey, { connectionId: CLAUDE_CLI_ID, model: '', saveStatus: 'idle' });
      } catch (err) {
        console.error(`[StageDefaultsTab] Failed to clear operation ${opKey}:`, err);
      }
    },
    [updateRow],
  );

  const toggleExpanded = useCallback((stage: StageName) => {
    setExpandedStages(prev => ({ ...prev, [stage]: !prev[stage] }));
  }, []);

  // ── Render ─────────────────────────────────────────────────────────────────

  if (loading) {
    return <p style={styles.loadingText}>Loading stage defaults...</p>;
  }

  if (loadError) {
    return (
      <div style={styles.errorBox}>
        <span>Warning</span> {loadError}
      </div>
    );
  }

  return (
    <div>
      <div style={styles.table} role="table" aria-label="Global stage defaults">
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
            <span role="columnheader" style={{ ...styles.headerCell, ...styles.colStatus }} />
          </div>
        </div>

        <div role="rowgroup">
          {STAGES.map((stage, idx) => {
            const row = rows[stage];
            const discoveredModels = stageModels[stage] ?? [];
            const hasDiscoveredModels = discoveredModels.length > 0;
            const isLast = idx === STAGES.length - 1;
            const ops = STAGE_OPERATIONS[stage];
            const hasOps = ops && ops.length > 0;
            const isExpanded = expandedStages[stage] ?? false;

            return (
              <div key={stage}>
                {/* Stage row */}
                <div
                  role="row"
                  style={{
                    ...styles.bodyRow,
                    borderBottom: (isLast && !isExpanded) ? 'none' : '1px solid #1e293b',
                  }}
                >
                  {/* Stage column */}
                  <div role="cell" style={{ ...styles.cell, ...styles.colStage }}>
                    {hasOps ? (
                      <button
                        onClick={() => toggleExpanded(stage)}
                        style={styles.expandBtn}
                        aria-expanded={isExpanded}
                        aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${STAGE_LABELS[stage]} sub-operations`}
                      >
                        <span style={{
                          display: 'inline-block',
                          transform: isExpanded ? 'rotate(90deg)' : 'rotate(0deg)',
                          transition: 'transform 150ms',
                          fontSize: '0.625rem',
                          color: '#64748b',
                        }}>
                          &#9654;
                        </span>
                      </button>
                    ) : (
                      <span style={{ width: '20px', display: 'inline-block' }} />
                    )}
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

                  {/* Connection column */}
                  <div role="cell" style={{ ...styles.cell, ...styles.colConnection }}>
                    <select
                      value={row.connectionId}
                      onChange={e => void handleConnectionChange(stage, e.target.value, false)}
                      aria-label={`Connection for ${STAGE_LABELS[stage]}`}
                      style={selectStyle}
                    >
                      <option value="">Claude CLI (default)</option>
                      {connections.map(c => (
                        <option key={c.id} value={c.id}>{c.name}</option>
                      ))}
                    </select>
                  </div>

                  {/* Model column */}
                  <div role="cell" style={{ ...styles.cell, ...styles.colModel }}>
                    {hasDiscoveredModels ? (
                      <select
                        value={row.model}
                        onChange={e => void handleModelSelect(stage, e.target.value, false)}
                        aria-label={`Model for ${STAGE_LABELS[stage]}`}
                        style={selectStyle}
                      >
                        {!discoveredModels.some(m => m.id === row.model) && row.model && (
                          <option value={row.model}>{row.model}</option>
                        )}
                        {discoveredModels.map(m => (
                          <option key={m.id} value={m.id}>{m.name || m.id}</option>
                        ))}
                      </select>
                    ) : (
                      <input
                        type="text"
                        value={row.model}
                        onChange={e => handleModelInput(stage, e.target.value)}
                        onBlur={() => void handleModelBlur(stage, false)}
                        placeholder="e.g. llama3:8b"
                        aria-label={`Model for ${STAGE_LABELS[stage]}`}
                        style={inputStyle}
                      />
                    )}
                  </div>

                  {/* Status column */}
                  <div role="cell" style={{ ...styles.cell, ...styles.colStatus }}>
                    <SaveStatusLabel
                      status={row.saveStatus}
                      onRetry={
                        row.saveStatus === 'error'
                          ? () => void saveRow(stage, row.connectionId, row.model, false)
                          : undefined
                      }
                    />
                  </div>
                </div>

                {/* Sub-operation rows */}
                {hasOps && isExpanded && ops!.map((op, opIdx) => {
                  const opRow = rows[op.key] ?? defaultRow();
                  const isOverridden = opOverridden[op.key] ?? false;
                  const opModels = stageModels[op.key] ?? [];
                  const hasOpModels = opModels.length > 0;
                  const isLastOp = opIdx === ops!.length - 1;

                  return (
                    <div
                      key={op.key}
                      role="row"
                      style={{
                        ...styles.bodyRow,
                        ...styles.subRow,
                        borderBottom: (isLast && isLastOp) ? 'none' : '1px solid #1e293b',
                      }}
                    >
                      {/* Sub-operation label */}
                      <div role="cell" style={{ ...styles.cell, ...styles.colStage }}>
                        <span style={{ width: '20px', display: 'inline-block' }} />
                        <span style={styles.subArrow}>&#8627;</span>
                        <span style={styles.subLabel}>{op.label}</span>
                      </div>

                      {/* Connection column — inherit or override */}
                      <div role="cell" style={{ ...styles.cell, ...styles.colConnection }}>
                        {isOverridden ? (
                          <select
                            value={opRow.connectionId}
                            onChange={e => void handleConnectionChange(op.key, e.target.value, true)}
                            aria-label={`Connection for ${op.label}`}
                            style={selectStyle}
                          >
                            <option value="">Claude CLI (default)</option>
                            {connections.map(c => (
                              <option key={c.id} value={c.id}>{c.name}</option>
                            ))}
                          </select>
                        ) : (
                          <span style={styles.inheritedLabel}>(inherited from stage)</span>
                        )}
                      </div>

                      {/* Model column */}
                      <div role="cell" style={{ ...styles.cell, ...styles.colModel }}>
                        {isOverridden ? (
                          hasOpModels ? (
                            <select
                              value={opRow.model}
                              onChange={e => void handleModelSelect(op.key, e.target.value, true)}
                              aria-label={`Model for ${op.label}`}
                              style={selectStyle}
                            >
                              {!opModels.some(m => m.id === opRow.model) && opRow.model && (
                                <option value={opRow.model}>{opRow.model}</option>
                              )}
                              {opModels.map(m => (
                                <option key={m.id} value={m.id}>{m.name || m.id}</option>
                              ))}
                            </select>
                          ) : (
                            <input
                              type="text"
                              value={opRow.model}
                              onChange={e => handleModelInput(op.key, e.target.value)}
                              onBlur={() => void handleModelBlur(op.key, true)}
                              placeholder="e.g. llama3:8b"
                              aria-label={`Model for ${op.label}`}
                              style={inputStyle}
                            />
                          )
                        ) : null}
                      </div>

                      {/* Status / actions column */}
                      <div role="cell" style={{ ...styles.cell, ...styles.colStatus }}>
                        {isOverridden ? (
                          <span style={{ display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
                            <SaveStatusLabel
                              status={opRow.saveStatus}
                              onRetry={
                                opRow.saveStatus === 'error'
                                  ? () => void saveRow(op.key, opRow.connectionId, opRow.model, true)
                                  : undefined
                              }
                            />
                            <button
                              onClick={() => void handleClearOperation(op.key)}
                              style={styles.clearBtn}
                              title="Clear override and inherit from stage"
                              aria-label={`Clear ${op.label} override`}
                            >
                              &#10005;
                            </button>
                          </span>
                        ) : (
                          <button
                            onClick={() => setOpOverridden(prev => ({ ...prev, [op.key]: true }))}
                            style={styles.overrideBtn}
                            title="Override this sub-operation"
                          >
                            Override
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>

      <p style={styles.scopeNote}>
        These defaults apply to all new projects. Existing projects can override
        settings per-stage from the{' '}
        <span style={styles.gearHint}>&#9881;</span> icon in the project header.
        Expand UX and Build stages to configure sub-operations individually.
      </p>

      <BuildPoolCard connections={connections} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

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
      : '#ef4444';

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
      {status === 'saved' && 'Saved'}
      {status === 'saving' && 'Saving...'}
      {status === 'error' && (
        <>
          Save failed
          {onRetry && (
            <>
              {' -- '}
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
// BuildPoolCard
// ---------------------------------------------------------------------------

function BuildPoolCard({ connections }: { connections: Connection[] }) {
  const [enabled, setEnabled] = useState(false);
  const [slots, setSlots] = useState<BuildPoolSlot[]>([]);
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    getBuildPool()
      .then(cfg => {
        if (cfg && cfg.slots && cfg.slots.length > 0) {
          setEnabled(true);
          setSlots(cfg.slots);
        }
        setLoaded(true);
      })
      .catch(() => setLoaded(true));
  }, []);

  const handleToggle = useCallback(async () => {
    if (enabled) {
      setSaving(true);
      try {
        await deleteBuildPool();
        setEnabled(false);
        setSlots([]);
      } catch (err) {
        console.error('Failed to disable build pool:', err);
      } finally {
        setSaving(false);
      }
    } else {
      setEnabled(true);
      if (slots.length === 0) {
        setSlots([{ connectionId: '', model: '', maxParallel: 2 }]);
      }
    }
  }, [enabled, slots]);

  const updateSlot = useCallback((idx: number, patch: Partial<BuildPoolSlot>) => {
    setSlots(prev => prev.map((s, i) => i === idx ? { ...s, ...patch } : s));
  }, []);

  const addSlot = useCallback(() => {
    setSlots(prev => [...prev, { connectionId: '', model: '', maxParallel: 2 }]);
  }, []);

  const removeSlot = useCallback((idx: number) => {
    setSlots(prev => prev.filter((_, i) => i !== idx));
  }, []);

  const handleSave = useCallback(async () => {
    const valid = slots.filter(s => s.connectionId);
    if (valid.length === 0) return;
    setSaving(true);
    try {
      await setBuildPool({ slots: valid });
    } catch (err) {
      console.error('Failed to save build pool:', err);
    } finally {
      setSaving(false);
    }
  }, [slots]);

  if (!loaded) return null;

  return (
    <div style={{
      marginTop: '1.5rem',
      background: '#1e293b',
      border: '1px solid #334155',
      borderRadius: '10px',
      padding: '1rem 1.25rem',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: enabled ? '0.75rem' : 0 }}>
        <span style={{ fontSize: '0.875rem', fontWeight: 600, color: '#e2e8f0', flex: 1 }}>
          Build Pool
        </span>
        <span style={{ fontSize: '0.6875rem', color: '#64748b' }}>
          {enabled ? 'Enabled' : 'Disabled'}
        </span>
        <button
          onClick={() => void handleToggle()}
          disabled={saving}
          style={{
            position: 'relative',
            width: '36px',
            height: '20px',
            borderRadius: '10px',
            background: enabled ? '#22c55e' : '#374151',
            border: 'none',
            cursor: 'pointer',
            flexShrink: 0,
            transition: 'background 0.15s',
            padding: 0,
          }}
        >
          <span style={{
            position: 'absolute',
            top: '2px',
            left: enabled ? '18px' : '2px',
            width: '16px',
            height: '16px',
            borderRadius: '50%',
            background: '#fff',
            boxShadow: '0 1px 3px rgba(0, 0, 0, 0.35)',
            transition: 'left 0.15s',
            display: 'block',
          }} />
        </button>
      </div>

      {enabled && (
        <>
          <p style={{ fontSize: '0.75rem', color: '#64748b', margin: '0 0 0.75rem' }}>
            Distribute bead execution across multiple connections. Each slot runs up to its parallel limit concurrently.
          </p>

          {slots.map((slot, idx) => (
            <div key={idx} style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.5rem',
              marginBottom: '0.5rem',
              padding: '0.5rem',
              background: 'rgba(15, 23, 42, 0.5)',
              borderRadius: '6px',
            }}>
              <select
                value={slot.connectionId}
                onChange={e => {
                  const conn = connections.find(c => c.id === e.target.value);
                  updateSlot(idx, {
                    connectionId: e.target.value,
                    model: conn?.defaultModel ?? slot.model,
                  });
                }}
                style={{ ...selectStyle, flex: '0 0 180px' }}
              >
                <option value="">Select connection...</option>
                {connections.map(c => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </select>

              <input
                type="text"
                value={slot.model}
                onChange={e => updateSlot(idx, { model: e.target.value })}
                placeholder="model"
                style={{ ...inputStyle, flex: 1 }}
              />

              <label style={{ display: 'flex', alignItems: 'center', gap: '0.25rem', flexShrink: 0 }}>
                <span style={{ fontSize: '0.6875rem', color: '#64748b' }}>Parallel:</span>
                <input
                  type="number"
                  min={1}
                  max={10}
                  value={slot.maxParallel}
                  onChange={e => updateSlot(idx, { maxParallel: Math.max(1, Math.min(10, Number(e.target.value))) })}
                  style={{ ...inputStyle, width: '48px', textAlign: 'center' }}
                />
              </label>

              <button
                onClick={() => removeSlot(idx)}
                style={{
                  background: 'none',
                  border: 'none',
                  color: '#64748b',
                  cursor: 'pointer',
                  fontSize: '0.875rem',
                  padding: '2px 4px',
                }}
                title="Remove slot"
              >
                &#10005;
              </button>
            </div>
          ))}

          <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
            <button
              onClick={addSlot}
              style={{
                background: 'none',
                border: '1px dashed #334155',
                borderRadius: '6px',
                color: '#64748b',
                cursor: 'pointer',
                fontSize: '0.75rem',
                padding: '0.375rem 0.75rem',
                fontFamily: 'inherit',
              }}
            >
              + Add Slot
            </button>
            <button
              onClick={() => void handleSave()}
              disabled={saving || slots.every(s => !s.connectionId)}
              style={{
                background: '#3b82f6',
                border: 'none',
                borderRadius: '6px',
                color: '#fff',
                cursor: saving ? 'not-allowed' : 'pointer',
                fontSize: '0.75rem',
                fontWeight: 500,
                padding: '0.375rem 1rem',
                opacity: saving ? 0.6 : 1,
                fontFamily: 'inherit',
              }}
            >
              {saving ? 'Saving...' : 'Save Pool'}
            </button>
          </div>
        </>
      )}
    </div>
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

  subRow: {
    background: 'rgba(15, 23, 42, 0.3)',
    paddingTop: '0.5rem',
    paddingBottom: '0.5rem',
  },

  cell: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.5rem',
    minWidth: 0,
  },

  colStage: { width: '160px', flexShrink: 0 },
  colConnection: { flex: '0 0 220px', minWidth: 0 },
  colModel: { flex: 1, minWidth: 0 },
  colStatus: { width: '6rem', flexShrink: 0, justifyContent: 'flex-end' },

  stageLabel: {
    fontSize: '0.875rem',
    fontWeight: 500,
    color: '#e2e8f0',
  },

  subArrow: {
    color: '#475569',
    fontSize: '0.875rem',
    marginRight: '0.25rem',
  },

  subLabel: {
    fontSize: '0.8125rem',
    fontWeight: 400,
    color: '#94a3b8',
  },

  inheritedLabel: {
    fontSize: '0.75rem',
    color: '#475569',
    fontStyle: 'italic',
  },

  expandBtn: {
    background: 'none',
    border: 'none',
    cursor: 'pointer',
    padding: '2px 4px',
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '20px',
    flexShrink: 0,
  },

  overrideBtn: {
    background: 'none',
    border: '1px solid #334155',
    borderRadius: '4px',
    color: '#64748b',
    cursor: 'pointer',
    fontSize: '0.6875rem',
    padding: '2px 8px',
    fontFamily: 'inherit',
  },

  clearBtn: {
    background: 'none',
    border: 'none',
    color: '#64748b',
    cursor: 'pointer',
    fontSize: '0.625rem',
    padding: '2px',
    lineHeight: 1,
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
