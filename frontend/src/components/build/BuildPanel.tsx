import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { getArtifact } from '../../api/artifacts';
import { useBeads } from '../../hooks/useBeads';
import { BeadGraph } from './BeadGraph';

interface Props {
  projectId: string;
  refreshTrigger: number;
  mode: 'artifact' | 'execute';
  onRequestExecuteTab?: () => void;
  hidden?: boolean;
  onBeadTokens?: (n: number) => void;
}

export function BuildPanel({ projectId, refreshTrigger, mode, onRequestExecuteTab, hidden, onBeadTokens }: Props) {
  const [artifactContent, setArtifactContent] = useState('');
  const [artifactExists, setArtifactExists] = useState(false);
  const [maxParallel, setMaxParallel] = useState(2);

  const { graph, phase, executionLog, streamingText, planLimitReached, loadGraph, generate, execute, stop, clearLog, clearPlanLimit } = useBeads(projectId);

  const logRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<HTMLDivElement>(null);

  // Load artifact
  useEffect(() => {
    getArtifact(projectId, 'build')
      .then(res => {
        setArtifactContent(res.content);
        setArtifactExists(res.exists);
      })
      .catch(console.error);
  }, [projectId, refreshTrigger]);

  // Load existing graph on mount
  useEffect(() => {
    loadGraph();
  }, [loadGraph]);

  // Auto-scroll log
  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [executionLog]);

  // Auto-scroll streaming text
  useEffect(() => {
    if (streamRef.current) {
      streamRef.current.scrollTop = streamRef.current.scrollHeight;
    }
  }, [streamingText]);

  const handleGenerate = () => {
    onRequestExecuteTab?.();
    generate();
  };

  const handleBuild = () => {
    execute(maxParallel);
  };

  const hasBeads = graph.beads.length > 0;
  const isGenerating = phase === 'generating';
  const isExecuting = phase === 'executing';
  const isDone = phase === 'done';

  const taskBeads = graph.beads.filter(b => b.type !== 'epic');
  const activeCount = graph.beads.filter(b => b.status === 'in_progress').length;
  const completedCount = taskBeads.filter(b => b.status === 'closed').length;
  const totalCount = taskBeads.length;

  // Report bead token deltas to parent as they accumulate
  const totalBeadTokens = graph.beads.reduce((s, b) => s + (b.tokens ?? 0), 0);
  const prevBeadTokensRef = useRef(0);
  useEffect(() => {
    const delta = totalBeadTokens - prevBeadTokensRef.current;
    if (delta > 0) {
      onBeadTokens?.(delta);
      prevBeadTokensRef.current = totalBeadTokens;
    }
  }, [totalBeadTokens]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="build-panel" style={{ display: hidden ? 'none' : 'flex' }}>
      {mode === 'execute' && (
        <div className="build-tab-actions" style={{ padding: '0.4rem 1rem', borderBottom: '1px solid #334155', background: '#1e293b', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
          {(isGenerating || isExecuting) && (
            <>
              <span className="execute-status-dot" />
              <span className="execute-status-label">
                {isGenerating ? `Generating… ${graph.beads.length > 0 ? `(${graph.beads.length} beads)` : ''}` : `Executing… ${activeCount > 0 ? `${activeCount} agent${activeCount !== 1 ? 's' : ''} active` : ''}`}
              </span>
              <button className="stop-button" style={{ marginLeft: 'auto' }} onClick={stop}>
                Stop
              </button>
            </>
          )}
          {!hasBeads && !isGenerating && (
            <button
              className="generate-mock-button"
              onClick={handleGenerate}
              disabled={!artifactExists}
            >
              Generate Beads
            </button>
          )}
        </div>
      )}

      <div className="build-panel-content">
        {mode === 'artifact' && (
          artifactExists ? (
            <div className="artifact-content">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{artifactContent}</ReactMarkdown>
            </div>
          ) : (
            <div className="artifact-preview empty">
              <p>No build artifact yet. Chat with the agent to generate one.</p>
            </div>
          )
        )}

        {mode === 'execute' && (
          <div className="execute-view">
            {planLimitReached && (
              <div className="plan-limit-banner">
                <span className="plan-limit-icon">⚠️</span>
                <span className="plan-limit-message">
                  Claude plan limit reached — execution stopped. Resume by clicking Build again.
                </span>
                <button className="plan-limit-dismiss" onClick={clearPlanLimit} title="Dismiss">✕</button>
              </div>
            )}

            {!hasBeads && !isGenerating && (
              <div className="artifact-preview empty">
                <p>No beads generated yet.</p>
                <p>Click <strong>Generate Beads</strong> to parse the build plan into trackable tasks.</p>
              </div>
            )}

            {isGenerating && (
              <div className="mock-generating">
                <div className="mock-stream-header">
                  <span className="mock-stream-dot" />
                  Generating beads... ({graph.beads.length} created)
                </div>
                {streamingText && (
                  <div className="mock-stream-text" ref={streamRef}>
                    {streamingText}
                  </div>
                )}
              </div>
            )}

            {hasBeads && (
              <div className="execute-main">
                <div className="execute-graph">
                  <BeadGraph graph={graph} />
                </div>

                <div className="execute-details">
                  {isExecuting && (
                    <div className="execute-status-sidebar">
                      <div className="execute-status-row">
                        <span className="execute-status-dot" />
                        <span className="execute-status-label">Executing</span>
                        {activeCount > 0 && (
                          <span className="execute-agent-badge">{activeCount} agent{activeCount !== 1 ? 's' : ''}</span>
                        )}
                      </div>
                      {totalCount > 0 && (
                        <div className="execute-progress-row">
                          <div className="execute-progress-track">
                            <div
                              className="execute-progress-fill"
                              style={{ width: `${(completedCount / totalCount) * 100}%` }}
                            />
                          </div>
                          <span className="execute-progress-label">{completedCount} / {totalCount}</span>
                        </div>
                      )}
                      {streamingText && (
                        <div className="mock-stream-text" ref={streamRef} style={{ flex: 'none', maxHeight: '200px' }}>
                          {streamingText}
                        </div>
                      )}
                    </div>
                  )}

                  {!isExecuting && !isDone && (
                    <div className="bead-config-bar">
                      <label className="bead-config-label">
                        Max parallel agents:
                        <input
                          className="bead-config-input"
                          type="number"
                          min={1}
                          max={10}
                          value={maxParallel}
                          onChange={e => setMaxParallel(Math.max(1, Math.min(10, parseInt(e.target.value) || 1)))}
                        />
                      </label>
                      <button
                        className="generate-mock-button"
                        onClick={handleBuild}
                        disabled={isGenerating}
                      >
                        Build
                      </button>
                    </div>
                  )}

                  {isDone && (
                    <div className="bead-config-bar">
                      <span className="bead-done-badge">All beads complete</span>
                    </div>
                  )}

                  {executionLog.length > 0 && (
                    <div className="execute-log" ref={logRef}>
                      <button className="log-clear-button" onClick={clearLog} title="Clear log">✕</button>
                      {executionLog.map((line, i) => (
                        <div
                          key={i}
                          className={`execute-log-entry${line.startsWith('ERROR') ? ' log-error' : line.includes('Done:') ? ' log-done' : line.includes('Starting:') ? ' log-claimed' : ''}`}
                        >
                          {line}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
