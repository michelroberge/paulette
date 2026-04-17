import { useState } from 'react';
import { useRunLog } from '../hooks/useRunLog';
import type { RunLogEntry } from '../api/runlog';
import { getTraceFile, deleteRun, pruneRuns } from '../api/runlog';

interface Props {
  projectId: string;
  onClose?: () => void;
}

type StatusFilter = 'all' | 'running' | 'success' | 'failed';

function formatDuration(ms?: number): string {
  if (!ms) return '—';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  const m = Math.floor(ms / 60_000);
  const s = Math.floor((ms % 60_000) / 1000);
  return `${m}m ${s}s`;
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function StatusBadge({ status }: { status: RunLogEntry['status'] }) {
  const cls = status === 'success'
    ? 'run-log-badge run-log-badge-success'
    : status === 'failed'
    ? 'run-log-badge run-log-badge-failed'
    : 'run-log-badge run-log-badge-running';
  const label = status === 'running' ? '⟳ running' : status === 'success' ? '✓ success' : '✗ failed';
  return <span className={cls}>{label}</span>;
}

interface TraceViewerProps {
  title: string;
  content: string;
  onClose: () => void;
}

function TraceViewer({ title, content, onClose }: TraceViewerProps) {
  return (
    <div className="run-log-trace-overlay" onClick={onClose}>
      <div className="run-log-trace-modal" onClick={e => e.stopPropagation()}>
        <div className="run-log-trace-header">
          <span className="run-log-trace-title">{title}</span>
          <button className="run-log-close-btn" onClick={onClose} title="Close">✕</button>
        </div>
        <pre className="run-log-trace-content">{content}</pre>
      </div>
    </div>
  );
}

interface EntryRowProps {
  entry: RunLogEntry;
  onViewTrace: (filename: string, runId: string) => void;
  onDelete: (runId: string) => void;
}

function EntryRow({ entry, onViewTrace, onDelete }: EntryRowProps) {
  const [expanded, setExpanded] = useState(false);
  const hasDetail = !!entry.error || (entry.artifacts && entry.artifacts.length > 0) || !!entry.notes || (entry.stepLogs && entry.stepLogs.length > 0);

  return (
    <><tr
        className={`run-log-row ${hasDetail ? 'run-log-row-clickable' : ''} ${entry.status === 'failed' ? 'run-log-row-failed' : ''}`}
        onClick={() => hasDetail && setExpanded(e => !e)}
      >
        <td className="run-log-cell run-log-cell-stage">{entry.stage}</td>
        <td className="run-log-cell run-log-cell-op">{entry.operation}</td>
        <td className="run-log-cell run-log-cell-attempt">#{entry.attempt}</td>
        <td className="run-log-cell"><StatusBadge status={entry.status} /></td>
        <td className="run-log-cell run-log-cell-time">{formatTime(entry.startedAt)}</td>
        <td className="run-log-cell run-log-cell-dur">{formatDuration(entry.durationMs)}</td>
        <td className="run-log-cell run-log-cell-tokens">{entry.tokensTotal ? `${entry.tokensTotal.toLocaleString()} tok` : '—'}</td>
        <td className="run-log-cell run-log-cell-info">
          {entry.status === 'failed' && entry.error
            ? <span className="run-log-error-preview">{entry.error.slice(0, 60)}{entry.error.length > 60 ? '…' : ''}</span>
            : entry.notes
            ? <span className="run-log-notes">{entry.notes}</span>
            : entry.artifacts?.length
            ? <span className="run-log-artifact-count">{entry.artifacts.length} artifact{entry.artifacts.length > 1 ? 's' : ''}</span>
            : null
          }
        </td>
        <td className="run-log-cell run-log-cell-actions">
          <button
            className="run-log-delete-btn"
            title="Delete run"
            onClick={e => { e.stopPropagation(); onDelete(entry.id); }}
          >🗑</button>
        </td>
      </tr>
      {expanded && hasDetail && (
        <tr className="run-log-detail-row">
          <td colSpan={9} className="run-log-detail-cell">
            {entry.error && (
              <div className="run-log-detail-section">
                <div className="run-log-detail-label">Error</div>
                <pre className="run-log-error-text">{entry.error}</pre>
                {entry.errorLogPath && (
                  <div className="run-log-detail-path">
                    Error log:{' '}
                    <code
                      className="run-log-trace-link"
                      onClick={e => { e.stopPropagation(); onViewTrace(entry.errorLogPath!, entry.id); }}
                    >
                      {entry.errorLogPath}
                    </code>
                  </div>
                )}
              </div>
            )}
            {entry.stepLogs && entry.stepLogs.length > 0 && (
              <div className="run-log-detail-section">
                <div className="run-log-detail-label">Agent Steps</div>
                <table className="run-log-steps-table">
                  <thead>
                    <tr>
                      <th>Step</th>
                      <th>Status</th>
                      <th>Duration</th>
                      <th>Detail</th>
                      <th>Trace</th>
                    </tr>
                  </thead>
                  <tbody>
                    {entry.stepLogs.map(s => (
                      <tr key={s.file} className={s.status === 'error' ? 'run-log-step-error' : ''}>
                        <td><code>{s.step}</code></td>
                        <td>{s.status === 'ok' ? '✓' : '✗'}</td>
                        <td>{s.durationMs ? `${(s.durationMs / 1000).toFixed(1)}s` : '—'}</td>
                        <td className="run-log-step-detail">{s.detail || '—'}</td>
                        <td>
                          <code
                            className="run-log-trace-link"
                            title="Click to view trace"
                            onClick={e => { e.stopPropagation(); onViewTrace(s.file, entry.id); }}
                          >
                            {s.file}
                          </code>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {entry.artifacts && entry.artifacts.length > 0 && (
              <div className="run-log-detail-section">
                <div className="run-log-detail-label">Artifacts</div>
                {entry.artifacts.map(a => (
                  <div key={a.path} className="run-log-artifact-row">
                    <span className="run-log-artifact-name">{a.name}</span>
                    <code
                      className="run-log-artifact-path"
                      title="Click to copy"
                      onClick={e => { e.stopPropagation(); navigator.clipboard?.writeText(a.path); }}
                    >
                      {a.path}
                    </code>
                  </div>
                ))}
              </div>
            )}
            {entry.notes && (
              <div className="run-log-detail-section">
                <div className="run-log-detail-label">Notes</div>
                <div className="run-log-notes-full">{entry.notes}</div>
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

export function RunLogView({ projectId, onClose }: Readonly<Props>) {
  const { entries, loading, refresh } = useRunLog(projectId);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [stageFilter, setStageFilter] = useState<string>('all');
  const [traceContent, setTraceContent] = useState<{ title: string; content: string } | null>(null);
  const [traceLoading, setTraceLoading] = useState(false);

  const stages = Array.from(new Set(entries.map(e => e.stage))).sort();

  const filtered = entries.filter(e => {
    if (statusFilter !== 'all' && e.status !== statusFilter) return false;
    if (stageFilter !== 'all' && e.stage !== stageFilter) return false;
    return true;
  });

  const handleViewTrace = async (filename: string, runId: string) => {
    setTraceLoading(true);
    try {
      const content = await getTraceFile(projectId, runId, filename);
      setTraceContent({ title: filename, content });
    } catch (err) {
      setTraceContent({ title: filename, content: `Failed to load trace: ${err}` });
    } finally {
      setTraceLoading(false);
    }
  };

  const handleDelete = async (runId: string) => {
    try {
      await deleteRun(projectId, runId);
      refresh();
    } catch (err) {
      console.error('Failed to delete run:', err);
    }
  };

  const handlePrune = async () => {
    try {
      const result = await pruneRuns(projectId);
      if (result.deleted > 0) refresh();
    } catch (err) {
      console.error('Failed to prune runs:', err);
    }
  };

  return (
    <div className="run-log-view">
      <div className="run-log-toolbar">
        <span className="run-log-title">Run Log</span>
        <div className="run-log-filters">
          <select
            className="run-log-select"
            value={statusFilter}
            onChange={e => setStatusFilter(e.target.value as StatusFilter)}
          >
            <option value="all">All status</option>
            <option value="running">Running</option>
            <option value="success">Success</option>
            <option value="failed">Failed</option>
          </select>
          <select
            className="run-log-select"
            value={stageFilter}
            onChange={e => setStageFilter(e.target.value)}
          >
            <option value="all">All stages</option>
            {stages.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
        </div>
        <button className="run-log-refresh-btn" onClick={handlePrune} title="Keep only last 10 runs">Prune</button>
        <button className="run-log-refresh-btn" onClick={refresh} title="Refresh">↻</button>
        {onClose && <button className="run-log-close-btn" onClick={onClose} title="Close">✕</button>}
      </div>

      {loading && <div className="run-log-loading">Loading…</div>}

      {!loading && filtered.length === 0 && (
        <div className="run-log-empty">
          {entries.length === 0
            ? 'No runs recorded yet. Runs appear here once tasks start.'
            : 'No runs match the current filters.'
          }
        </div>
      )}

      {filtered.length > 0 && (
        <div className="run-log-table-wrap">
          <table className="run-log-table">
            <thead>
              <tr>
                <th className="run-log-th">Stage</th>
                <th className="run-log-th">Operation</th>
                <th className="run-log-th">#</th>
                <th className="run-log-th">Status</th>
                <th className="run-log-th">Started</th>
                <th className="run-log-th">Duration</th>
                <th className="run-log-th">Tokens</th>
                <th className="run-log-th">Info</th>
                <th className="run-log-th"></th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(entry => (
                <EntryRow
                  key={entry.id}
                  entry={entry}
                  onViewTrace={handleViewTrace}
                  onDelete={handleDelete}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {traceLoading && (
        <div className="run-log-trace-overlay">
          <div className="run-log-trace-modal">
            <div className="run-log-loading">Loading trace…</div>
          </div>
        </div>
      )}

      {traceContent && !traceLoading && (
        <TraceViewer
          title={traceContent.title}
          content={traceContent.content}
          onClose={() => setTraceContent(null)}
        />
      )}
    </div>
  );
}
