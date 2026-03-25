import { useState, useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Project, StageName, VersionBump, StageActivity, SessionSummary } from '../../types';
import { regenerateSummary, getSummary, watchSummary, approveSummary } from '../../api/pipeline';
import { getSessions } from '../../api/sessions';
import { BuildingAnimation } from '../ux/BuildingAnimation';

const ARTIFACT_STAGES: { name: StageName; label: string }[] = [
  { name: 'vision', label: 'Vision' },
  { name: 'ux', label: 'UX Design' },
  { name: 'architecture', label: 'Architecture' },
  { name: 'build', label: 'Build Plan' },
];

interface Props {
  project: Project;
  activity?: StageActivity;
  onNewProject: () => void;
  onViewStage: (stage: StageName) => void;
  onEnhance: (vision: string, bump: VersionBump) => void;
  onSummaryReady?: () => void;
}

export function CompletionView({ project, activity, onNewProject, onViewStage, onEnhance, onSummaryReady }: Props) {
  const [showEnhanceForm, setShowEnhanceForm] = useState(false);
  const [enhanceVision, setEnhanceVision] = useState('');
  const [versionBump, setVersionBump] = useState<VersionBump>('minor');
  const [submitting, setSubmitting] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [approving, setApproving] = useState(false);
  const [summaryApproved, setSummaryApproved] = useState(project.summaryApproved ?? false);
  const [summaryContent, setSummaryContent] = useState('');
  const [summaryError, setSummaryError] = useState('');
  const [tokenCount, setTokenCount] = useState(0);
  const [sessionData, setSessionData] = useState<SessionSummary | null>(null);
  const [showTokens, setShowTokens] = useState(false);
  const summaryAbortRef = useRef<AbortController | null>(null);
  const streamRef = useRef<HTMLDivElement>(null);

  const isGenerating = generating || !!activity;

  // If summary already ready, fetch it
  useEffect(() => {
    if (project.summaryReady) {
      getSummary(project.id).then(res => {
        if (res.exists) setSummaryContent(res.content);
      }).catch(console.error);
    }
  }, [project.id, project.summaryReady]);

  // Fetch session token data when summary is ready
  useEffect(() => {
    if (!project.summaryReady) return;
    getSessions(project.id).then(setSessionData).catch(console.error);
  }, [project.id, project.summaryReady]);

  // Connect to live stream only while generating
  useEffect(() => {
    if (project.summaryReady || !isGenerating) return;

    setSummaryContent('');
    setSummaryError('');
    const controller = new AbortController();
    summaryAbortRef.current = controller;

    watchSummary(project.id, (event) => {
      if (event.type === 'chunk') {
        setSummaryContent(prev => prev + event.content);
        if (streamRef.current) {
          streamRef.current.scrollTop = streamRef.current.scrollHeight;
        }
      } else if (event.type === 'tokens') {
        setTokenCount(Number.parseInt(event.content, 10));
      } else if (event.type === 'done') {
        if (event.content) setSummaryContent(event.content);
        setGenerating(false);
        onSummaryReady?.();
      } else if (event.type === 'error') {
        setSummaryError(event.content || 'Summary generation failed');
        setGenerating(false);
      }
    }, controller.signal).catch(() => { setGenerating(false); });

    return () => {
      controller.abort();
      summaryAbortRef.current = null;
    };
  }, [project.id, project.summaryReady, isGenerating]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleGenerate = async () => {
    setGenerating(true);
    setSummaryError('');
    setSummaryContent('');
    try {
      await regenerateSummary(project.id);
    } catch {
      setGenerating(false);
    }
  };

  const handleApproveSummary = async () => {
    setApproving(true);
    try {
      await approveSummary(project.id);
      setSummaryApproved(true);
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to save summary to docs');
    } finally {
      setApproving(false);
    }
  };

  const handleEnhance = async () => {
    if (!enhanceVision.trim()) return;
    setSubmitting(true);
    try {
      onEnhance(enhanceVision.trim(), versionBump);
    } finally {
      setSubmitting(false);
    }
  };

  if (!project.summaryReady && !isGenerating) {
    return (
      <div className="completion-view">
        <div className="completion-layout">
          <div className="completion-left">
            <div className="completion-header">
              <div className="completion-icon">✓</div>
              <h2>Almost There</h2>
              <p className="completion-subtitle">
                All stages approved for <strong>{project.name}</strong> v{project.version}.
                Generate a summary of this iteration to complete the pipeline.
              </p>
            </div>
            {summaryError && (
              <div className="summary-error-panel">
                <span className="summary-error-icon">✗</span>
                <span className="summary-error-text">{summaryError}</span>
              </div>
            )}
            <div className="completion-actions">
              <button className="approve-button" onClick={handleGenerate}>
                Generate Summary
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (!project.summaryReady && isGenerating) {
    return (
      <div className="completion-view">
        <div className="mock-split-layout">
          <div className="mock-main-column">
            <BuildingAnimation />
          </div>
          <div className="mock-activity-panel">
            <div className="mock-activity-header">
              <span className="mock-stream-dot" />
              <span>Generating Summary…</span>
              {tokenCount > 0 && (
                <span className="mock-stream-tokens">{tokenCount.toLocaleString()} tokens</span>
              )}
            </div>
            <div className="mock-activity-content" ref={streamRef}>
              {summaryContent}
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="completion-view">
      <div className="completion-layout">
        <div className="completion-left">
          <div className="completion-header">
            <div className="completion-icon">✓</div>
            <h2>Pipeline Complete</h2>
            <p className="completion-subtitle">
              All stages have been approved for <strong>{project.name}</strong> v{project.version}
              {project.iteration > 1 && <span> (iteration {project.iteration})</span>}.
            </p>
          </div>

          <div className="completion-artifacts">
            <h3>Approved Artifacts</h3>
            <ul>
              {ARTIFACT_STAGES.map(({ name, label }) => (
                <li key={name} className="completion-artifact-item">
                  <span className="artifact-check">✓</span>
                  <button className="artifact-link" onClick={() => onViewStage(name)}>
                    {label}
                  </button>
                </li>
              ))}
            </ul>
          </div>

          {sessionData && (
            <div className="completion-tokens">
              <button
                className="tokens-toggle"
                onClick={() => setShowTokens(v => !v)}
              >
                {showTokens ? '▾' : '▸'} Token Usage — {sessionData.grandTotal.toLocaleString()} total
              </button>
              {showTokens && (
                <table className="tokens-table">
                  <thead>
                    <tr>
                      <th>Stage</th>
                      <th>Sessions</th>
                      <th>Tokens</th>
                      <th>%</th>
                    </tr>
                  </thead>
                  <tbody>
                    {ARTIFACT_STAGES.map(({ name, label }) => {
                      const s = sessionData.byStage[name];
                      if (!s) return null;
                      const pct = sessionData.grandTotal > 0
                        ? ((s.tokens / sessionData.grandTotal) * 100).toFixed(1)
                        : '0';
                      return (
                        <tr key={name}>
                          <td>{label}</td>
                          <td>{s.count}</td>
                          <td>{s.tokens.toLocaleString()}</td>
                          <td>{pct}%</td>
                        </tr>
                      );
                    })}
                    {sessionData.byStage.complete && (
                      <tr>
                        <td>Summary</td>
                        <td>{sessionData.byStage.complete.count}</td>
                        <td>{sessionData.byStage.complete.tokens.toLocaleString()}</td>
                        <td>
                          {sessionData.grandTotal > 0
                            ? ((sessionData.byStage.complete.tokens / sessionData.grandTotal) * 100).toFixed(1)
                            : '0'}%
                        </td>
                      </tr>
                    )}
                  </tbody>
                  <tfoot>
                    <tr>
                      <td><strong>Total</strong></td>
                      <td><strong>{sessionData.sessions.length}</strong></td>
                      <td><strong>{sessionData.grandTotal.toLocaleString()}</strong></td>
                      <td><strong>100%</strong></td>
                    </tr>
                  </tfoot>
                </table>
              )}
            </div>
          )}

          {showEnhanceForm ? (
            <div className="enhance-form">
              <h3>Enhance {project.name}</h3>
              <p className="enhance-description">
                Describe how you want to improve your app. The previous iteration's artifacts
                will be used as a starting point for each stage.
              </p>
              <textarea
                className="enhance-vision-input"
                placeholder="I want to improve my app like this: ..."
                value={enhanceVision}
                onChange={e => setEnhanceVision(e.target.value)}
                rows={4}
              />
              <div className="version-bump-selector">
                <label>Version bump (current: v{project.version}):</label>
                <div className="version-bump-options">
                  {(['patch', 'minor', 'major'] as VersionBump[]).map(bump => (
                    <label key={bump} className="version-bump-option">
                      <input
                        type="radio"
                        name="versionBump"
                        value={bump}
                        checked={versionBump === bump}
                        onChange={() => setVersionBump(bump)}
                      />
                      {bump.charAt(0).toUpperCase() + bump.slice(1)}
                    </label>
                  ))}
                </div>
              </div>
              <div className="enhance-form-actions">
                <button
                  className="approve-button"
                  onClick={handleEnhance}
                  disabled={!enhanceVision.trim() || submitting}
                >
                  {submitting ? 'Starting...' : 'Start Enhancement'}
                </button>
                <button
                  className="approve-button secondary"
                  onClick={() => { setShowEnhanceForm(false); setEnhanceVision(''); }}
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <div className="completion-actions">
              {!summaryApproved && (
                <button
                  className="approve-button"
                  onClick={handleApproveSummary}
                  disabled={approving}
                >
                  {approving ? 'Saving…' : 'Save to Docs'}
                </button>
              )}
              {summaryApproved && (
                <>
                  <button className="approve-button" onClick={() => setShowEnhanceForm(true)}>
                    Enhance
                  </button>
                  {(() => {
                    const match = /## Suggested Enhancements\n([\s\S]*?)(?=\n## |$)/.exec(summaryContent);
                    const suggestions = match?.[1]?.trim() ?? '';
                    return suggestions ? (
                      <button
                        className="approve-button"
                        onClick={() => { setShowEnhanceForm(true); setEnhanceVision(suggestions); }}
                      >
                        Quick Enhance
                      </button>
                    ) : null;
                  })()}
                  <button className="approve-button secondary" onClick={onNewProject}>
                    Start New Project
                  </button>
                </>
              )}
            </div>
          )}
        </div>

        <div className="completion-summary-panel">
          <div className="completion-summary-header">
            <h3>Iteration Summary</h3>
          </div>
          <div className="completion-summary-body">
            <div className="artifact-content">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{summaryContent}</ReactMarkdown>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
