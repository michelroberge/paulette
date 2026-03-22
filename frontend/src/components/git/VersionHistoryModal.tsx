import { useState, useEffect, useCallback } from 'react';
import type { CommitEntry, GitStatus, Project, PipelineState } from '../../types';
import { getGitLog, getGitStatus, resetToCommit, discardChanges, setRemote, removeRemote, push, pull } from '../../api/git';

interface Props {
  projectId: string;
  onClose: () => void;
  onReset: (project: Project, pipeline: PipelineState) => void;
}

export function VersionHistoryModal({ projectId, onClose, onReset }: Props) {
  const [commits, setCommits] = useState<CommitEntry[]>([]);
  const [status, setStatus] = useState<GitStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [confirmRef, setConfirmRef] = useState<string | null>(null);
  const [remoteInput, setRemoteInput] = useState('');
  const [showRemote, setShowRemote] = useState(false);
  const [opLoading, setOpLoading] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [log, st] = await Promise.all([getGitLog(projectId), getGitStatus(projectId)]);
      setCommits(log);
      setStatus(st);
      setRemoteInput(st.remoteUrl || '');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load git info');
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => { refresh(); }, [refresh]);

  const handleDiscard = async () => {
    setOpLoading('discard');
    try {
      await discardChanges(projectId);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Discard failed');
    } finally {
      setOpLoading(null);
    }
  };

  const handleReset = async (ref: string) => {
    setOpLoading('reset');
    try {
      const result = await resetToCommit(projectId, ref);
      setConfirmRef(null);
      onReset(result.project, result.pipeline);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Reset failed');
    } finally {
      setOpLoading(null);
    }
  };

  const handleSetRemote = async () => {
    if (!remoteInput.trim()) return;
    setOpLoading('remote');
    try {
      await setRemote(projectId, remoteInput.trim());
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Set remote failed');
    } finally {
      setOpLoading(null);
    }
  };

  const handleRemoveRemote = async () => {
    setOpLoading('remote');
    try {
      await removeRemote(projectId);
      setRemoteInput('');
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Remove remote failed');
    } finally {
      setOpLoading(null);
    }
  };

  const handlePush = async () => {
    setOpLoading('push');
    try {
      await push(projectId);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Push failed');
    } finally {
      setOpLoading(null);
    }
  };

  const handlePull = async () => {
    setOpLoading('pull');
    try {
      const result = await pull(projectId);
      onReset(result.project, result.pipeline);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Pull failed');
    } finally {
      setOpLoading(null);
    }
  };

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr);
    return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal version-history-modal" onClick={e => e.stopPropagation()}>
        <div className="version-history-header">
          <h2>Version History</h2>
          <button className="close-btn" onClick={onClose}>&times;</button>
        </div>

        {error && <div className="version-history-error">{error}</div>}

        {/* Git status banner */}
        {status && !status.clean && (
          <div className="version-history-dirty">
            <span>{status.dirty} uncommitted change{status.dirty !== 1 ? 's' : ''}</span>
            <button
              disabled={opLoading !== null}
              onClick={handleDiscard}
            >
              {opLoading === 'discard' ? 'Discarding...' : 'Discard all'}
            </button>
          </div>
        )}

        {/* Commit list */}
        <div className="version-history-commits">
          {loading ? (
            <div className="version-history-loading">Loading...</div>
          ) : commits.length === 0 ? (
            <div className="version-history-empty">No commits yet</div>
          ) : (
            commits.map(c => (
              <div key={c.hash} className="commit-row">
                <div className="commit-info">
                  <div className="commit-message">
                    {c.message}
                    {c.tags?.map(tag => (
                      <span key={tag} className="commit-tag">{tag}</span>
                    ))}
                  </div>
                  <div className="commit-meta">
                    <span className="commit-hash">{c.shortHash}</span>
                    <span className="commit-author">{c.author}</span>
                    <span className="commit-date">{formatDate(c.date)}</span>
                  </div>
                </div>
                <div className="commit-actions">
                  {confirmRef === c.hash ? (
                    <div className="commit-confirm">
                      <span>Reset to here? This discards later work.</span>
                      <button
                        className="btn-danger"
                        disabled={opLoading !== null}
                        onClick={() => handleReset(c.hash)}
                      >
                        {opLoading === 'reset' ? 'Resetting...' : 'Confirm'}
                      </button>
                      <button onClick={() => setConfirmRef(null)}>Cancel</button>
                    </div>
                  ) : (
                    <button
                      className="btn-reset-to"
                      disabled={opLoading !== null}
                      onClick={() => setConfirmRef(c.hash)}
                    >
                      Reset to here
                    </button>
                  )}
                </div>
              </div>
            ))
          )}
        </div>

        {/* Remote section */}
        <div className="version-history-remote">
          <button className="remote-toggle" onClick={() => setShowRemote(!showRemote)}>
            {showRemote ? 'Hide' : 'Show'} Remote Settings
            {status?.hasRemote && <span className="remote-indicator">connected</span>}
          </button>

          {showRemote && (
            <div className="remote-panel">
              <div className="remote-url-row">
                <input
                  type="text"
                  value={remoteInput}
                  onChange={e => setRemoteInput(e.target.value)}
                  placeholder="git@github.com:user/repo.git"
                />
                <button
                  disabled={opLoading !== null || !remoteInput.trim()}
                  onClick={handleSetRemote}
                >
                  {opLoading === 'remote' ? '...' : 'Set'}
                </button>
                {status?.hasRemote && (
                  <button
                    disabled={opLoading !== null}
                    onClick={handleRemoveRemote}
                  >
                    Remove
                  </button>
                )}
              </div>
              <div className="remote-actions">
                <button
                  disabled={opLoading !== null || !status?.hasRemote}
                  onClick={handlePush}
                >
                  {opLoading === 'push' ? 'Pushing...' : 'Push'}
                </button>
                <button
                  disabled={opLoading !== null || !status?.hasRemote}
                  onClick={handlePull}
                >
                  {opLoading === 'pull' ? 'Pulling...' : 'Pull'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
