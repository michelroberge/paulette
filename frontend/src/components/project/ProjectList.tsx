import { useState, useEffect } from 'react';
import { listProjects, createProject } from '../../api/projects';
import type { Project } from '../../types';

interface Props {
  onSelect: (project: Project) => void;
}

export function ProjectList({ onSelect }: Props) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [author, setAuthor] = useState('');
  const [hostDir, setHostDir] = useState('');

  useEffect(() => {
    listProjects().then(setProjects).catch(console.error);
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    const project = await createProject({ name, author, hostDir });
    setProjects(prev => [...prev, project]);
    setShowForm(false);
    setName('');
    setAuthor('');
    setHostDir('');
    onSelect(project);
  };

  return (
    <div className="project-list">
      <h1>AI App Factory</h1>
      <p>Select a project or create a new one.</p>

      <div className="project-grid">
        {projects.map(p => (
          <div key={p.id} className="project-card" onClick={() => onSelect(p)}>
            <h3>{p.name}</h3>
            <div className="project-meta">
              <span>Stage: {p.currentStage}</span>
              <span>v{p.version}</span>
            </div>
            <div className="project-meta">
              <span>{p.author}</span>
              <span>{new Date(p.updatedAt).toLocaleDateString()}</span>
            </div>
          </div>
        ))}

        <div className="project-card new-project" onClick={() => setShowForm(true)}>
          <span className="plus">+</span>
          <span>New Project</span>
        </div>
      </div>

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
