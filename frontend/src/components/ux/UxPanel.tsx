import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
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

  const { html, generating, streamingText, loaded, load, generate } = useMock(projectId);

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

  // Render HTML into iframe via srcdoc
  useEffect(() => {
    if (iframeRef.current && html) {
      iframeRef.current.srcdoc = html;
    }
  }, [html]);

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
          <button
            className="generate-mock-button"
            onClick={handleGenerate}
            disabled={generating}
          >
            {generating ? 'Generating...' : html ? 'Regenerate Mock' : 'Generate Mock Preview'}
          </button>
        </div>
      </div>

      <div className="ux-panel-content">
        {tab === 'artifact' && (
          artifactExists ? (
            <div className="artifact-content">
              <ReactMarkdown>{artifactContent}</ReactMarkdown>
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
                </div>
                <pre className="mock-stream-text">{streamingText}</pre>
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
                className="mock-iframe"
                sandbox="allow-scripts"
                title="UI Mock Preview"
              />
            )}
          </>
        )}
      </div>
    </div>
  );
}
