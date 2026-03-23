import { useState, useEffect } from 'react';
import { listProjects, createProject, deleteProject } from '../../api/projects';
import { importProject } from '../../api/import';
import { getConfig } from '../../api/config';
import type { Project } from '../../types';

const STAGE_ORDER = ['vision', 'ux', 'architecture', 'build', 'complete'];
const STAGE_LABELS: Record<string, string> = {
  vision: 'Vision', ux: 'UX', architecture: 'Arch', build: 'Build', complete: 'Done',
};

function StageProgress({ currentStage }: { currentStage: string }) {
  const current = STAGE_ORDER.indexOf(currentStage);
  return (
    <div className="stage-progress">
      {STAGE_ORDER.slice(0, -1).map((s, i) => (
        <div
          key={s}
          className={`stage-pip ${i < current ? 'done' : i === current ? 'active' : ''}`}
          title={STAGE_LABELS[s]}
        />
      ))}
      <span className="stage-label">{STAGE_LABELS[currentStage] ?? currentStage}</span>
    </div>
  );
}

interface Props {
  onSelect: (project: Project) => void;
}

export function ProjectList({ onSelect }: Props) {
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

  useEffect(() => {
    listProjects().then(setProjects).catch(console.error);
    getConfig().then(c => {
      setReposPath(c.reposPath);
      setAppVersion(c.version);
      setAppAuthor(c.author);
    }).catch(console.error);
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
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

  const handleImport = async (e: React.FormEvent) => {
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

  return (
    <div className="project-list">
      <div className="claudette-banner">
        <pre className="claudette-ascii">
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
        <div className="claudette-title-block">
          <h1>Claudette</h1>
          <span className="claudette-subtitle">AI App Factory</span>
          <span className="claudette-version">{appVersion ? `v${appVersion}` : ''}</span>
        </div>
      </div>
      <p>Select a project or create a new one.</p>

      <div className="project-grid">
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
            <div className="project-meta">
              <span>{p.author}</span>
              <span>v{p.version} · {new Date(p.updatedAt).toLocaleDateString()}</span>
            </div>
          </div>
        ))}

        <div className="project-card new-project" onClick={() => setShowForm(true)}>
          <span className="plus">+</span>
          <span>New Project</span>
        </div>

        <div className="project-card new-project" onClick={() => setShowImportForm(true)}>
          <span className="plus" style={{ fontSize: '1.5rem' }}>&#8615;</span>
          <span>Import Repo</span>
        </div>
      </div>

      {confirmDelete && (
        <div className="modal-overlay" onClick={() => setConfirmDelete(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>Delete Project</h2>
            <p style={{ color: '#94a3b8', marginBottom: '1.5rem' }}>
              This removes the project from the registry. The host directory is not deleted.
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
