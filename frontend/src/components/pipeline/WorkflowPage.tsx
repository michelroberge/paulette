import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getProject } from '../../api/projects';
import { WorkflowGraph } from './WorkflowGraph';
import type { Project } from '../../types';

function resolveCurrentNodeId(project: Project): string {
  switch (project.currentStage) {
    case 'vision':
      return project.useRefinementLoop ? 'ref-summarize' : 'vision-chat';
    case 'ux':
      return 'ux-chat';
    case 'architecture':
      return 'arch-chat';
    case 'build':
      return 'build-chat';
    case 'complete':
      return project.summaryApproved ? 'done' : 'summary';
    default:
      return 'begin';
  }
}

export function WorkflowPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [project, setProject] = useState<Project | null>(null);

  useEffect(() => {
    if (id) getProject(id).then(setProject).catch(() => navigate('/'));
  }, [id, navigate]);

  if (!project) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
        <span style={{ color: '#64748b' }}>Loading...</span>
      </div>
    );
  }

  const aiMode = project.aiMode || 'files';

  return (
    <div className="workflow-page">
      <header className="workflow-page-header">
        <button className="back-button" onClick={() => navigate(`/projects/${project.id}`)}>&larr;</button>
        <h2>{project.name} — Workflow</h2>
        <span className="workflow-page-mode">Mode: {aiMode}</span>
      </header>
      <WorkflowGraph
        target={{ type: 'project', projectId: project.id, aiMode }}
        useRefinementLoop={project.useRefinementLoop}
        currentNodeId={resolveCurrentNodeId(project)}
      />
    </div>
  );
}
