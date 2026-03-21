import type { Project, StageName } from '../../types';

const ARTIFACT_STAGES: { name: StageName; label: string }[] = [
  { name: 'vision', label: 'Vision' },
  { name: 'ux', label: 'UX Design' },
  { name: 'architecture', label: 'Architecture' },
  { name: 'build', label: 'Build Plan' },
  { name: 'review', label: 'Review Report' },
];

interface Props {
  project: Project;
  onNewProject: () => void;
  onViewStage: (stage: StageName) => void;
}

export function CompletionView({ project, onNewProject, onViewStage }: Props) {
  return (
    <div className="completion-view">
      <div className="completion-header">
        <div className="completion-icon">✓</div>
        <h2>Pipeline Complete</h2>
        <p className="completion-subtitle">
          All stages have been approved for <strong>{project.name}</strong>.
        </p>
      </div>

      <div className="completion-artifacts">
        <h3>Approved Artifacts</h3>
        <ul>
          {ARTIFACT_STAGES.map(({ name, label }) => (
            <li key={name} className="completion-artifact-item">
              <span className="artifact-check">✓</span>
              <button className="artifact-link" onClick={() => onViewStage(name)}>
                {label}
              </button>
            </li>
          ))}
        </ul>
      </div>

      <div className="completion-actions">
        <button className="approve-button" onClick={onNewProject}>
          Start New Project
        </button>
      </div>
    </div>
  );
}
