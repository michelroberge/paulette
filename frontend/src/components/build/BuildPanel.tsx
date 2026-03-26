import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { getArtifact } from '../../api/artifacts';
import { useBeads } from '../../hooks/useBeads';
import { BeadGraph } from './BeadGraph';
import { BeadDetailPanel } from './BeadDetailPanel';
import { BuildingAnimation } from '../ux/BuildingAnimation';

interface Props {
  projectId: string;
  refreshTrigger: number;
  mode: 'artifact' | 'execute';
  onRequestExecuteTab?: () => void;
  hidden?: boolean;
  onBeadTokens?: (n: number) => void;
  onExecutionComplete?: () => void;
  onBuildDone?: () => void;
  onHasBeads?: (v: boolean) => void;
  agentActive?: boolean;
  agentOperation?: string;
  agentStreamingText?: string;
}

export function BuildPanel({ projectId, refreshTrigger, mode, onRequestExecuteTab, hidden, onBeadTokens, onExecutionComplete, onBuildDone, onHasBeads, agentActive, agentOperation, agentStreamingText }: Props) {
  const [artifactContent, setArtifactContent] = useState('');
  const [artifactExists, setArtifactExists] = useState(false);
  const [maxParallel, setMaxParallel] = useState(2);
  const [selectedBeadId, setSelectedBeadId] = useState<string | null>(null);

  const { graph, setGraph, phase, loading, executionLog, streamingText, generateTokens, planLimitReached, loadGraph, generate, execute, stop, clearLog, clearPlanLimit } = useBeads(projectId);

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

  const hasBeads = graph.beads.length > 0;
  const isGenerating = phase === 'generating';
  const isExecuting = phase === 'executing';
  const isDone = phase === 'done';

  const taskBeads = graph.beads.filter(b => b.type !== 'epic');
  const activeCount = graph.beads.filter(b => b.status === 'in_progress').length;
  const completedCount = taskBeads.filter(b => b.status === 'closed').length;
  const totalCount = taskBeads.length;

  // Notify parent of bead existence
  useEffect(() => {
    onHasBeads?.(hasBeads);
  }, [hasBeads]); // eslint-disable-line react-hooks/exhaustive-deps

  // Notify parent when execution completes
  const prevPhaseRef = useRef(phase);
  useEffect(() => {
    if (phase === 'done') {
      onBuildDone?.();
      if (prevPhaseRef.current !== 'done') {
        onExecutionComplete?.();
      }
    }
    prevPhaseRef.current = phase;
  }, [phase]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleGenerate = () => {
    onRequestExecuteTab?.();
    generate();
  };

  const handleBuild = () => {
    execute(maxParallel);
  };

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

  const prevGenerateTokensRef = useRef(0);
  useEffect(() => {
    const delta = generateTokens - prevGenerateTokensRef.current;
    if (delta > 0) {
      onBeadTokens?.(delta);
      prevGenerateTokensRef.current = generateTokens;
    }
  }, [generateTokens]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="build-panel" style={{ display: hidden ? 'none' : 'flex' }}>
      {mode === 'execute' && (
        <div className="build-tab-actions" style={{ padding: '0.4rem 1rem', borderBottom: '1px solid #334155', background: '#1e293b', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
          {isGenerating && (
            <>
              <span className="execute-status-dot" />
              <span className="execute-status-label">
                {`Generating… ${graph.beads.length > 0 ? `(${graph.beads.length} beads)` : ''}`}
              </span>
              <button className="stop-button" style={{ marginLeft: 'auto' }} onClick={stop}>
                Stop
              </button>
            </>
          )}
          {!hasBeads && !isGenerating && !loading && (
            <button
              className="generate-mock-button"
              onClick={handleGenerate}
              disabled={!artifactExists}
            >
              Generate Beads
            </button>
          )}
          {!hasBeads && !isGenerating && loading && (
            <BuildingAnimation />
          )}
        </div>
      )}

      <div className="build-panel-content">
        {mode === 'artifact' && agentActive && agentOperation === 'chat' ? (
          <div className="mock-split-layout">
            <div className="mock-main-column">
              <BuildingAnimation />
            </div>
            <div className="mock-activity-panel">
              <div className="mock-activity-header">
                <span className="mock-stream-dot" />
                <span>Refining build plan…</span>
              </div>
              <div className="mock-activity-content">
                {agentStreamingText || 'Starting…'}
              </div>
            </div>
          </div>
        ) : mode === 'artifact' && (
          artifactExists ? (
            <div className="artifact-content">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{artifactContent}</ReactMarkdown>
              {!hasBeads && !isGenerating && (
                <div style={{ padding: '1rem', borderTop: '1px solid #334155', display: 'flex', justifyContent: 'center' }}>
                  <button
                    className="generate-mock-button"
                    onClick={handleGenerate}
                  >
                    Generate Beads
                  </button>
                </div>
              )}
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

            <div className="execute-main">
              <div className="execute-graph">
                {hasBeads ? (
                  <BeadGraph graph={graph} onBeadClick={setSelectedBeadId} />
                ) : (isGenerating || loading) ? (
                  <div className="execute-paulette">
                    <BuildingAnimation />
                  </div>
                ) : null}
              </div>

              <div className="execute-details">
                {isGenerating && streamingText && (
                  <div className="mock-stream-text" ref={streamRef}>
                    {streamingText}
                  </div>
                )}

                {isExecuting && (
                  <div className="execute-status-sidebar">
                    <div className="execute-status-row">
                      <span className="execute-status-dot" />
                      <span className="execute-status-label">Executing</span>
                      {activeCount > 0 && (
                        <span className="execute-agent-badge">{activeCount} agent{activeCount !== 1 ? 's' : ''}</span>
                      )}
                      <button className="stop-button" style={{ marginLeft: 'auto' }} onClick={stop}>
                        Stop
                      </button>
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

                {!isGenerating && !isExecuting && !hasBeads && !loading && (
                  <div className="execute-details-empty">
                    <p>No beads generated yet.</p>
                    <p>Click <strong>Generate Beads</strong> above to parse the build plan into trackable tasks.</p>
                  </div>
                )}

                {!isGenerating && !isExecuting && !isDone && hasBeads && (
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
          </div>
        )}
      </div>

      {selectedBeadId && (() => {
        const selectedBead = graph.beads.find(b => b.id === selectedBeadId);
        return selectedBead ? (
          <BeadDetailPanel
            projectId={projectId}
            beadId={selectedBeadId}
            bead={selectedBead}
            allBeads={graph.beads}
            onBeadSelect={setSelectedBeadId}
            onClose={() => setSelectedBeadId(null)}
            onBeadsUpdated={setGraph}
            onBeadControlled={loadGraph}
          />
        ) : null;
      })()}
    </div>
  );
}
