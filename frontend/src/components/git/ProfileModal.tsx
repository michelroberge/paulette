import { useState, useEffect } from 'react';
import { getGitStatus, setIdentity, getSSHKey } from '../../api/git';

interface Props {
  projectId: string;
  initialName?: string;
  onClose: () => void;
}

export function ProfileModal({ projectId, initialName, onClose }: Readonly<Props>) {
  const [name, setName] = useState(initialName || '');
  const [email, setEmail] = useState('');
  const [sshKey, setSSHKey] = useState<string | null>(null);
  const [sshCopied, setSSHCopied] = useState(false);
  const [saving, setSaving] = useState(false);
  const [sshLoading, setSSHLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    getGitStatus(projectId).then(st => {
      if (st.gitUserName) setName(st.gitUserName);
      if (st.gitUserEmail) setEmail(st.gitUserEmail);
    }).catch(e => {
      setError(e instanceof Error ? e.message : 'Failed to load profile');
    });
  }, [projectId]);

  const handleSave = async () => {
    if (!name.trim() || !email.trim()) return;
    setSaving(true);
    setError(null);
    try {
      await setIdentity(projectId, name.trim(), email.trim());
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save identity');
    } finally {
      setSaving(false);
    }
  };

  const handleShowSSH = async () => {
    if (sshKey) { setSSHKey(null); return; }
    setSSHLoading(true);
    try {
      const res = await getSSHKey(projectId);
      setSSHKey(res.publicKey);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to get SSH key');
    } finally {
      setSSHLoading(false);
    }
  };

  const handleCopySSH = () => {
    if (!sshKey) return;
    navigator.clipboard.writeText(sshKey);
    setSSHCopied(true);
    setTimeout(() => setSSHCopied(false), 2000);
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal profile-modal" onClick={e => e.stopPropagation()}>
        <div className="git-modal-header">
          <h2>Profile</h2>
          <button className="close-btn" onClick={onClose}>&times;</button>
        </div>

        {error && <div className="version-history-error">{error}</div>}

        <div className="profile-identity">
          <label>Name</label>
          <input
            type="text"
            placeholder="Your name"
            value={name}
            onChange={e => setName(e.target.value)}
          />
          <label>Email</label>
          <input
            type="email"
            placeholder="your@email.com"
            value={email}
            onChange={e => setEmail(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleSave()}
          />
          <button
            className="profile-save-btn"
            disabled={saving || !name.trim() || !email.trim()}
            onClick={handleSave}
          >
            {saving ? 'Saving...' : saved ? 'Saved!' : 'Save'}
          </button>
        </div>

        <div className="ssh-key-section">
          <button className="ssh-key-toggle" onClick={handleShowSSH} disabled={sshLoading}>
            {sshLoading ? 'Loading…' : sshKey ? 'Hide SSH Key' : 'Show SSH Public Key'}
          </button>
          {sshKey && (
            <div className="ssh-key-panel">
              <p className="ssh-key-hint">
                Add this key to your GitHub account under <strong>Settings → SSH keys</strong> to authenticate this container.
              </p>
              <div className="ssh-key-box">
                <code>{sshKey}</code>
                <button className="ssh-copy-btn" onClick={handleCopySSH}>
                  {sshCopied ? 'Copied!' : 'Copy'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
