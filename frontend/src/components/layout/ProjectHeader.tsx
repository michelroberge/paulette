import { useEffect, useState } from 'react';
import type { Project } from '../../types';
import { getConfig } from '../../api/config';
import { RAGStatusBadge } from './RAGStatusBadge';

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
  onShowProfile?: () => void;
  onShowStageSettings?: () => void;
  onShowRunLog?: () => void;
  autonomous?: boolean;
  onToggleAutonomous?: () => void;
  onChangeAIMode?: (mode: 'files' | 'rag') => void;
  onShowWorkflow?: () => void;
  onVersionClick?: () => void;
  viewingVersion?: string | null;
}

export function ProjectHeader({ project, onBack, totalTokens, onShowHistory, onShowProfile, onShowStageSettings, onShowRunLog, autonomous, onToggleAutonomous, onChangeAIMode, onShowWorkflow, onVersionClick, viewingVersion }: Readonly<Props>) {
  const [ragEnabled, setRagEnabled] = useState(false);

  useEffect(() => {
    getConfig().then(c => setRagEnabled(c.ragEnabled)).catch(() => {});
  }, []);

  const aiMode = project.aiMode || 'files';

  return (
    <header className="project-header">
      <button className="back-button" onClick={onBack}>&larr;</button>
      <h2>{project.name}</h2>
      {onVersionClick ? (
        <button className="version version-btn" onClick={onVersionClick} title="Browse version history">
          v{project.version}{project.iteration > 1 && ` (iter ${project.iteration})`}
          {viewingVersion && <span className="version-readonly-badge"> — viewing v{viewingVersion}</span>}
        </button>
      ) : (
        <span className="version">
          v{project.version}{project.iteration > 1 && ` (iter ${project.iteration})`}
        </span>
      )}
      {onShowProfile
        ? <button className="author-button" onClick={onShowProfile}>{project.author}</button>
        : <span className="author">{project.author}</span>
      }
      {(totalTokens ?? 0) > 0 && (
        <span className="header-token-total">{formatTokens(totalTokens!)} tok</span>
      )}
      <RAGStatusBadge />
      {onChangeAIMode && (
        <select
          className="ai-mode-select"
          value={aiMode}
          onChange={e => onChangeAIMode(e.target.value as 'files' | 'rag')}
          title="AI prompt mode"
        >
          <option value="files">Files</option>
          <option value="rag" disabled={!ragEnabled}>RAG{!ragEnabled ? ' (not configured)' : ''}</option>
        </select>
      )}
      {onShowWorkflow && (
        <button
          className="workflow-button"
          onClick={onShowWorkflow}
          title="View workflow & edit prompts"
          aria-label="Workflow editor"
        >
          Workflow
        </button>
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
      {onShowStageSettings && (
        <button
          className="stage-settings-button"
          onClick={onShowStageSettings}
          title="Stage connection settings"
          aria-label="Stage connection settings"
        >
          ⚙
        </button>
      )}
      {onShowRunLog && (
        <button
          className="run-log-button"
          onClick={onShowRunLog}
          title="Run log"
          aria-label="Run log"
        >
          ▤
        </button>
      )}
      {onShowHistory && (
        <button className="history-button" onClick={onShowHistory} title="Git">
          Git
        </button>
      )}
    </header>
  );
}
