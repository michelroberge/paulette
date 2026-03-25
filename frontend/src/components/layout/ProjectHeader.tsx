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
  autonomous?: boolean;
  onToggleAutonomous?: () => void;
}

export function ProjectHeader({ project, onBack, totalTokens, onShowHistory, autonomous, onToggleAutonomous }: Props) {
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
      {onToggleAutonomous && (
        <button
          className={`autonomous-toggle ${autonomous ? 'active' : ''}`}
          onClick={onToggleAutonomous}
          title={autonomous ? 'Autonomous mode ON — phases auto-advance' : 'Manual mode — approve each phase manually'}
        >
          {autonomous ? 'Auto' : 'Manual'}
        </button>
      )}
      {onShowHistory && (
        <button className="history-button" onClick={onShowHistory} title="Version History">
          History
        </button>
      )}
    </header>
  );
}
