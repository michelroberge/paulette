import { useState, useEffect } from 'react';
import type { CSSProperties } from 'react';
import { getGlobalIdentity, getGlobalSSHKey, setGlobalIdentity } from '../../api/git';

const s = {
  section: {
    background: '#1e293b',
    border: '1px solid #334155',
    borderRadius: '0.5rem',
    padding: '1.5rem',
    marginBottom: '1.5rem',
  } as CSSProperties,

  sectionTitle: {
    fontSize: '1rem',
    fontWeight: 600,
    color: '#f1f5f9',
    marginBottom: '1rem',
    marginTop: 0,
  } as CSSProperties,

  fieldRow: {
    display: 'flex',
    flexDirection: 'column' as const,
    gap: '0.375rem',
    marginBottom: '0.875rem',
  } as CSSProperties,

  label: {
    fontSize: '0.8125rem',
    color: '#94a3b8',
    fontWeight: 500,
  } as CSSProperties,

  input: {
    background: '#0f172a',
    border: '1px solid #334155',
    borderRadius: '0.375rem',
    color: '#e2e8f0',
    fontFamily: 'inherit',
    fontSize: '0.875rem',
    padding: '0.5rem 0.75rem',
    width: '100%',
    boxSizing: 'border-box' as const,
  } as CSSProperties,

  footer: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.75rem',
  } as CSSProperties,

  saveBtn: {
    background: '#3b82f6',
    border: 'none',
    borderRadius: '0.375rem',
    color: '#fff',
    cursor: 'pointer',
    fontFamily: 'inherit',
    fontSize: '0.875rem',
    fontWeight: 500,
    padding: '0.5rem 1.25rem',
  } as CSSProperties,

  saveBtnDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  } as CSSProperties,

  savedLabel: {
    color: '#4ade80',
    fontSize: '0.875rem',
  } as CSSProperties,

  errorLabel: {
    color: '#f87171',
    fontSize: '0.875rem',
  } as CSSProperties,

  note: {
    color: '#64748b',
    fontSize: '0.8125rem',
    marginTop: '0.75rem',
    marginBottom: 0,
  } as CSSProperties,

  toggleBtn: {
    background: '#334155',
    border: '1px solid #475569',
    borderRadius: '0.375rem',
    color: '#e2e8f0',
    cursor: 'pointer',
    fontFamily: 'inherit',
    fontSize: '0.8125rem',
    padding: '0.4375rem 0.875rem',
  } as CSSProperties,

  keyArea: {
    background: '#0f172a',
    border: '1px solid #334155',
    borderRadius: '0.375rem',
    color: '#94a3b8',
    fontFamily: 'monospace',
    fontSize: '0.75rem',
    lineHeight: 1.5,
    marginTop: '0.75rem',
    padding: '0.75rem',
    resize: 'none' as const,
    width: '100%',
    boxSizing: 'border-box' as const,
  } as CSSProperties,

  copyBtn: {
    background: '#334155',
    border: '1px solid #475569',
    borderRadius: '0.375rem',
    color: '#e2e8f0',
    cursor: 'pointer',
    fontFamily: 'inherit',
    fontSize: '0.8125rem',
    marginTop: '0.5rem',
    padding: '0.375rem 0.75rem',
  } as CSSProperties,

  hint: {
    color: '#64748b',
    fontSize: '0.8125rem',
    marginTop: '0.5rem',
    marginBottom: 0,
  } as CSSProperties,
};

export function GitTab() {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [saveError, setSaveError] = useState('');

  const [sshKey, setSSHKey] = useState<string | null>(null);
  const [sshLoading, setSSHLoading] = useState(false);
  const [sshCopied, setSSHCopied] = useState(false);

  useEffect(() => {
    getGlobalIdentity()
      .then(id => { setName(id.name); setEmail(id.email); })
      .catch(() => {});
  }, []);

  const handleSave = async () => {
    if (!name.trim() || !email.trim()) return;
    setSaving(true);
    setSaveError('');
    try {
      await setGlobalIdentity(name.trim(), email.trim());
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const handleToggleSSH = async () => {
    if (sshKey) { setSSHKey(null); return; }
    setSSHLoading(true);
    try {
      const res = await getGlobalSSHKey();
      setSSHKey(res.publicKey);
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Failed to load SSH key');
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

  const canSave = name.trim() !== '' && email.trim() !== '' && !saving;

  return (
    <div>
      {/* Git Identity */}
      <div style={s.section}>
        <h2 style={s.sectionTitle}>Git Identity</h2>

        <div style={s.fieldRow}>
          <label style={s.label}>Name</label>
          <input
            style={s.input}
            type="text"
            placeholder="Your name"
            value={name}
            onChange={e => setName(e.target.value)}
          />
        </div>

        <div style={s.fieldRow}>
          <label style={s.label}>Email</label>
          <input
            style={s.input}
            type="email"
            placeholder="you@example.com"
            value={email}
            onChange={e => setEmail(e.target.value)}
          />
        </div>

        <div style={s.footer}>
          <button
            style={{ ...s.saveBtn, ...(!canSave ? s.saveBtnDisabled : {}) }}
            onClick={handleSave}
            disabled={!canSave}
            type="button"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          {saved && <span style={s.savedLabel}>Saved ✓</span>}
          {saveError && <span style={s.errorLabel}>{saveError}</span>}
        </div>

        <p style={s.note}>
          Applied to all new projects. Override per-project from the project's Profile menu.
        </p>
      </div>

      {/* SSH Public Key */}
      <div style={s.section}>
        <h2 style={s.sectionTitle}>SSH Public Key</h2>

        <button style={s.toggleBtn} onClick={handleToggleSSH} type="button" disabled={sshLoading}>
          {sshLoading ? 'Loading…' : sshKey ? 'Hide SSH Key' : 'Show SSH Key'}
        </button>

        {sshKey && (
          <>
            <textarea
              style={s.keyArea}
              readOnly
              rows={3}
              value={sshKey}
            />
            <div>
              <button style={s.copyBtn} onClick={handleCopySSH} type="button">
                {sshCopied ? 'Copied!' : 'Copy'}
              </button>
            </div>
            <p style={s.hint}>
              Add this key to GitHub under Settings → SSH and GPG keys to enable SSH clone and push.
            </p>
          </>
        )}
      </div>
    </div>
  );
}
