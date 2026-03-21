import { useState, useEffect } from 'react';
import { listProjects, createProject, deleteProject } from '../../api/projects';
import type { Project } from '../../types';

const STAGE_ORDER = ['vision', 'ux', 'architecture', 'build', 'review', 'complete'];
const STAGE_LABELS: Record<string, string> = {
  vision: 'Vision', ux: 'UX', architecture: 'Arch', build: 'Build', review: 'Review', complete: 'Done',
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
  const [name, setName] = useState('');
  const [author, setAuthor] = useState('');
  const [version, setVersion] = useState('0.1.0');
  const [hostDir, setHostDir] = useState('');
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  useEffect(() => {
    listProjects().then(setProjects).catch(console.error);
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    const project = await createProject({ name, author, hostDir, version });
    setProjects(prev => [...prev, project]);
    setShowForm(false);
    setName('');
    setAuthor('');
    setVersion('0.1.0');
    setHostDir('');
    onSelect(project);
  };

  const handleDelete = async (id: string) => {
    await deleteProject(id);
    setProjects(prev => prev.filter(p => p.id !== id));
    setConfirmDelete(null);
  };

  return (
    <div className="project-list">
      <h1>AI App Factory</h1>
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
              Host Directory
              <input value={hostDir} onChange={e => setHostDir(e.target.value)} required
                placeholder="/path/to/your/project" />
            </label>
            <div className="form-actions">
              <button type="button" onClick={() => setShowForm(false)}>Cancel</button>
              <button type="submit" className="primary">Create</button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
