import { useState } from 'react';
import { useRunLog } from '../hooks/useRunLog';
import type { RunLogEntry } from '../api/runlog';

interface Props {
  projectId: string;
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

function EntryRow({ entry }: { entry: RunLogEntry }) {
  const [expanded, setExpanded] = useState(false);
  const hasDetail = !!entry.error || (entry.artifacts && entry.artifacts.length > 0) || !!entry.notes;

  return (
    <>
      <tr
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
      </tr>
      {expanded && hasDetail && (
        <tr className="run-log-detail-row">
          <td colSpan={8} className="run-log-detail-cell">
            {entry.error && (
              <div className="run-log-detail-section">
                <div className="run-log-detail-label">Error</div>
                <pre className="run-log-error-text">{entry.error}</pre>
                {entry.errorLogPath && (
                  <div className="run-log-detail-path">
                    Error log: <code>{entry.errorLogPath}</code>
                  </div>
                )}
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

export function RunLogView({ projectId }: Readonly<Props>) {
  const { entries, loading, refresh } = useRunLog(projectId);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [stageFilter, setStageFilter] = useState<string>('all');

  const stages = Array.from(new Set(entries.map(e => e.stage))).sort();

  const filtered = entries.filter(e => {
    if (statusFilter !== 'all' && e.status !== statusFilter) return false;
    if (stageFilter !== 'all' && e.stage !== stageFilter) return false;
    return true;
  });

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
        <button className="run-log-refresh-btn" onClick={refresh} title="Refresh">↻</button>
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
              </tr>
            </thead>
            <tbody>
              {filtered.map(entry => <EntryRow key={entry.id} entry={entry} />)}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
