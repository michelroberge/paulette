import { useEffect, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { getVersionSnapshot, getVersionBeadExecution } from '../../api/versions';
import { getConfig } from '../../api/config';
import { StageRobot } from './StageRobot';
import type { Project, StageName, VersionSnapshot, VersionBeadDetail, Bead } from '../../types';

const STAGE_LABELS: Record<StageName, string> = {
  vision: 'Vision',
  ux: 'UX Design',
  ui: 'UI Framework',
  architecture: 'Architecture',
  build: 'Build',
  complete: 'Complete',
};

const PIPELINE_STAGES: StageName[] = ['vision', 'ux', 'architecture', 'build', 'complete'];

interface Props {
  project: Project;
  version: string;
  onClose: () => void;
}

interface BeadDetailState {
  loading: boolean;
  data: VersionBeadDetail | null;
}

export function VersionHistoryView({ project, version, onClose }: Readonly<Props>) {
  const [snapshot, setSnapshot] = useState<VersionSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedStage, setSelectedStage] = useState<StageName>('vision');
  const [activeTab, setActiveTab] = useState('artifact');
  const [beadDetails, setBeadDetails] = useState<Record<string, BeadDetailState>>({});
  const [appVersion, setAppVersion] = useState('');
  const [appAuthor, setAppAuthor] = useState('');

  useEffect(() => {
    getConfig().then(c => {
      setAppVersion(c.version);
      setAppAuthor(c.author);
    }).catch(console.error);
  }, []);

  useEffect(() => {
    setLoading(true);
    getVersionSnapshot(project.id, version)
      .then(setSnapshot)
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [project.id, version]);

  useEffect(() => {
    setActiveTab('artifact');
  }, [selectedStage]);

  function loadBeadDetail(beadId: string) {
    if (beadDetails[beadId]) return;
    setBeadDetails(prev => ({ ...prev, [beadId]: { loading: true, data: null } }));
    getVersionBeadExecution(project.id, version, beadId)
      .then(data => setBeadDetails(prev => ({ ...prev, [beadId]: { loading: false, data } })))
      .catch(() => setBeadDetails(prev => ({ ...prev, [beadId]: { loading: false, data: null } })));
  }

  function getTabsForStage(stage: StageName): { id: string; label: string }[] {
    if (stage === 'ux') return [{ id: 'artifact', label: 'Artifact' }, { id: 'chat', label: 'Chat' }, { id: 'mock', label: 'Mock' }];
    if (stage === 'build') return [{ id: 'artifact', label: 'Artifact' }, { id: 'chat', label: 'Chat' }, { id: 'beads', label: 'Beads' }];
    if (stage === 'complete') return [{ id: 'summary', label: 'Summary' }];
    return [{ id: 'artifact', label: 'Artifact' }, { id: 'chat', label: 'Chat' }];
  }

  function renderTabContent() {
    if (!snapshot) return null;
    const stageData = snapshot.stages[selectedStage];

    if (selectedStage === 'complete') {
      return (
        <div className="artifact-preview">
          {snapshot.meta && (
            <div className="version-meta-box">
              <span className="version-meta-tag">{snapshot.meta.tagName}</span>
              <span className="version-meta-commit" title="Git commit hash">{snapshot.meta.commitHash.slice(0, 12)}</span>
              <span className="version-meta-date">{new Date(snapshot.meta.approvedAt).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })}</span>
            </div>
          )}
          {snapshot.summary ? (
            <div className="artifact-content">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{snapshot.summary}</ReactMarkdown>
            </div>
          ) : (
            <p className="history-empty">No summary available for this version.</p>
          )}
        </div>
      );
    }

    if (!stageData) {
      return <p className="history-empty">No data available for this stage.</p>;
    }

    if (activeTab === 'artifact') {
      return stageData.artifact ? (
        <div className="artifact-preview">
          <div className="artifact-content">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{stageData.artifact}</ReactMarkdown>
          </div>
        </div>
      ) : (
        <p className="history-empty">No artifact for this stage.</p>
      );
    }

    if (activeTab === 'chat') {
      return stageData.chatHistory && stageData.chatHistory.messages.length > 0 ? (
        <div className="chat-panel">
          <div className="messages">
            {stageData.chatHistory.messages.map((msg, i) => (
              <div key={i} className={`message ${msg.role}`}>
                <div className="message-role">{msg.role === 'user' ? 'You' : 'Agent'}</div>
                <div className="message-content">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <p className="history-empty">No chat history archived for this version.</p>
      );
    }

    if (activeTab === 'mock' && selectedStage === 'ux') {
      return stageData.mockHtml ? (
        <iframe
          className="mock-preview"
          srcDoc={stageData.mockHtml}
          sandbox="allow-scripts"
          title={`UX Mock v${version}`}
        />
      ) : (
        <p className="history-empty">No UX mock available for this version.</p>
      );
    }

    if (activeTab === 'beads' && selectedStage === 'build') {
      return (
        <div className="history-beads-list">
          {stageData.beadsGraph && stageData.beadsGraph.beads.length > 0 ? (
            stageData.beadsGraph.beads.map((bead: Bead) => {
              const detail = beadDetails[bead.id];
              const expanded = !!detail;
              return (
                <div key={bead.id} className="history-bead-card">
                  <button
                    className="history-bead-header"
                    onClick={() => {
                      if (!expanded) loadBeadDetail(bead.id);
                      else setBeadDetails(prev => { const next = { ...prev }; delete next[bead.id]; return next; });
                    }}
                  >
                    <span className={`bead-status-dot status-${bead.status}`} />
                    <span className="history-bead-title">{bead.title}</span>
                    <span className="history-bead-toggle">{expanded ? '▲' : '▼'}</span>
                  </button>
                  {expanded && (
                    <div className="history-bead-detail">
                      {detail.loading ? (
                        <p className="history-empty">Loading…</p>
                      ) : detail.data ? (
                        <>
                          {detail.data.executionContent && (
                            <div className="artifact-content">
                              <ReactMarkdown remarkPlugins={[remarkGfm]}>{detail.data.executionContent}</ReactMarkdown>
                            </div>
                          )}
                          {detail.data.chatMessages.length > 0 && (
                            <div className="messages">
                              {detail.data.chatMessages.map((msg, i) => (
                                <div key={i} className={`message ${msg.role}`}>
                                  <div className="message-role">{msg.role === 'user' ? 'You' : 'Agent'}</div>
                                  <div className="message-content">
                                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
                                  </div>
                                </div>
                              ))}
                            </div>
                          )}
                        </>
                      ) : (
                        <p className="history-empty">No execution data available.</p>
                      )}
                    </div>
                  )}
                </div>
              );
            })
          ) : (
            <p className="history-empty">No beads graph available for this version.</p>
          )}
        </div>
      );
    }

    return null;
  }

  const tabs = getTabsForStage(selectedStage);

  return (
    <div className="version-history-view">
      {/* Sidebar — same structure and CSS as StagesSidebar */}
      <nav className="stages-sidebar">
        <h3>v{version} <span style={{ color: '#f59e0b', fontWeight: 400 }}>read-only</span></h3>
        <ul>
          {PIPELINE_STAGES.map(stage => (
            <li
              key={stage}
              className={`stage-item approved${selectedStage === stage ? ' selected' : ''}`}
              onClick={() => setSelectedStage(stage)}
            >
              <div className="stage-item-row">
                <span className="stage-robot-wrapper">
                  <StageRobot stage={stage} />
                </span>
                <span className="stage-name">{STAGE_LABELS[stage]}</span>
              </div>
            </li>
          ))}
        </ul>
        <div className="sidebar-branding">
          <pre className="sidebar-ascii">
{"   ♥\n"}
{"  ╱│╲\n"}
{"┌──────┐\n"}
{"│ ◠  ◠ │\n"}
{"│ ╰──╯ │\n"}
{"└──┬───┘\n"}
{" "}
<span className="bead bead-pink">●</span>
<span className="bead bead-cyan">◉</span>
<span className="bead bead-amber">●</span>
<span className="bead bead-purple">◉</span>
<span className="bead bead-green">●</span>
          </pre>
          <div className="sidebar-brand-name">paulette {appVersion && <span>v{appVersion}</span>}</div>
          <div className="sidebar-copyright">&copy; {new Date().getFullYear()} {appAuthor || 'Michel Roberge'}</div>
        </div>
      </nav>

      {/* Main content area */}
      <div className="history-main-area">
        <div className="history-banner">
          <span>Viewing v{version} — read-only</span>
          <button className="history-exit-btn" onClick={onClose}>← Back to live</button>
        </div>

        {loading ? (
          <p className="history-empty" style={{ padding: '1rem' }}>Loading version data…</p>
        ) : (
          <div className="stage-view">
            <div className="stage-tab-bar">
              {tabs.map(tab => (
                <button
                  key={tab.id}
                  className={`stage-tab${activeTab === tab.id ? ' active' : ''}`}
                  onClick={() => setActiveTab(tab.id)}
                >
                  {tab.label}
                </button>
              ))}
            </div>
            <div className="stage-tab-content" style={{ overflowY: 'auto' }}>
              {renderTabContent()}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
