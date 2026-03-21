import type { PipelineState, StageName } from '../../types';

interface Props {
  pipeline: PipelineState | null;
  onSelectStage: (stage: StageName) => void;
  selectedStage: StageName | null;
}

const stageLabels: Record<StageName, string> = {
  vision: 'Vision',
  ux: 'UX Design',
  architecture: 'Architecture',
  build: 'Build',
  review: 'Review',
  complete: 'Complete',
};

const statusIcons: Record<string, string> = {
  locked: '\u{1F512}',
  active: '\u{25B6}',
  approved: '\u2705',
};

export function StagesSidebar({ pipeline, onSelectStage, selectedStage }: Props) {
  if (!pipeline) return null;

  return (
    <nav className="stages-sidebar">
      <h3>Pipeline</h3>
      <ul>
        {pipeline.stages.map(stage => (
          <li
            key={stage.name}
            className={`stage-item ${stage.status} ${stage.name === selectedStage ? 'selected' : ''}`}
            onClick={() => stage.status !== 'locked' && onSelectStage(stage.name)}
          >
            <span className="stage-icon">{statusIcons[stage.status]}</span>
            <span className="stage-label">{stageLabels[stage.name]}</span>
          </li>
        ))}
      </ul>
    </nav>
  );
}
