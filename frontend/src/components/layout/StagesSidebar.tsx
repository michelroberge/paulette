import { useState, useEffect } from 'react';
import type { PipelineState, StageName } from '../../types';
import { getConfig } from '../../api/config';
import { StageRobot } from './StageRobot';

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

interface Props {
  pipeline: PipelineState | null;
  onSelectStage: (stage: StageName) => void;
  selectedStage: StageName | null;
  onReset: (stage: StageName) => void;
  stageTokens?: Partial<Record<StageName, number>>;
  /** Set of stage names that have a project-level connection override active. */
  stagesWithOverrides?: Set<StageName>;
}

const stageLabels: Record<StageName, string> = {
  vision: 'Vision',
  ux: 'UX Design',
  architecture: 'Architecture',
  build: 'Build',
  complete: 'Complete',
};

export function StagesSidebar({ pipeline, onSelectStage, selectedStage, onReset, stageTokens, stagesWithOverrides }: Readonly<Props>) {
  const [confirmStage, setConfirmStage] = useState<StageName | null>(null);
  const [appVersion, setAppVersion] = useState('');
  const [appAuthor, setAppAuthor] = useState('');
  useEffect(() => {
    getConfig().then(c => {
      setAppVersion(c.version);
      setAppAuthor(c.author);
    }).catch(console.error);
  }, []);

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
          {pipeline.stages.map(stage => {
            return (
              <li
                key={stage.name}
                className={`stage-item ${stage.status} ${stage.name === selectedStage ? 'selected' : ''}`}
                onClick={() => stage.status !== 'locked' && onSelectStage(stage.name)}
              >
                <div className="stage-item-row">
                  <span
                    className={stagesWithOverrides?.has(stage.name) ? 'stage-robot-wrapper stage-robot-wrapper--override' : 'stage-robot-wrapper'}
                    title={stagesWithOverrides?.has(stage.name) ? 'Custom connection override active for this stage' : undefined}
                  >
                    <StageRobot stage={stage.name} activity={stage.activity} />
                  </span>
                  <span className="stage-name">{stageLabels[stage.name]}</span>
                  {(stageTokens?.[stage.name] ?? 0) > 0 && (
                    <span className="stage-token-count">{formatTokens(stageTokens![stage.name]!)}</span>
                  )}
                  {stage.status !== 'locked' && stage.name !== 'complete' && (
                    <button
                      className="stage-reset-btn"
                      title={stage.status === 'approved' ? 'Roll back to this stage' : 'Reset this stage'}
                      onClick={e => handleResetClick(e, stage.name)}
                    >
                      ↺
                    </button>
                  )}
                </div>

              </li>
            );
          })}
        </ul>
        <div className="sidebar-branding">
          <pre className="sidebar-ascii">
{"   ♥\n"}
{"  ╱│╲\n"}
{"┌──────┐\n"}
{"│ ◠  ◠ │\n"}
{"│ ╰──╯ │\n"}
{"└──┬───┘\n"}
{" "}
<span className="bead bead-pink">●</span>
<span className="bead bead-cyan">◉</span>
<span className="bead bead-amber">●</span>
<span className="bead bead-purple">◉</span>
<span className="bead bead-green">●</span>
          </pre>
          <div className="sidebar-brand-name">paulette {appVersion && <span>v{appVersion}</span>}</div>
          <div className="sidebar-copyright">&copy; {new Date().getFullYear()} {appAuthor || 'Michel Roberge'}</div>
        </div>
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
