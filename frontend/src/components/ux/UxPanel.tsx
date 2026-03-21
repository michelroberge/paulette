import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useMock } from '../../hooks/useMock';
import { getArtifact } from '../../api/artifacts';
import { FrameworkSelector } from './FrameworkSelector';

interface Props {
  projectId: string;
  refreshTrigger: number;
  mode: 'artifact' | 'mock';
  onRequestMockTab?: () => void;
  hidden?: boolean;
}

export function UxPanel({ projectId, refreshTrigger, mode, onRequestMockTab, hidden }: Props) {
  const [artifactContent, setArtifactContent] = useState('');
  const [artifactExists, setArtifactExists] = useState(false);
  const [refinement, setRefinement] = useState('');

  const iframeRef = useRef<HTMLIFrameElement>(null);
  const { html, generating, tokenCount, loaded, load, generate, stop } = useMock(projectId);

  // Load artifact
  useEffect(() => {
    getArtifact(projectId, 'ux')
      .then(res => {
        setArtifactContent(res.content);
        setArtifactExists(res.exists);
      })
      .catch(console.error);
  }, [projectId, refreshTrigger]);

  // Load existing mock on mount
  useEffect(() => {
    load();
  }, [load]);

  const handleIframeLoad = () => {
    const iframe = iframeRef.current;
    if (!iframe) return;
    const height = iframe.contentDocument?.body?.scrollHeight;
    if (height) iframe.style.height = `${height}px`;
  };

  const handleGenerate = () => {
    onRequestMockTab?.();
    generate(refinement);
    setRefinement('');
  };

  return (
    <div className="ux-panel" style={{ display: hidden ? 'none' : 'flex' }}>
      {mode === 'mock' && (
        <div className="ux-tab-actions" style={{ padding: '0.4rem 1rem', borderBottom: '1px solid #334155', background: '#1e293b' }}>
          <FrameworkSelector projectId={projectId} />
          <input
            className="refinement-input"
            placeholder="Refinement instruction (optional)..."
            value={refinement}
            onChange={e => setRefinement(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') handleGenerate(); }}
            disabled={generating || !artifactExists}
          />
          {generating ? (
            <button className="stop-button" onClick={stop}>
              Stop
            </button>
          ) : (
            <button
              className="generate-mock-button"
              onClick={handleGenerate}
              disabled={!artifactExists}
            >
              {html ? 'Regenerate Mock' : 'Generate Mock Preview'}
            </button>
          )}
        </div>
      )}

      {mode === 'artifact' && (
        <div className="ux-tab-actions" style={{ padding: '0.4rem 1rem', borderBottom: '1px solid #334155', background: '#1e293b', justifyContent: 'flex-end' }}>
          <FrameworkSelector projectId={projectId} />
          {generating ? (
            <button className="stop-button" onClick={stop}>
              Stop
            </button>
          ) : (
            <button
              className="generate-mock-button"
              onClick={handleGenerate}
              disabled={!artifactExists}
            >
              {html ? 'Regenerate Mock' : 'Generate Mock Preview'}
            </button>
          )}
        </div>
      )}

      <div className="ux-panel-content">
        {mode === 'artifact' && (
          artifactExists ? (
            <div className="artifact-content">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{artifactContent}</ReactMarkdown>
            </div>
          ) : (
            <div className="artifact-preview empty">
              <p>No UX artifact yet. Chat with the agent to generate one.</p>
            </div>
          )
        )}

        {mode === 'mock' && (
          <>
            {generating && (
              <div className="mock-generating">
                <div className="mock-stream-header">
                  <span className="mock-stream-dot" />
                  Generating wireframes...
                  {tokenCount > 0 && (
                    <span className="mock-stream-tokens">{tokenCount.toLocaleString()} tokens</span>
                  )}
                </div>
              </div>
            )}

            {!generating && !html && loaded && (
              <div className="artifact-preview empty">
                <p>No mock preview yet.</p>
                <p>Click <strong>Generate Mock Preview</strong> to create wireframes from the UX artifact.</p>
              </div>
            )}

            {!generating && html && (
              <iframe
                ref={iframeRef}
                srcDoc={html}
                className="mock-iframe"
                sandbox="allow-scripts allow-same-origin"
                title="UI Mock Preview"
                onLoad={handleIframeLoad}
              />
            )}
          </>
        )}
      </div>
    </div>
  );
}
