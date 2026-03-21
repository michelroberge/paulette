import type { Project } from '../../types';

interface Props {
  project: Project;
  onBack: () => void;
}

export function ProjectHeader({ project, onBack }: Props) {
  return (
    <header className="project-header">
      <button className="back-button" onClick={onBack}>&larr;</button>
      <h2>{project.name}</h2>
      <span className="version">
        v{project.version}{project.iteration > 1 && ` (iter ${project.iteration})`}
      </span>
      <span className="author">{project.author}</span>
    </header>
  );
}
