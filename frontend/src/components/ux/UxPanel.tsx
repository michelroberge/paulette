import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useMock } from '../../hooks/useMock';
import { getArtifact } from '../../api/artifacts';

interface Props {
  projectId: string;
  refreshTrigger: number;
}

export function UxPanel({ projectId, refreshTrigger }: Props) {
  const [tab, setTab] = useState<'artifact' | 'mock'>('artifact');
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
    setTab('mock');
    generate(refinement);
    setRefinement('');
  };

  return (
    <div className="ux-panel">
      <div className="ux-panel-tabs">
        <button
          className={`ux-tab${tab === 'artifact' ? ' active' : ''}`}
          onClick={() => setTab('artifact')}
        >
          UX Design Artifact
        </button>
        <button
          className={`ux-tab${tab === 'mock' ? ' active' : ''}`}
          onClick={() => setTab('mock')}
        >
          Mock Preview {html ? '●' : ''}
        </button>

        <div className="ux-tab-actions">
          {tab === 'mock' && (
            <input
              className="refinement-input"
              placeholder="Refinement instruction (optional)..."
              value={refinement}
              onChange={e => setRefinement(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') handleGenerate(); }}
              disabled={generating || !artifactExists}
            />
          )}
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
      </div>

      <div className="ux-panel-content">
        {tab === 'artifact' && (
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

        {tab === 'mock' && (
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
