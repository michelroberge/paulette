import { useState, useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Project, StageName, VersionBump } from '../../types';
import { regenerateSummary, getSummary, watchSummary } from '../../api/pipeline';

const ARTIFACT_STAGES: { name: StageName; label: string }[] = [
  { name: 'vision', label: 'Vision' },
  { name: 'ux', label: 'UX Design' },
  { name: 'architecture', label: 'Architecture' },
  { name: 'build', label: 'Build Plan' },
];

interface Props {
  project: Project;
  onNewProject: () => void;
  onViewStage: (stage: StageName) => void;
  onEnhance: (vision: string, bump: VersionBump) => void;
}

export function CompletionView({ project, onNewProject, onViewStage, onEnhance }: Props) {
  const [showEnhanceForm, setShowEnhanceForm] = useState(false);
  const [enhanceVision, setEnhanceVision] = useState('');
  const [versionBump, setVersionBump] = useState<VersionBump>('minor');
  const [submitting, setSubmitting] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [summaryContent, setSummaryContent] = useState('');
  const [summaryError, setSummaryError] = useState('');
  const summaryAbortRef = useRef<AbortController | null>(null);

  // If summary already ready, fetch it; otherwise stream live chunks
  useEffect(() => {
    if (project.summaryReady) {
      getSummary(project.id).then(res => {
        if (res.exists) setSummaryContent(res.content);
      }).catch(console.error);
      return;
    }

    // Connect to live stream
    setSummaryContent('');
    setSummaryError('');
    const controller = new AbortController();
    summaryAbortRef.current = controller;

    watchSummary(project.id, (event) => {
      if (event.type === 'chunk') {
        setSummaryContent(prev => prev + event.content);
      } else if (event.type === 'done' && event.content) {
        setSummaryContent(event.content);
      } else if (event.type === 'error') {
        setSummaryError(event.content || 'Summary generation failed');
      }
    }, controller.signal).catch(() => { /* stream ended or aborted */ });

    return () => {
      controller.abort();
      summaryAbortRef.current = null;
    };
  }, [project.id, project.summaryReady]);

  const handleRetry = async () => {
    setRetrying(true);
    setSummaryError('');
    setSummaryContent('');
    try {
      await regenerateSummary(project.id);
    } finally {
      setRetrying(false);
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

  return (
    <div className="completion-view">
      <div className="completion-layout">
        <div className="completion-left">
          <div className="completion-header">
            <div className="completion-icon">{project.summaryReady ? '✓' : '⏳'}</div>
            <h2>{project.summaryReady ? 'Pipeline Complete' : 'Almost done!'}</h2>
            <p className="completion-subtitle">
              {project.summaryReady
                ? <>All stages have been approved for <strong>{project.name}</strong> v{project.version}
                    {project.iteration > 1 && <span> (iteration {project.iteration})</span>}.</>
                : <>Generating iteration summary for <strong>{project.name}</strong> v{project.version}…</>
              }
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

          {!project.summaryReady && (
            <div className="summary-generating">
              {summaryError ? (
                <>
                  <span className="summary-error-icon">✗</span>
                  <span className="summary-error-text">{summaryError}</span>
                </>
              ) : (
                <>
                  <span className="summary-spinner" />
                  Generating iteration summary…
                </>
              )}
              <button
                className="summary-retry-btn"
                onClick={handleRetry}
                disabled={retrying}
              >
                {retrying ? 'Retrying…' : 'Retry'}
              </button>
            </div>
          )}

          {!showEnhanceForm ? (
            <div className="completion-actions">
              <button
                className="approve-button"
                onClick={() => setShowEnhanceForm(true)}
                disabled={!project.summaryReady}
                title={!project.summaryReady ? 'Waiting for summary to finish generating…' : undefined}
              >
                Enhance
              </button>
              {(() => {
                const match = summaryContent.match(/## Suggested Enhancements\n([\s\S]*?)(?=\n## |$)/);
                const suggestions = match?.[1]?.trim() ?? '';
                return suggestions ? (
                  <button
                    className="approve-button"
                    onClick={() => { setShowEnhanceForm(true); setEnhanceVision(suggestions); }}
                    disabled={!project.summaryReady}
                  >
                    Quick Enhance
                  </button>
                ) : null;
              })()}
              <button className="approve-button secondary" onClick={onNewProject}>
                Start New Project
              </button>
            </div>
          ) : (
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
          )}
        </div>

        <div className="completion-summary-panel">
          <div className="completion-summary-header">
            <h3>Iteration Summary</h3>
          </div>
          <div className="completion-summary-body">
            {project.summaryReady && summaryContent ? (
              <div className="artifact-content">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{summaryContent}</ReactMarkdown>
              </div>
            ) : (
              <div className="summary-generating-panel">
                <span className="summary-spinner" />
                <span>Generating summary…</span>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
