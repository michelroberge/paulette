import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useBeadDetail } from '../../hooks/useBeadDetail';
import { BeadCodeTab } from './BeadCodeTab';
import type { Bead, BeadGraph, InstructionPlan } from '../../types';

interface Props {
  projectId: string;
  beadId: string;
  bead: Bead;
  allBeads: Bead[];
  onBeadSelect: (id: string) => void;
  onClose: () => void;
  onBeadsUpdated?: (graph: BeadGraph) => void;
}

type ChatMode = 'ask' | 'instruct';

export function BeadDetailPanel({ projectId, beadId, bead, allBeads, onBeadSelect, onClose, onBeadsUpdated }: Props) {
  const {
    detail, loading,
    chatStreaming, chatStreamingContent,
    instructStreaming, instructStreamingContent, instructionProposal, applyingProposal,
    loadDetail, saveDetail, sendChat, stopChat, sendInstruct, applyProposal, dismissProposal,
    control,
  } = useBeadDetail(projectId);

  const [description, setDescription] = useState(bead.description ?? '');
  const [notes, setNotes] = useState('');
  const [chatInput, setChatInput] = useState('');
  const [descDirty, setDescDirty] = useState(false);
  const [notesDirty, setNotesDirty] = useState(false);
  const [activeTab, setActiveTab] = useState<'details' | 'context' | 'code'>('details');
  const [chatMode, setChatMode] = useState<ChatMode>('ask');
  const [selectedNewBeads, setSelectedNewBeads] = useState<Set<number>>(new Set());
  const [selectedUpdates, setSelectedUpdates] = useState<Set<number>>(new Set());
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
  }, [detail?.chatMessages?.length, chatStreamingContent, instructStreamingContent]);

  // Pre-select all items when proposal arrives
  useEffect(() => {
    if (instructionProposal) {
      setSelectedNewBeads(new Set((instructionProposal.newBeads ?? []).map((_, i) => i)));
      setSelectedUpdates(new Set((instructionProposal.updatedBeads ?? []).map((_, i) => i)));
    }
  }, [instructionProposal]);

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
    const msg = chatInput.trim();
    setChatInput('');
    if (chatMode === 'ask') {
      sendChat(beadId, msg);
    } else {
      sendInstruct(msg);
    }
  };

  const handleApproveProposal = async () => {
    if (!instructionProposal) return;
    const filteredPlan: InstructionPlan = {
      ...instructionProposal,
      newBeads: (instructionProposal.newBeads ?? []).filter((_, i) => selectedNewBeads.has(i)),
      updatedBeads: (instructionProposal.updatedBeads ?? []).filter((_, i) => selectedUpdates.has(i)),
    };
    const updatedGraph = await applyProposal(filteredPlan);
    if (updatedGraph && onBeadsUpdated) {
      onBeadsUpdated(updatedGraph);
    }
  };

  const statusColor: Record<string, string> = {
    open: '#f59e0b',
    in_progress: '#3b82f6',
    reviewing: '#ef4444',
    closed: '#22c55e',
    blocked: '#475569',
  };

  const anyStreaming = chatStreaming || instructStreaming;

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
            {/* Left column: tabs + content */}
            <div className={`bead-detail-left${activeTab === 'code' ? ' bead-detail-left--code' : ''}`}>
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
                {(bead.targetFiles && bead.targetFiles.length > 0) && (
                  <button
                    className={`bead-detail-tab${activeTab === 'code' ? ' bead-detail-tab--active' : ''}`}
                    onClick={() => setActiveTab('code')}
                  >
                    Code
                    {bead.status === 'in_progress' && (
                      <span className="bead-tab-live-dot" title="Agent is writing code" />
                    )}
                  </button>
                )}
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

              {activeTab === 'code' && (
                <BeadCodeTab
                  projectId={projectId}
                  beadId={beadId}
                  beadStatus={bead.status}
                  targetFiles={bead.targetFiles ?? []}
                />
              )}
            </div>

            {/* Right column: chat */}
            <div className="bead-detail-right">
              {/* Chat mode toggle */}
              <div className="bead-chat-header">
                <div className="bead-chat-mode-toggle">
                  <button
                    className={`bead-chat-mode-btn${chatMode === 'ask' ? ' bead-chat-mode-btn--active' : ''}`}
                    onClick={() => { setChatMode('ask'); dismissProposal(); }}
                    disabled={anyStreaming}
                  >
                    Ask
                  </button>
                  <button
                    className={`bead-chat-mode-btn${chatMode === 'instruct' ? ' bead-chat-mode-btn--active' : ''}`}
                    onClick={() => setChatMode('instruct')}
                    disabled={anyStreaming}
                  >
                    Instruct
                  </button>
                </div>
                {chatMode === 'instruct' && (
                  <span className="bead-chat-mode-hint">Describe additional work needed</span>
                )}
              </div>

              <div className="bead-detail-chat-messages">
                {chatMode === 'ask' && (
                  <>
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
                  </>
                )}

                {chatMode === 'instruct' && (
                  <>
                    {/* Streaming planning text */}
                    {instructStreaming && instructStreamingContent && (
                      <div className="bead-chat-msg bead-chat-assistant">
                        <span className="bead-chat-role">Planning…</span>
                        <div className="bead-instruct-stream">{instructStreamingContent}</div>
                      </div>
                    )}

                    {/* Instruction proposal */}
                    {instructionProposal && !instructStreaming && (
                      <InstructionProposalView
                        plan={instructionProposal}
                        selectedNewBeads={selectedNewBeads}
                        selectedUpdates={selectedUpdates}
                        onToggleNewBead={i => setSelectedNewBeads(prev => {
                          const next = new Set(prev);
                          if (next.has(i)) next.delete(i); else next.add(i);
                          return next;
                        })}
                        onToggleUpdate={i => setSelectedUpdates(prev => {
                          const next = new Set(prev);
                          if (next.has(i)) next.delete(i); else next.add(i);
                          return next;
                        })}
                        onApprove={handleApproveProposal}
                        onDismiss={dismissProposal}
                        applying={applyingProposal}
                      />
                    )}

                    {!instructStreaming && !instructionProposal && (
                      <div className="bead-instruct-hint">
                        <p>Describe additional work, new requirements, or changes to the build plan. Claude will propose new beads and plan updates for your approval.</p>
                      </div>
                    )}
                  </>
                )}

                <div ref={chatEndRef} />
              </div>

              <div className="bead-detail-chat-input">
                {anyStreaming ? (
                  <button className="stop-button" onClick={stopChat} style={{ width: '100%' }}>Stop</button>
                ) : (
                  <>
                    <input
                      type="text"
                      value={chatInput}
                      onChange={e => setChatInput(e.target.value)}
                      onKeyDown={e => { if (e.key === 'Enter') handleSendChat(); }}
                      placeholder={chatMode === 'ask' ? 'Ask about this bead...' : 'Describe additional work needed...'}
                      className="refinement-input"
                    />
                    <button className="generate-mock-button" onClick={handleSendChat} disabled={!chatInput.trim()}>
                      {chatMode === 'ask' ? 'Send' : 'Plan'}
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

// ── Instruction Proposal View ──

interface ProposalProps {
  plan: InstructionPlan;
  selectedNewBeads: Set<number>;
  selectedUpdates: Set<number>;
  onToggleNewBead: (i: number) => void;
  onToggleUpdate: (i: number) => void;
  onApprove: () => void;
  onDismiss: () => void;
  applying: boolean;
}

function InstructionProposalView({
  plan,
  selectedNewBeads,
  selectedUpdates,
  onToggleNewBead,
  onToggleUpdate,
  onApprove,
  onDismiss,
  applying,
}: ProposalProps) {
  const [reasoningOpen, setReasoningOpen] = useState(false);
  const hasChanges = selectedNewBeads.size > 0 || selectedUpdates.size > 0
    || !!plan.buildPlanChanges || !!plan.architectureChanges;

  return (
    <div className="bead-instruct-proposal">
      <div className="bead-instruct-proposal-header">
        <span className="bead-instruct-proposal-title">Proposed Changes</span>
        <button className="bead-instruct-dismiss" onClick={onDismiss} title="Dismiss">✕</button>
      </div>

      {/* Reasoning (collapsible) */}
      {plan.reasoning && (
        <div className="bead-instruct-section">
          <button className="bead-instruct-collapse" onClick={() => setReasoningOpen(v => !v)}>
            {reasoningOpen ? '▾' : '▸'} Reasoning
          </button>
          {reasoningOpen && (
            <div className="bead-instruct-reasoning">{plan.reasoning}</div>
          )}
        </div>
      )}

      {/* Build plan changes */}
      {plan.buildPlanChanges && (
        <div className="bead-instruct-section">
          <div className="bead-instruct-change-label">Build Plan Update</div>
          <div className="bead-instruct-doc-preview">{plan.buildPlanChanges.slice(0, 300)}{plan.buildPlanChanges.length > 300 ? '…' : ''}</div>
        </div>
      )}

      {/* Architecture changes */}
      {plan.architectureChanges && (
        <div className="bead-instruct-section">
          <div className="bead-instruct-change-label">Architecture Update</div>
          <div className="bead-instruct-doc-preview">{plan.architectureChanges.slice(0, 300)}{plan.architectureChanges.length > 300 ? '…' : ''}</div>
        </div>
      )}

      {/* New beads */}
      {plan.newBeads && plan.newBeads.length > 0 && (
        <div className="bead-instruct-section">
          <div className="bead-instruct-change-label">New Beads ({plan.newBeads.length})</div>
          {plan.newBeads.map((nb, i) => (
            <label key={i} className="bead-instruct-item">
              <input
                type="checkbox"
                checked={selectedNewBeads.has(i)}
                onChange={() => onToggleNewBead(i)}
              />
              <div className="bead-instruct-item-info">
                <span className="bead-instruct-item-title">{nb.title}</span>
                {nb.description && <span className="bead-instruct-item-desc">{nb.description.slice(0, 100)}{nb.description.length > 100 ? '…' : ''}</span>}
                <div className="bead-instruct-item-meta">
                  <span className="bead-context-tag">{nb.type}</span>
                  {nb.tags?.map(t => <span key={t} className="bead-context-tag">{t}</span>)}
                </div>
              </div>
            </label>
          ))}
        </div>
      )}

      {/* Bead updates */}
      {plan.updatedBeads && plan.updatedBeads.length > 0 && (
        <div className="bead-instruct-section">
          <div className="bead-instruct-change-label">Bead Updates ({plan.updatedBeads.length})</div>
          {plan.updatedBeads.map((ub, i) => (
            <label key={i} className="bead-instruct-item">
              <input
                type="checkbox"
                checked={selectedUpdates.has(i)}
                onChange={() => onToggleUpdate(i)}
              />
              <div className="bead-instruct-item-info">
                <span className="bead-instruct-item-title">{ub.title ?? ub.id}</span>
                {ub.description && <span className="bead-instruct-item-desc">{ub.description.slice(0, 100)}{ub.description.length > 100 ? '…' : ''}</span>}
              </div>
            </label>
          ))}
        </div>
      )}

      {/* Actions */}
      <div className="bead-instruct-actions">
        <button
          className="generate-mock-button"
          onClick={onApprove}
          disabled={applying || !hasChanges}
        >
          {applying ? 'Applying…' : 'Approve Selected'}
        </button>
        <button className="stop-button" onClick={onDismiss} disabled={applying}>
          Discard
        </button>
      </div>
    </div>
  );
}
