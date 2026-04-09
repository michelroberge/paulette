import { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { listProjects, createProject, deleteProject } from '../../api/projects';
import { importProject } from '../../api/import';
import { getConfig } from '../../api/config';
import { getProjectsActivity } from '../../api/activity';
import { getAuthStatus, startLogin, logout, sendLoginCode, getOIDCUser, oidcLogout } from '../../api/auth';
import type { AuthStatus, OIDCUser } from '../../api/auth';
import { getGlobalIdentity } from '../../api/git';
import type { GlobalGitIdentity } from '../../api/git';
import { StageRobot } from '../layout/StageRobot';
import type { Project, StageName } from '../../types';

const STAGE_ORDER: StageName[] = ['vision', 'ux', 'architecture', 'build', 'complete'];

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function totalTokens(p: Project): number {
  const stage = Object.values(p.stageTokens ?? {}).reduce((s, v) => s + (v ?? 0), 0);
  return stage + (p.summaryTokens ?? 0);
}
const STAGE_LABELS: Record<string, string> = {
  vision: 'Vision', ux: 'UX', architecture: 'Arch', build: 'Build', complete: 'Done',
};

function StageProgress({ currentStage }: Readonly<{ currentStage: string }>) {
  const current = STAGE_ORDER.indexOf(currentStage as StageName);
  const getPipClass = (i: number) => {
    if (i < current) return 'done';
    if (i === current) return 'active';
    return '';
  };
  return (
    <div className="stage-progress">
      {STAGE_ORDER.slice(0, -1).map((s, i) => (
        <div
          key={s}
          className={`stage-pip ${getPipClass(i)}`}
          title={STAGE_LABELS[s]}
        >
          <StageRobot stage={s} size="small" />
        </div>
      ))}
      <span className="stage-label">{STAGE_LABELS[currentStage] ?? currentStage}</span>
    </div>
  );
}

interface Props {
  readonly onSelect: (project: Project) => void;
  readonly onConfigure?: () => void;
}

export function ProjectList({ onSelect, onConfigure }: Props) {
  const navigate = useNavigate();
  const [projects, setProjects] = useState<Project[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [showImportForm, setShowImportForm] = useState(false);
  const [name, setName] = useState('');
  const [author, setAuthor] = useState('');
  const [version, setVersion] = useState('0.1.0');
  const [hostDir, setHostDir] = useState('');
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [importRepoUrl, setImportRepoUrl] = useState('');
  const [importHostDir, setImportHostDir] = useState('');
  const [importName, setImportName] = useState('');
  const [importAuthor, setImportAuthor] = useState('');
  const [importVersion, setImportVersion] = useState('1.0.0');
  const [importing, setImporting] = useState(false);
  const [reposPath, setReposPath] = useState('');
  const [appVersion, setAppVersion] = useState('');
  const [appAuthor, setAppAuthor] = useState('');
  const [activityCounts, setActivityCounts] = useState<Record<string, number>>({});
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [oidcUser, setOidcUser] = useState<OIDCUser | null>(null);
  const [gitIdentity, setGitIdentity] = useState<GlobalGitIdentity | null>(null);
  const [gitIdentityLoaded, setGitIdentityLoaded] = useState(false);
  const [showLoginPanel, setShowLoginPanel] = useState(false);
  const [loginLines, setLoginLines] = useState<string[]>([]);
  const [loginError, setLoginError] = useState('');
  const [loginCode, setLoginCode] = useState('');
  const [loginCodeSent, setLoginCodeSent] = useState(false);
  const loginCleanup = useRef<(() => void) | null>(null);
  const authPollTimer = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    listProjects().then(setProjects).catch(console.error);
    getConfig().then(c => {
      setReposPath(c.reposPath);
      setAppVersion(c.version);
      setAppAuthor(c.author);
      if (c.oidcEnabled) {
        setOidcEnabled(true);
        getOIDCUser().then(setOidcUser).catch(() => { /* not logged in — login page will show */ });
      }
    }).catch(console.error);
    getAuthStatus().then(setAuthStatus).catch(console.error);
    getGlobalIdentity().then(id => { setGitIdentity(id); setGitIdentityLoaded(true); }).catch(() => setGitIdentityLoaded(true));
    return () => { if (authPollTimer.current) clearInterval(authPollTimer.current); };
  }, []);

  useEffect(() => {
    let cancelled = false;
    const poll = () => getProjectsActivity().then(counts => { if (!cancelled) setActivityCounts(counts); }).catch(() => {});
    poll();
    const id = setInterval(poll, 5000);
    return () => { cancelled = true; clearInterval(id); };
  }, []);

  const handleCreate = async (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    const project = await createProject({ name, author, ...(hostDir ? { hostDir } : {}), version });
    setProjects(prev => [...prev, project]);
    setShowForm(false);
    setName('');
    setAuthor('');
    setVersion('0.1.0');
    setHostDir('');
    onSelect(project);
  };

  const handleImport = async (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    setImporting(true);
    try {
      const project = await importProject({
        repoUrl: importRepoUrl || undefined,
        hostDir: importHostDir || undefined,
        name: importName || undefined,
        author: importAuthor || undefined,
        version: importVersion || undefined,
      });
      setProjects(prev => [...prev, project]);
      setShowImportForm(false);
      setImportRepoUrl('');
      setImportHostDir('');
      setImportName('');
      setImportAuthor('');
      setImportVersion('1.0.0');
      onSelect(project);
    } finally {
      setImporting(false);
    }
  };

  const handleDelete = async (id: string) => {
    await deleteProject(id);
    setProjects(prev => prev.filter(p => p.id !== id));
    setConfirmDelete(null);
  };

  const handleStartLogin = () => {
    setLoginLines([]);
    setLoginError('');
    setLoginCode('');
    setLoginCodeSent(false);
    setShowLoginPanel(true);
    loginCleanup.current = startLogin(
      (line) => setLoginLines(prev => [...prev, line]),
      () => { getAuthStatus().then(setAuthStatus).catch(console.error); setShowLoginPanel(false); },
      (msg) => setLoginError(msg),
    );
  };

  const handleLogout = async () => {
    loginCleanup.current?.();
    loginCleanup.current = null;
    if (authPollTimer.current) { clearInterval(authPollTimer.current); authPollTimer.current = null; }
    await logout().catch(console.error);
    setAuthStatus({ authenticated: false });
    setShowLoginPanel(false);
    setLoginLines([]);
  };

  const handleSubmitCode = async (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!loginCode.trim()) return;
    try {
      await sendLoginCode(loginCode.trim());
      setLoginCodeSent(true);
      setLoginCode('');
      // Keep the SSE stream open — the backend will send a "done" or "error"
      // event once the token exchange completes.  Poll as a fallback in case
      // the SSE connection drops before that event arrives.
      let attempts = 0;
      authPollTimer.current = setInterval(() => {
        attempts++;
        getAuthStatus().then(status => {
          if (status.authenticated) {
            clearInterval(authPollTimer.current!);
            authPollTimer.current = null;
            loginCleanup.current?.();
            loginCleanup.current = null;
            setAuthStatus(status);
            setShowLoginPanel(false);
          } else if (attempts >= 30) {
            clearInterval(authPollTimer.current!);
            authPollTimer.current = null;
            loginCleanup.current?.();
            loginCleanup.current = null;
            setLoginError('Authentication timed out — please try again.');
            setLoginCodeSent(false);
          }
        }).catch(console.error);
      }, 2000);
    } catch (err) {
      setLoginError(`Failed to send code: ${err instanceof Error ? err.message : String(err)}`);
    }
  };

  const handleSwitchAccount = () => {
    handleLogout().then(handleStartLogin);
  };

  // Show a login page when OIDC is enabled but user is not authenticated.
  if (oidcEnabled && !oidcUser) {
    return (
      <div className="project-list">
        <div className="paulette-banner">
          <pre className="paulette-ascii">
{"        ♥\n"}
{"       ╱│╲\n"}
{"    ┌──────────┐\n"}
{"    │  ◠    ◠  │\n"}
{"    │    ▽     │\n"}
{"    │  ╰────╯  │\n"}
{"    └─────┬────┘\n"}
{"     "}
<span className="bead bead-pink">●</span>
<span className="bead bead-cyan">◉</span>
<span className="bead bead-amber">●</span>
<span className="bead bead-purple">◉</span>
<span className="bead bead-green">●</span>
<span className="bead bead-pink">◉</span>
<span className="bead bead-cyan">●</span>
<span className="bead bead-amber">◉</span>
<span className="bead bead-purple">●</span>
{"\n"}
{"    ┌─────┴────┐\n"}
{"    │  ░▓░▓░▓  │\n"}
{"    │  ▓░▓░▓░  │\n"}
{"    └──┬────┬──┘\n"}
{"       │    │\n"}
{"      ═╧═  ═╧═"}
          </pre>
          <div className="paulette-title-block">
            <h1>paulette</h1>
            <span className="paulette-subtitle">Claude's wannabe assistant</span>
            <span className="paulette-version">{appVersion ? `v${appVersion}` : ''}</span>
          </div>
        </div>
        <div className="oidc-login-prompt">
          <p>Sign in to continue</p>
          <button onClick={() => { globalThis.location.href = '/api/auth/oidc/login'; }}>
            Sign in with SSO
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="project-list">
      <div className="paulette-banner">
        {onConfigure && (
          <button className="paulette-banner-cog" onClick={onConfigure} title="Configure Paulette">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
            </svg>
          </button>
        )}
        <pre className="paulette-ascii">
{"        ♥\n"}
{"       ╱│╲\n"}
{"    ┌──────────┐\n"}
{"    │  ◠    ◠  │\n"}
{"    │    ▽     │\n"}
{"    │  ╰────╯  │\n"}
{"    └─────┬────┘\n"}
{"     "}
<span className="bead bead-pink">●</span>
<span className="bead bead-cyan">◉</span>
<span className="bead bead-amber">●</span>
<span className="bead bead-purple">◉</span>
<span className="bead bead-green">●</span>
<span className="bead bead-pink">◉</span>
<span className="bead bead-cyan">●</span>
<span className="bead bead-amber">◉</span>
<span className="bead bead-purple">●</span>
{"\n"}
{"    ┌─────┴────┐\n"}
{"    │  ░▓░▓░▓  │\n"}
{"    │  ▓░▓░▓░  │\n"}
{"    └──┬────┬──┘\n"}
{"       │    │\n"}
{"      ═╧═  ═╧═"}
        </pre>
        <div className="paulette-title-block">
          <h1>paulette</h1>
          <span className="paulette-subtitle">Claude's wannabe assistant</span>
          <span className="paulette-version">{appVersion ? `v${appVersion}` : ''}</span>
        </div>
      </div>

      {oidcEnabled && oidcUser && (
        <div className="auth-banner auth-ok">
          <span className="auth-dot">●</span>
          <span>OIDC: Signed in as {oidcUser.name || oidcUser.email}</span>
          <button className="auth-action" onClick={() => oidcLogout()}>Sign Out</button>
        </div>
      )}

      {authStatus && (
        <div className={`auth-banner ${authStatus.authenticated ? 'auth-ok' : 'auth-warn'}`}>
          {authStatus.authenticated ? (
            <>
              <span className="auth-dot">●</span>
              <span>Claude authenticated{authStatus.account && authStatus.account !== 'api-key' ? ` · ${authStatus.account}` : ''}</span>
              <button className="auth-action" onClick={handleSwitchAccount}>Switch Account</button>
            </>
          ) : (
            <>
              <span className="auth-dot">●</span>
              <span>Not authenticated</span>
              <button className="auth-action" onClick={handleStartLogin}>Login</button>
            </>
          )}
        </div>
      )}

      {gitIdentityLoaded && (
        <div className={`auth-banner ${gitIdentity?.name && gitIdentity?.email ? 'auth-ok' : 'auth-warn'}`}>
          {gitIdentity?.name && gitIdentity?.email ? (
            <>
              <span className="auth-dot">●</span>
              <span>Git identity: {gitIdentity.name} &lt;{gitIdentity.email}&gt;</span>
              <button className="auth-action" onClick={() => navigate('/configure?tab=git')}>Configure</button>
            </>
          ) : (
            <>
              <span className="auth-dot">●</span>
              <span>Git identity not set — SSH clone may fail</span>
              <button className="auth-action" onClick={() => navigate('/configure?tab=git')}>Set Up Git</button>
            </>
          )}
        </div>
      )}

      {showLoginPanel && (
        <div className="auth-login-panel">
          <div className="auth-login-header">
            <span>Claude Login</span>
            <button className="auth-close" onClick={() => { loginCleanup.current?.(); setShowLoginPanel(false); }}>×</button>
          </div>
          <div className="auth-login-output">
            {loginLines.length === 0 && !loginError && <span className="auth-waiting">Starting auth flow…</span>}
            {loginLines.map((line, i) => {
              const urlMatch = line.match(/https?:\/\/\S+/);
              if (urlMatch) {
                const before = line.slice(0, urlMatch.index);
                const after = line.slice((urlMatch.index ?? 0) + urlMatch[0].length);
                return (
                  <div key={i} className="auth-line">
                    {before}
                    <a href={urlMatch[0]} target="_blank" rel="noreferrer" className="auth-url">{urlMatch[0]}</a>
                    {after}
                  </div>
                );
              }
              return <div key={i} className="auth-line">{line}</div>;
            })}
            {loginError && <div className="auth-line auth-error">{loginError}</div>}
          </div>
          {loginLines.some(l => /https?:\/\//.test(l)) && !loginCodeSent && (
            <form className="auth-code-form" onSubmit={handleSubmitCode}>
              <input
                className="auth-code-input"
                placeholder="Paste the redirect URL from your browser address bar…"
                value={loginCode}
                onChange={e => setLoginCode(e.target.value)}
                autoFocus
              />
              <button type="submit" className="auth-action" disabled={!loginCode.trim()}>Submit</button>
            </form>
          )}
          {loginCodeSent && <p className="auth-hint">Completing auth — please wait…</p>}
          {!loginCodeSent && (
            <p className="auth-hint">
              Open the link above and authorize. If the redirect page fails to load, copy the full URL from your browser&apos;s address bar and paste it above.
            </p>
          )}
        </div>
      )}

      <p>Select a project or create a new one.</p>

      <div className="project-grid">

        <div className="project-card new-project" onClick={() => setShowForm(true)}>
          <span className="plus">+</span>
          <span>New Project</span>
        </div>

        <div className="project-card new-project" onClick={() => setShowImportForm(true)}>
          <span className="plus" style={{ fontSize: '1.5rem' }}>&#8615;</span>
          <span>Import Repo</span>
        </div>
        
        {projects.map(p => (
          <div key={p.id} className="project-card" onClick={() => onSelect(p)}>
            <div className="project-card-header">
              <h3>{p.name}</h3>
              <button
                className="delete-button"
                title="Delete project"
                onClick={e => { e.stopPropagation(); setConfirmDelete(p.id); }}
              >
                ×
              </button>
            </div>
            <StageProgress currentStage={p.currentStage} />
            {(activityCounts[p.id] ?? 0) > 0 && (
              <div className="project-card-activity">
                <span className="activity-pulse" />
                <span>{activityCounts[p.id]} agent{activityCounts[p.id] > 1 ? 's' : ''} running</span>
              </div>
            )}
            <div className="project-card-stats">
              <span title="Total tokens consumed">{fmtTokens(totalTokens(p))} tokens</span>
              <span title="Completed iterations">{p.iteration} iter</span>
            </div>
            <div className="project-meta">
              <span>{p.author}</span>
              <span>v{p.version} · {new Date(p.updatedAt).toLocaleDateString()}</span>
            </div>
          </div>
        ))}



      </div>

      {confirmDelete && (
        <div className="modal-overlay" onClick={() => setConfirmDelete(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>Delete Project</h2>
            <p style={{ color: '#f87171', marginBottom: '1.5rem' }}>
              Warning: this will permanently delete the project directory from disk. This action cannot be undone.
            </p>
            <div className="form-actions">
              <button onClick={() => setConfirmDelete(null)}>Cancel</button>
              <button className="primary danger" onClick={() => handleDelete(confirmDelete)}>Delete</button>
            </div>
          </div>
        </div>
      )}

      {showForm && (
        <div className="modal-overlay" onClick={() => setShowForm(false)}>
          <form className="modal" onClick={e => e.stopPropagation()} onSubmit={handleCreate}>
            <h2>New Project</h2>
            <label>
              Project Name
              <input value={name} onChange={e => setName(e.target.value)} required />
            </label>
            <label>
              Author
              <input value={author} onChange={e => setAuthor(e.target.value)} />
            </label>
            <label>
              Version
              <input value={version} onChange={e => setVersion(e.target.value)} placeholder="0.1.0" />
            </label>
            <label>
              Host Directory <span style={{ color: '#64748b', fontSize: '0.8rem' }}>(optional — defaults to repos/&lt;name&gt;)</span>
              <input value={hostDir} onChange={e => setHostDir(e.target.value)}
                placeholder={reposPath ? `${reposPath}/${name || '<project-name>'}` : '/path/to/your/project'} />
            </label>
            <div className="form-actions">
              <button type="button" onClick={() => setShowForm(false)}>Cancel</button>
              <button type="submit" className="primary">Create</button>
            </div>
          </form>
        </div>
      )}

      {showImportForm && (
        <div className="modal-overlay" onClick={() => setShowImportForm(false)}>
          <form className="modal" onClick={e => e.stopPropagation()} onSubmit={handleImport}>
            <h2>Import Existing Repo</h2>
            <p style={{ color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1rem' }}>
              Clone a remote repo or point to a local directory. Artifacts will be auto-generated from the codebase.
            </p>
            <label>
              Git Repo URL <span style={{ color: '#64748b', fontSize: '0.8rem' }}>(optional — leave blank for local)</span>
              <input value={importRepoUrl} onChange={e => setImportRepoUrl(e.target.value)}
                placeholder="https://github.com/user/repo.git" />
            </label>
            <label>
              Host Directory {!importRepoUrl && <span style={{ color: '#64748b', fontSize: '0.8rem' }}>(required for local import)</span>}
              {importRepoUrl && <span style={{ color: '#64748b', fontSize: '0.8rem' }}>(optional — defaults to repos/&lt;repo-name&gt;)</span>}
              <input value={importHostDir} onChange={e => setImportHostDir(e.target.value)}
                required={!importRepoUrl}
                placeholder={importRepoUrl && reposPath
                  ? `${reposPath}/${importRepoUrl.split('/').pop()?.replace('.git', '') || ''}`
                  : '/path/to/local/repo'} />
            </label>
            <label>
              Project Name <span style={{ color: '#64748b', fontSize: '0.8rem' }}>(optional — defaults to directory name)</span>
              <input value={importName} onChange={e => setImportName(e.target.value)} />
            </label>
            <label>
              Author
              <input value={importAuthor} onChange={e => setImportAuthor(e.target.value)} />
            </label>
            <label>
              Version
              <input value={importVersion} onChange={e => setImportVersion(e.target.value)} placeholder="1.0.0" />
            </label>
            <div className="form-actions">
              <button type="button" onClick={() => setShowImportForm(false)}>Cancel</button>
              <button type="submit" className="primary" disabled={importing}>
                {importing ? 'Importing...' : 'Import'}
              </button>
            </div>
          </form>
        </div>
      )}

      <footer className="copyright">
        &copy; {new Date().getFullYear()} {appAuthor || 'Michel Roberge'}. All rights reserved.
      </footer>
    </div>
  );
}
