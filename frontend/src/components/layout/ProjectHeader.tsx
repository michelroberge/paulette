import type { Project } from '../../types';

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

interface Props {
  project: Project;
  onBack: () => void;
  totalTokens?: number;
  onShowHistory?: () => void;
}

export function ProjectHeader({ project, onBack, totalTokens, onShowHistory }: Props) {
  return (
    <header className="project-header">
      <button className="back-button" onClick={onBack}>&larr;</button>
      <h2>{project.name}</h2>
      <span className="version">
        v{project.version}{project.iteration > 1 && ` (iter ${project.iteration})`}
      </span>
      <span className="author">{project.author}</span>
      {(totalTokens ?? 0) > 0 && (
        <span className="header-token-total">{formatTokens(totalTokens!)} tok</span>
      )}
      {onShowHistory && (
        <button className="history-button" onClick={onShowHistory} title="Version History">
          History
        </button>
      )}
    </header>
  );
}
