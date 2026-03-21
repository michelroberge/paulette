import { useEffect, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import { getArtifact } from '../../api/artifacts';
import type { StageName } from '../../types';

const STAGE_LABELS: Record<string, string> = {
  vision: 'Vision',
  ux: 'UX Design',
  architecture: 'Architecture',
  build: 'Build Plan',
  review: 'Review',
};

interface Props {
  projectId: string;
  stage: StageName;
  refreshTrigger: number;
}

export function ArtifactPreview({ projectId, stage, refreshTrigger }: Props) {
  const [content, setContent] = useState('');
  const [exists, setExists] = useState(false);

  useEffect(() => {
    getArtifact(projectId, stage)
      .then(res => {
        setContent(res.content);
        setExists(res.exists);
      })
      .catch(console.error);
  }, [projectId, stage, refreshTrigger]);

  if (!exists) {
    return (
      <div className="artifact-preview empty">
        <p>No artifact yet. Chat with the agent to generate one.</p>
      </div>
    );
  }

  return (
    <div className="artifact-preview">
      <h4>{STAGE_LABELS[stage] ?? stage} Artifact</h4>
      <div className="artifact-content">
        <ReactMarkdown>{content}</ReactMarkdown>
      </div>
    </div>
  );
}
