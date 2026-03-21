import { useState } from 'react';
import type { PipelineState, StageName } from '../../types';

interface Props {
  pipeline: PipelineState | null;
  onSelectStage: (stage: StageName) => void;
  selectedStage: StageName | null;
  onReset: (stage: StageName) => void;
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

export function StagesSidebar({ pipeline, onSelectStage, selectedStage, onReset }: Props) {
  const [confirmStage, setConfirmStage] = useState<StageName | null>(null);

  if (!pipeline) return null;

  const handleResetClick = (e: React.MouseEvent, stage: StageName) => {
    e.stopPropagation();
    setConfirmStage(stage);
  };

  const handleConfirm = () => {
    if (confirmStage) {
      onReset(confirmStage);
      setConfirmStage(null);
    }
  };

  const isRollback = confirmStage
    ? pipeline.stages.findIndex(s => s.name === confirmStage) <
      pipeline.stages.findIndex(s => s.name === pipeline.currentStage)
    : false;

  return (
    <>
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
              <span className="stage-name">{stageLabels[stage.name]}</span>
              {stage.status !== 'locked' && stage.name !== 'complete' && (
                <button
                  className="stage-reset-btn"
                  title={stage.status === 'approved' ? 'Roll back to this stage' : 'Reset this stage'}
                  onClick={e => handleResetClick(e, stage.name)}
                >
                  ↺
                </button>
              )}
            </li>
          ))}
        </ul>
      </nav>

      {confirmStage && (
        <div className="modal-overlay" onClick={() => setConfirmStage(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>{isRollback ? 'Roll Back Stage' : 'Reset Stage'}</h2>
            <p style={{ color: '#94a3b8', margin: '0.75rem 0 1.5rem' }}>
              {isRollback
                ? <>Rolling back to <strong>{stageLabels[confirmStage]}</strong> will clear this stage and all subsequent stages — chat history, artifacts, and mocks. This cannot be undone.</>
                : <>Resetting <strong>{stageLabels[confirmStage]}</strong> will clear its chat history, artifact, and mock. This cannot be undone.</>
              }
            </p>
            <div className="form-actions">
              <button onClick={() => setConfirmStage(null)}>Cancel</button>
              <button className="primary danger" onClick={handleConfirm}>
                {isRollback ? 'Roll Back' : 'Reset'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
