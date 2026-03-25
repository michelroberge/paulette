import { useState, useEffect, useCallback } from 'react';
import type { CommitEntry, GitStatus, Project, PipelineState } from '../../types';
import { getGitLog, getGitStatus, resetToCommit, discardChanges, commitAll, setIdentity, renameBranch, setRemote, removeRemote, push, pull, getSSHKey } from '../../api/git';

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
  const [pushBranch, setPushBranch] = useState('');
  const [forcePush, setForcePush] = useState(false);
  const [commitMsg, setCommitMsg] = useState('');
  const [identityName, setIdentityName] = useState('');
  const [identityEmail, setIdentityEmail] = useState('');
  const [sshKey, setSSHKey] = useState<string | null>(null);
  const [sshCopied, setSSHCopied] = useState(false);
  const [pushSuccess, setPushSuccess] = useState(false);
  const [renamingBranch, setRenamingBranch] = useState(false);
  const [branchNameInput, setBranchNameInput] = useState('');

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [log, st] = await Promise.all([getGitLog(projectId), getGitStatus(projectId)]);
      setCommits(log);
      setStatus(st);
      setRemoteInput(st.remoteUrl || '');
      setPushBranch(st.remoteBranch || st.branch || 'main');
      if (!st.gitUserName) setIdentityName('');
      if (!st.gitUserEmail) setIdentityEmail('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load git info');
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => { refresh(); }, [refresh]);

  const handleSetIdentity = async () => {
    if (!identityName.trim() || !identityEmail.trim()) return;
    setOpLoading('identity');
    try {
      await setIdentity(projectId, identityName.trim(), identityEmail.trim());
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to set identity');
    } finally {
      setOpLoading(null);
    }
  };

  const handleCommit = async () => {
    if (!commitMsg.trim()) return;
    setOpLoading('commit');
    try {
      await commitAll(projectId, commitMsg.trim());
      setCommitMsg('');
      await refresh();
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

  const handleShowSSHKey = async () => {
    if (sshKey) { setSSHKey(null); return; }
    setOpLoading('ssh');
    try {
      const res = await getSSHKey(projectId);
      setSSHKey(res.publicKey);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to get SSH key');
    } finally {
      setOpLoading(null);
    }
  };

  const handleCopySSHKey = () => {
    if (!sshKey) return;
    navigator.clipboard.writeText(sshKey);
    setSSHCopied(true);
    setTimeout(() => setSSHCopied(false), 2000);
  };

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr);
    return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  const missingIdentity = status && (!status.gitUserName || !status.gitUserEmail);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal version-history-modal" onClick={e => e.stopPropagation()}>
        <div className="version-history-header">
          <h2>Version History</h2>
          <button className="close-btn" onClick={onClose}>&times;</button>
        </div>

        {error && <div className="version-history-error">{error}</div>}

        {/* Git identity banner */}
        {missingIdentity && (
          <div className="git-identity-banner">
            <span className="git-identity-title">Git identity not set</span>
            <div className="git-identity-row">
              <input
                type="text"
                placeholder="Your name"
                value={identityName}
                onChange={e => setIdentityName(e.target.value)}
              />
              <input
                type="email"
                placeholder="your@email.com"
                value={identityEmail}
                onChange={e => setIdentityEmail(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSetIdentity()}
              />
              <button
                disabled={opLoading !== null || !identityName.trim() || !identityEmail.trim()}
                onClick={handleSetIdentity}
              >
                {opLoading === 'identity' ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        )}

        {/* Git status banner */}
        {status && !status.clean && (
          <div className="version-history-dirty">
            <span>{status.dirty} uncommitted change{status.dirty !== 1 ? 's' : ''}</span>
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
                  <button disabled={opLoading !== null} onClick={handleRemoveRemote}>
                    Remove
                  </button>
                )}
              </div>

              {/* SSH key panel */}
              <div className="ssh-key-section">
                <button className="ssh-key-toggle" onClick={handleShowSSHKey} disabled={opLoading !== null}>
                  {opLoading === 'ssh' ? 'Loading…' : sshKey ? 'Hide SSH Key' : 'Show SSH Public Key'}
                </button>
                {sshKey && (
                  <div className="ssh-key-panel">
                    <p className="ssh-key-hint">
                      Add this key to your GitHub account under <strong>Settings → SSH keys</strong> to authenticate this container.
                    </p>
                    <div className="ssh-key-box">
                      <code>{sshKey}</code>
                      <button className="ssh-copy-btn" onClick={handleCopySSHKey}>
                        {sshCopied ? 'Copied!' : 'Copy'}
                      </button>
                    </div>
                  </div>
                )}
              </div>

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
              {commits.length === 0 && (
                <p className="remote-no-commits">No commits yet — commit your changes first.</p>
              )}
              <div className="remote-actions">
                <button
                  disabled={opLoading !== null || !status?.hasRemote || commits.length === 0}
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
          )}
        </div>
      </div>
    </div>
  );
}
