import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getConnection } from '../../api/connections';
import { initConnectionPrompts } from '../../api/connectionPrompts';
import { WorkflowGraph } from './WorkflowGraph';
import type { Connection } from '../../types/provider';

export function ConnectionWorkflowPage() {
  const { connId } = useParams<{ connId: string }>();
  const navigate = useNavigate();
  const [connection, setConnection] = useState<Connection | null>(null);
  const [initing, setIniting] = useState(false);

  useEffect(() => {
    if (connId) getConnection(connId).then(setConnection).catch(() => navigate('/configure?tab=connections'));
  }, [connId, navigate]);

  // Auto-init prompts directory on first visit.
  useEffect(() => {
    if (!connId) return;
    setIniting(true);
    initConnectionPrompts(connId).finally(() => setIniting(false));
  }, [connId]);

  if (!connection || initing) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
        <span style={{ color: '#64748b' }}>{initing ? 'Initializing prompts...' : 'Loading...'}</span>
      </div>
    );
  }

  return (
    <div className="workflow-page">
      <header className="workflow-page-header">
        <button className="back-button" onClick={() => navigate('/configure?tab=connections')}>&larr;</button>
        <h2>{connection.name} — Prompt Templates</h2>
        <span className="workflow-page-mode">{connection.providerType}</span>
      </header>
      <WorkflowGraph
        target={{ type: 'connection', connectionId: connection.id }}
      />
    </div>
  );
}
