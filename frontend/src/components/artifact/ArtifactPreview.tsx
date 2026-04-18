import { useEffect, useState, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { getArtifact } from '../../api/artifacts';
import { ArtifactSelectionToolbar } from './ArtifactSelectionToolbar';
import type { StageName } from '../../types';

const STAGE_LABELS: Record<string, string> = {
  vision: 'Vision',
  ux: 'UX Design',
  architecture: 'Architecture',
  build: 'Build Plan',
};

interface Props {
  projectId: string;
  stage: StageName;
  refreshTrigger: number;
  onArtifactUpdated?: () => void;
}

export function ArtifactPreview({ projectId, stage, refreshTrigger, onArtifactUpdated }: Props) {
  const [content, setContent] = useState('');
  const [exists, setExists] = useState(false);
  const [localRefresh, setLocalRefresh] = useState(0);

  useEffect(() => {
    getArtifact(projectId, stage)
      .then(res => {
        setContent(res.content);
        setExists(res.exists);
      })
      .catch(console.error);
  }, [projectId, stage, refreshTrigger, localRefresh]);

  const handleArtifactUpdated = useCallback(() => {
    setLocalRefresh(prev => prev + 1);
    onArtifactUpdated?.();
  }, [onArtifactUpdated]);

  if (!exists) {
    return (
      <div className="artifact-preview empty">
        <p>No artifact yet. Chat with the agent to generate one.</p>
      </div>
    );
  }

  return (
    <div className="artifact-preview" style={{ position: 'relative' }}>
      <h4>{STAGE_LABELS[stage] ?? stage} Artifact</h4>
      <ArtifactSelectionToolbar
        projectId={projectId}
        stage={stage}
        onArtifactUpdated={handleArtifactUpdated}
      />
      <div className="artifact-content">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
      </div>
    </div>
  );
}
