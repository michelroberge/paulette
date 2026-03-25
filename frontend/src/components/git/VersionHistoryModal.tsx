import { useState, useEffect, useCallback } from 'react';
import type { CommitEntry, GitStatus, Project, PipelineState } from '../../types';
import { getGitLog, getGitStatus, getBranches, resetToCommit, discardChanges, commitAll, renameBranch, setRemote, removeRemote, push, pull } from '../../api/git';
import { getProject, patchProject } from '../../api/projects';
import { UncommittedDiffViewer } from './UncommittedDiffViewer';

interface Props {
  projectId: string;
  onClose: () => void;
  onReset: (project: Project, pipeline: PipelineState) => void;
}

type GitTab = 'repo' | 'history';
const LIMIT = 10;

export function VersionHistoryModal({ projectId, onClose, onReset }: Props) {
  const [activeTab, setActiveTab] = useState<GitTab>('repo');

  // Shared state
  const [status, setStatus] = useState<GitStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [opLoading, setOpLoading] = useState<string | null>(null);

  const [showDiff, setShowDiff] = useState(false);

  // Repo tab state
  const [remoteInput, setRemoteInput] = useState('');
  const [pushBranch, setPushBranch] = useState('');
  const [forcePush, setForcePush] = useState(false);
  const [pushSuccess, setPushSuccess] = useState(false);
  const [renamingBranch, setRenamingBranch] = useState(false);
  const [branchNameInput, setBranchNameInput] = useState('');
  const [baseBranchInput, setBaseBranchInput] = useState('');
  const [baseBranchSaved, setBaseBranchSaved] = useState(false);

  // History tab state
  const [commits, setCommits] = useState<CommitEntry[]>([]);
  const [branchList, setBranchList] = useState<string[]>([]);
  const [historyBranch, setHistoryBranch] = useState('');
  const [historyOffset, setHistoryOffset] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [confirmRef, setConfirmRef] = useState<string | null>(null);
  const [commitMsg, setCommitMsg] = useState('');

  const loadCommits = useCallback(async (branch: string, offset: number, append: boolean) => {
    try {
      const entries = await getGitLog(projectId, LIMIT, offset, branch || undefined);
      setCommits(prev => append ? [...prev, ...entries] : entries);
      setHistoryOffset(offset + entries.length);
      setHasMore(entries.length === LIMIT);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load commits');
    }
  }, [projectId]);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [st, bl, proj] = await Promise.all([getGitStatus(projectId), getBranches(projectId), getProject(projectId)]);
      setStatus(st);
      setBranchList(bl.branches);
      setHistoryBranch(prev => prev || bl.current);
      setRemoteInput(st.remoteUrl || '');
      setPushBranch(st.remoteBranch || st.branch || 'main');
      setBaseBranchInput(proj.baseBranch ?? '');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load git info');
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => { refresh(); }, [refresh]);

  useEffect(() => {
    if (historyBranch) {
      setCommits([]);
      setHistoryOffset(0);
      setHasMore(true);
      loadCommits(historyBranch, 0, false);
    }
  }, [historyBranch]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleCommit = async () => {
    if (!commitMsg.trim()) return;
    setOpLoading('commit');
    try {
      await commitAll(projectId, commitMsg.trim());
      setCommitMsg('');
      await refresh();
      loadCommits(historyBranch, 0, false);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Commit failed');
    } finally {
      setOpLoading(null);
    }
  };

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

  const handleSetBaseBranch = async () => {
    setOpLoading('baseBranch');
    try {
      await patchProject(projectId, { baseBranch: baseBranchInput.trim() });
      setBaseBranchSaved(true);
      setTimeout(() => setBaseBranchSaved(false), 2000);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save base branch');
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
      await push(projectId, status?.branch, pushBranch, forcePush);
      setError(null);
      setForcePush(false);
      setPushSuccess(true);
      setTimeout(() => setPushSuccess(false), 3000);
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

  const handleRenameBranch = async () => {
    if (!branchNameInput.trim()) return;
    setOpLoading('rename');
    try {
      await renameBranch(projectId, branchNameInput.trim());
      setRenamingBranch(false);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Rename failed');
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
      <div className="modal git-modal" onClick={e => e.stopPropagation()}>
        <div className="git-modal-header">
          <h2>Git</h2>
          <button className="close-btn" onClick={onClose}>&times;</button>
        </div>

        {error && <div className="version-history-error">{error}</div>}

        <div className="git-modal-tabs">
          <button className={`git-tab${activeTab === 'repo' ? ' active' : ''}`} onClick={() => setActiveTab('repo')}>Repo</button>
          <button className={`git-tab${activeTab === 'history' ? ' active' : ''}`} onClick={() => setActiveTab('history')}>History</button>
        </div>

        <div className="git-modal-body">
          {loading && activeTab === 'repo' ? (
            <div className="version-history-loading">Loading...</div>
          ) : activeTab === 'repo' ? (
            <div className="repo-tab">
              {/* Remote URL */}
              <div className="remote-url-row">
                <input
                  type="text"
                  value={remoteInput}
                  onChange={e => setRemoteInput(e.target.value)}
                  placeholder="git@github.com:user/repo.git"
                />
                <button disabled={opLoading !== null || !remoteInput.trim()} onClick={handleSetRemote}>
                  {opLoading === 'remote' ? '...' : 'Set'}
                </button>
                {status?.hasRemote && (
                  <button disabled={opLoading !== null} onClick={handleRemoveRemote}>Remove</button>
                )}
              </div>
              {status?.remoteUrl?.startsWith('https://') && (
                <div className="remote-url-hint">
                  HTTPS remotes require credentials. Use SSH format instead: <code>git@github.com:user/repo.git</code>
                </div>
              )}

              {/* Base branch for PR automation */}
              <div className="remote-url-row base-branch-row">
                <input
                  type="text"
                  value={baseBranchInput}
                  onChange={e => setBaseBranchInput(e.target.value)}
                  placeholder="main"
                  title="Branch that iteration PRs merge into (leave empty to disable PR automation)"
                />
                <button disabled={opLoading !== null} onClick={handleSetBaseBranch}>
                  {opLoading === 'baseBranch' ? '...' : baseBranchSaved ? 'Saved!' : 'Set base branch'}
                </button>
              </div>

              {/* Uncommitted changes banner */}
              {status && !status.clean && (
                <div className="version-history-dirty">
                  <div className="version-history-dirty-top">
                    <span>{status.dirty} uncommitted change{status.dirty !== 1 ? 's' : ''}</span>
                    <button className="dirty-view-btn" onClick={() => setShowDiff(true)}>View</button>
                  </div>
                  <div className="dirty-commit-row">
                    <input
                      type="text"
                      className="dirty-commit-input"
                      placeholder="Commit message…"
                      value={commitMsg}
                      onChange={e => setCommitMsg(e.target.value)}
                      onKeyDown={e => e.key === 'Enter' && handleCommit()}
                    />
                    <button disabled={opLoading !== null || !commitMsg.trim()} onClick={handleCommit}>
                      {opLoading === 'commit' ? 'Committing...' : 'Commit'}
                    </button>
                    <button disabled={opLoading !== null} onClick={handleDiscard}>
                      {opLoading === 'discard' ? 'Discarding...' : 'Discard all'}
                    </button>
                  </div>
                </div>
              )}

              {/* Branch */}
              <div className="remote-push-row">
                {status?.branch && (
                  renamingBranch ? (
                    <div className="branch-rename-inline">
                      <input
                        type="text"
                        value={branchNameInput}
                        onChange={e => setBranchNameInput(e.target.value)}
                        onKeyDown={e => {
                          if (e.key === 'Enter') handleRenameBranch();
                          if (e.key === 'Escape') setRenamingBranch(false);
                        }}
                        autoFocus
                        placeholder={status.branch}
                      />
                      <button disabled={opLoading !== null || !branchNameInput.trim()} onClick={handleRenameBranch}>
                        {opLoading === 'rename' ? '…' : 'OK'}
                      </button>
                      <button onClick={() => setRenamingBranch(false)}>✕</button>
                    </div>
                  ) : (
                    <button
                      className="local-branch-label"
                      title="Click to rename local branch"
                      onClick={() => { setBranchNameInput(status.branch); setRenamingBranch(true); }}
                    >
                      {status.branch} ✎
                    </button>
                  )
                )}
                <input
                  type="text"
                  value={pushBranch}
                  onChange={e => setPushBranch(e.target.value)}
                  placeholder="main"
                  title="Remote branch name to push to"
                />
                <button
                  type="button"
                  className={`force-push-toggle${forcePush ? ' active' : ''}`}
                  title="Force push (-f) — overwrites remote history"
                  onClick={() => setForcePush(f => !f)}
                >
                  <span className="toggle-track"><span className="toggle-thumb" /></span>
                  <span className="toggle-label">Force</span>
                </button>
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
              {pushSuccess && <span className="push-success">Pushed successfully</span>}
            </div>
          ) : (
            <div className="history-tab">
              {/* Branch selector */}
              {branchList.length > 0 && (
                <select
                  className="git-branch-select"
                  value={historyBranch}
                  onChange={e => setHistoryBranch(e.target.value)}
                >
                  {branchList.map(b => (
                    <option key={b} value={b}>{b}</option>
                  ))}
                </select>
              )}

              {/* Commit list */}
              <div className="version-history-commits">
                {commits.length === 0 ? (
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

              {hasMore && (
                <button
                  className="git-load-more"
                  onClick={() => loadCommits(historyBranch, historyOffset, true)}
                  disabled={opLoading !== null}
                >
                  Load more
                </button>
              )}
            </div>
          )}
        </div>
      </div>
      {showDiff && (
        <UncommittedDiffViewer projectId={projectId} onClose={() => setShowDiff(false)} />
      )}
    </div>
  );
}
