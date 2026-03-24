import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useBeadDetail } from '../../hooks/useBeadDetail';
import type { Bead } from '../../types';

interface Props {
  projectId: string;
  beadId: string;
  bead: Bead;
  allBeads: Bead[];
  onBeadSelect: (id: string) => void;
  onClose: () => void;
}

export function BeadDetailPanel({ projectId, beadId, bead, allBeads, onBeadSelect, onClose }: Props) {
  const { detail, loading, chatStreaming, chatStreamingContent, loadDetail, saveDetail, sendChat, stopChat, control } = useBeadDetail(projectId);
  const [description, setDescription] = useState(bead.description ?? '');
  const [notes, setNotes] = useState('');
  const [chatInput, setChatInput] = useState('');
  const [descDirty, setDescDirty] = useState(false);
  const [notesDirty, setNotesDirty] = useState(false);
  const [activeTab, setActiveTab] = useState<'details' | 'context'>('details');
  const chatEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    loadDetail(beadId);
  }, [beadId, loadDetail]);

  useEffect(() => {
    if (detail) {
      setDescription(detail.description ?? '');
      setNotes(detail.notes ?? '');
      setDescDirty(false);
      setNotesDirty(false);
    }
  }, [detail?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-scroll chat
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [detail?.chatMessages?.length, chatStreamingContent]);

  const isClosed = bead.status === 'closed';
  const isInProgress = bead.status === 'in_progress' || bead.status === 'reviewing';

  const handleSave = async () => {
    const updates: { description?: string; notes?: string } = {};
    if (descDirty) updates.description = description;
    if (notesDirty) updates.notes = notes;
    if (Object.keys(updates).length > 0) {
      await saveDetail(beadId, updates);
      setDescDirty(false);
      setNotesDirty(false);
    }
  };

  const handleSendChat = () => {
    if (!chatInput.trim()) return;
    sendChat(beadId, chatInput.trim());
    setChatInput('');
  };

  const statusColor: Record<string, string> = {
    open: '#f59e0b',
    in_progress: '#3b82f6',
    reviewing: '#ef4444',
    closed: '#22c55e',
    blocked: '#475569',
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal bead-detail-modal" onClick={e => e.stopPropagation()}>
        {/* Header */}
        <div className="bead-detail-header">
          <div className="bead-detail-title-row">
            <span className="bead-detail-type-badge" style={{ background: bead.type === 'epic' ? '#7c3aed' : '#0ea5e9' }}>
              {bead.type.toUpperCase()}
            </span>
            <span className="bead-detail-status-badge" style={{ background: statusColor[bead.status] ?? '#475569' }}>
              {bead.status.replace('_', ' ')}
            </span>
            <h2 className="bead-detail-title">{bead.title}</h2>
          </div>
          <button className="bead-detail-close" onClick={onClose}>✕</button>
        </div>

        {loading && !detail ? (
          <div className="bead-detail-loading">Loading...</div>
        ) : (
          <div className="bead-detail-body">
            {/* Left column: details + execution */}
            <div className="bead-detail-left">
              {/* Tab bar */}
              <div className="bead-detail-tabs">
                <button
                  className={`bead-detail-tab${activeTab === 'details' ? ' bead-detail-tab--active' : ''}`}
                  onClick={() => setActiveTab('details')}
                >
                  Details
                </button>
                <button
                  className={`bead-detail-tab${activeTab === 'context' ? ' bead-detail-tab--active' : ''}`}
                  onClick={() => setActiveTab('context')}
                >
                  Context
                </button>
              </div>

              {activeTab === 'details' && (
                <>
                  {/* Description */}
                  <div className="bead-detail-section">
                    <h3>Description</h3>
                    {isClosed ? (
                      <div className="bead-detail-readonly">
                        <ReactMarkdown remarkPlugins={[remarkGfm]}>{description || '*No description*'}</ReactMarkdown>
                      </div>
                    ) : (
                      <textarea
                        className="bead-detail-textarea"
                        value={description}
                        onChange={e => { setDescription(e.target.value); setDescDirty(true); }}
                        rows={4}
                        placeholder="Bead description..."
                      />
                    )}
                  </div>

                  {/* Notes */}
                  <div className="bead-detail-section">
                    <h3>Notes</h3>
                    {isClosed ? (
                      <div className="bead-detail-readonly">
                        <ReactMarkdown remarkPlugins={[remarkGfm]}>{notes || '*No notes*'}</ReactMarkdown>
                      </div>
                    ) : (
                      <textarea
                        className="bead-detail-textarea"
                        value={notes}
                        onChange={e => { setNotes(e.target.value); setNotesDirty(true); }}
                        rows={3}
                        placeholder="Add notes or extra context..."
                      />
                    )}
                  </div>

                  {/* Save + Controls */}
                  {!isClosed && (
                    <div className="bead-detail-actions">
                      {(descDirty || notesDirty) && (
                        <button className="generate-mock-button" onClick={handleSave}>Save Changes</button>
                      )}
                      {isInProgress && (
                        <button className="stop-button" onClick={() => control(beadId, 'pause')}>Pause</button>
                      )}
                      {(descDirty || notesDirty) && !isInProgress && (
                        <button className="stop-button" onClick={async () => { await handleSave(); await control(beadId, 'restart'); }}>
                          Save &amp; Restart
                        </button>
                      )}
                    </div>
                  )}

                  {/* Execution Result */}
                  {detail?.executionContent && (
                    <div className="bead-detail-section">
                      <h3>Execution Result</h3>
                      <div className="bead-detail-execution">
                        <ReactMarkdown remarkPlugins={[remarkGfm]}>{detail.executionContent}</ReactMarkdown>
                      </div>
                    </div>
                  )}
                </>
              )}

              {activeTab === 'context' && (
                <div className="bead-context">
                  {/* Parent Epic */}
                  {bead.epicId && (() => {
                    const epic = allBeads.find(b => b.id === bead.epicId);
                    return (
                      <div className="bead-detail-section">
                        <h3>Parent Epic</h3>
                        <button className="bead-context-chip bead-context-chip--epic" onClick={() => onBeadSelect(bead.epicId!)}>
                          {epic ? epic.title : bead.epicId}
                        </button>
                      </div>
                    );
                  })()}

                  {/* Dependencies */}
                  {bead.deps.length > 0 && (
                    <div className="bead-detail-section">
                      <h3>Dependencies</h3>
                      <div className="bead-context-chips">
                        {bead.deps.map(depId => {
                          const dep = allBeads.find(b => b.id === depId);
                          return (
                            <button
                              key={depId}
                              className="bead-context-chip"
                              style={{ borderColor: statusColor[dep?.status ?? ''] ?? '#475569' }}
                              onClick={() => onBeadSelect(depId)}
                            >
                              <span className="bead-context-chip-dot" style={{ background: statusColor[dep?.status ?? ''] ?? '#475569' }} />
                              {dep ? dep.title : depId}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  )}

                  {/* Tags */}
                  {bead.tags && bead.tags.length > 0 && (
                    <div className="bead-detail-section">
                      <h3>Tags</h3>
                      <div className="bead-context-chips">
                        {bead.tags.map(tag => (
                          <span key={tag} className="bead-context-tag">{tag}</span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Target Files */}
                  {bead.targetFiles && bead.targetFiles.length > 0 && (
                    <div className="bead-detail-section">
                      <h3>Target Files</h3>
                      <ul className="bead-context-file-list">
                        {bead.targetFiles.map(f => (
                          <li key={f} className="bead-context-file">{f}</li>
                        ))}
                      </ul>
                    </div>
                  )}

                  {/* Journey Refs */}
                  {bead.journeyRefs && bead.journeyRefs.length > 0 && (
                    <div className="bead-detail-section">
                      <h3>Journey References</h3>
                      <div className="bead-context-chips">
                        {bead.journeyRefs.map(ref => (
                          <span key={ref} className="bead-context-tag bead-context-tag--ref">{ref}</span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Architecture Refs */}
                  {bead.archRefs && bead.archRefs.length > 0 && (
                    <div className="bead-detail-section">
                      <h3>Architecture References</h3>
                      <div className="bead-context-chips">
                        {bead.archRefs.map(ref => (
                          <span key={ref} className="bead-context-tag bead-context-tag--ref">{ref}</span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Empty state */}
                  {!bead.epicId && bead.deps.length === 0 && (!bead.tags || bead.tags.length === 0) && (!bead.targetFiles || bead.targetFiles.length === 0) && (!bead.journeyRefs || bead.journeyRefs.length === 0) && (!bead.archRefs || bead.archRefs.length === 0) && (
                    <div className="bead-detail-readonly" style={{ color: '#64748b', fontStyle: 'italic' }}>
                      No context references on this bead.
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* Right column: chat */}
            <div className="bead-detail-right">
              <h3>Chat</h3>
              <div className="bead-detail-chat-messages">
                {detail?.chatMessages?.map((msg, i) => (
                  <div key={i} className={`bead-chat-msg bead-chat-${msg.role}`}>
                    <span className="bead-chat-role">{msg.role === 'user' ? 'You' : 'AI'}</span>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
                  </div>
                ))}
                {chatStreaming && chatStreamingContent && (
                  <div className="bead-chat-msg bead-chat-assistant">
                    <span className="bead-chat-role">AI</span>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{chatStreamingContent}</ReactMarkdown>
                  </div>
                )}
                <div ref={chatEndRef} />
              </div>
              <div className="bead-detail-chat-input">
                {chatStreaming ? (
                  <button className="stop-button" onClick={stopChat} style={{ width: '100%' }}>Stop</button>
                ) : (
                  <>
                    <input
                      type="text"
                      value={chatInput}
                      onChange={e => setChatInput(e.target.value)}
                      onKeyDown={e => { if (e.key === 'Enter') handleSendChat(); }}
                      placeholder="Ask about this bead..."
                      className="refinement-input"
                    />
                    <button className="generate-mock-button" onClick={handleSendChat} disabled={!chatInput.trim()}>
                      Send
                    </button>
                  </>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
