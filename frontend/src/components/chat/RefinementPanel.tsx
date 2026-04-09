import { useEffect, useRef, useState } from 'react';
import type { UseRefinementLoopResult, LogEntry } from '../../hooks/useRefinementLoop';
import type { SectionName, ReviewItem, ReviewSource } from '../../types/refinement';

export const SECTION_LABELS: Record<SectionName, string> = {
  problem: 'Problem',
  users: 'Users',
  features: 'Features',
  ux: 'User Experience',
  metrics: 'Metrics',
  constraints: 'Constraints',
  out_of_scope: 'Out of Scope',
};

const ALL_SECTIONS: SectionName[] = [
  'problem', 'users', 'features', 'ux', 'metrics', 'constraints', 'out_of_scope',
];

const PHASE_ICONS: Record<string, string> = {
  summarize: 'S',
  generate_questions: '?',
  await_answer: 'A',
  extract_facts: 'F',
  update_sections: 'U',
  score_confidence: '%',
  find_gaps: 'G',
  critique: 'C',
  synthesize: 'D',
  complete: 'OK',
};

const SOURCE_LABELS: Record<ReviewSource, string> = {
  critique: 'Critique',
  coherence: 'Contradiction',
  tension: 'Challenge',
  gap: 'Gap',
};

const SOURCE_COLORS: Record<ReviewSource, string> = {
  critique: '#ff9800',
  coherence: '#f44336',
  tension: '#ffb74d',
  gap: '#4a9eff',
};

interface Props {
  loop: UseRefinementLoopResult;
  onSend: (message: string) => void;
  onContinue?: (message: string) => void;
}

export function RefinementPanel({ loop, onSend, onContinue }: Props) {
  const [inputValue, setInputValue] = useState('');
  const [answerValue, setAnswerValue] = useState('');
  const [continueValue, setContinueValue] = useState('');
  const [showReviewQueue, setShowReviewQueue] = useState(false);
  const [addressingItem, setAddressingItem] = useState<ReviewItem | null>(null);
  const [addressValue, setAddressValue] = useState('');
  const [addressSending, setAddressSending] = useState(false);
  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [loop.log.length, loop.phase]);

  useEffect(() => {
    loop.loadState();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleStart = () => {
    if (inputValue.trim()) {
      onSend(inputValue.trim());
      setInputValue('');
    }
  };

  const handleAnswer = async () => {
    if (answerValue.trim()) {
      await loop.answer(answerValue.trim());
      setAnswerValue('');
    }
  };

  const handleAddressSend = async () => {
    if (!addressingItem || !addressValue.trim() || addressSending) return;
    setAddressSending(true);
    try {
      await loop.addressReview(addressingItem.id, addressValue.trim());
      setAddressingItem(null);
      setAddressValue('');
      setShowReviewQueue(false);
    } finally {
      setAddressSending(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent, action: () => void) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      action();
    }
  };

  const avgConfidence = (() => {
    const values = ALL_SECTIONS.map(s => loop.confidence[s] ?? 0);
    return values.reduce((a, b) => a + b, 0) / values.length;
  })();

  // Not started yet — show start form
  if (!loop.isRunning && loop.phase === 'init' && !loop.state) {
    return (
      <div style={{ padding: '1rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
        <div style={{ fontSize: '0.95rem', color: 'var(--text-secondary, #888)' }}>
          Guided refinement breaks your idea into focused questions, building the vision step by step.
          Works well with smaller models.
        </div>
        <textarea
          value={inputValue}
          onChange={e => setInputValue(e.target.value)}
          onKeyDown={e => handleKeyDown(e, handleStart)}
          placeholder="Describe your product idea..."
          rows={4}
          style={{
            width: '100%', padding: '0.75rem', borderRadius: '6px',
            border: '1px solid var(--border, #333)', background: 'var(--bg-input, #1a1a1a)',
            color: 'var(--text, #e0e0e0)', resize: 'vertical', fontFamily: 'inherit',
          }}
        />
        <button
          onClick={handleStart}
          disabled={!inputValue.trim()}
          style={{
            padding: '0.5rem 1.5rem', borderRadius: '6px', border: 'none',
            background: inputValue.trim() ? 'var(--accent, #4a9eff)' : '#333',
            color: '#fff', cursor: inputValue.trim() ? 'pointer' : 'default',
            alignSelf: 'flex-end',
          }}
        >
          Start Guided Refinement
        </button>
      </div>
    );
  }

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      gap: '0.5rem', overflow: 'hidden', position: 'relative',
    }}>
      {/* Confidence dashboard — compact, always visible */}
      <div style={{
        padding: '0.5rem 0.75rem', flexShrink: 0,
        background: 'var(--bg-surface, #1e1e1e)', borderBottom: '1px solid var(--border, #333)',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '0.4rem' }}>
          <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-secondary, #888)' }}>
            {loop.isRunning && <span style={{
              display: 'inline-block', width: '8px', height: '8px', borderRadius: '50%',
              background: '#4a9eff', marginRight: '6px', animation: 'pulse 1.5s ease infinite',
            }} />}
            Step {loop.iteration} / {loop.maxIter}
            <span style={{ marginLeft: '0.75rem', fontWeight: 400 }}>
              Avg: <span style={{ color: avgConfidence >= 0.85 ? '#4caf50' : '#ff9800' }}>
                {Math.round(avgConfidence * 100)}%
              </span>
            </span>
            {loop.pendingReviewCount > 0 && (
              <button
                onClick={() => setShowReviewQueue(v => !v)}
                title="Review queue — AI-identified concerns"
                style={{
                  marginLeft: '0.5rem', display: 'inline-flex', alignItems: 'center',
                  justifyContent: 'center', minWidth: '20px', height: '20px',
                  borderRadius: '10px', border: 'none', padding: '0 5px',
                  background: '#ff9800', color: '#000', fontWeight: 700,
                  fontSize: '0.65rem', cursor: 'pointer', verticalAlign: 'middle',
                }}
              >
                {loop.pendingReviewCount}
              </button>
            )}
          </span>
          {!loop.isRunning && loop.phase !== 'init' && (
            <button onClick={loop.reset} style={{
              padding: '2px 8px', borderRadius: '3px', fontSize: '0.7rem',
              border: '1px solid var(--border, #444)', background: 'transparent',
              color: 'var(--text-secondary, #888)', cursor: 'pointer',
            }}>Reset</button>
          )}
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: '4px' }}>
          {ALL_SECTIONS.map(section => {
            const value = loop.confidence[section] ?? 0;
            return (
              <div key={section} style={{ textAlign: 'center' }}>
                <div style={{
                  height: '4px', borderRadius: '2px', marginBottom: '2px',
                  background: 'var(--bg-input, #2a2a2a)', overflow: 'hidden',
                }}>
                  <div style={{
                    width: `${Math.round(value * 100)}%`, height: '100%',
                    borderRadius: '2px', transition: 'width 0.3s ease',
                    background: value >= 0.85 ? '#4caf50' : value >= 0.5 ? '#ff9800' : '#f44336',
                  }} />
                </div>
                <span style={{ fontSize: '0.6rem', color: 'var(--text-secondary, #666)' }}>
                  {SECTION_LABELS[section].slice(0, 4)}
                </span>
              </div>
            );
          })}
        </div>
      </div>

      {/* Review queue panel — collapsible */}
      {showReviewQueue && (
        <div style={{
          padding: '0.5rem 0.75rem', flexShrink: 0, maxHeight: '200px', overflow: 'auto',
          background: 'var(--bg-highlight, #1a1a2e)', borderBottom: '1px solid var(--border, #333)',
        }}>
          <div style={{
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
            marginBottom: '0.4rem',
          }}>
            <span style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-secondary, #888)' }}>
              Review Queue ({loop.pendingReviewCount})
            </span>
            <button
              onClick={() => setShowReviewQueue(false)}
              style={{
                padding: '1px 6px', borderRadius: '3px', fontSize: '0.65rem',
                border: '1px solid var(--border, #444)', background: 'transparent',
                color: 'var(--text-secondary, #888)', cursor: 'pointer',
              }}
            >Close</button>
          </div>
          {loop.reviewItems.filter(r => r.status === 'pending').map(item => (
            <div key={item.id} style={{
              padding: '0.4rem 0.5rem', marginBottom: '4px', borderRadius: '4px',
              background: 'var(--bg-surface, #1e1e1e)', border: '1px solid var(--border, #333)',
            }}>
              <div style={{ display: 'flex', gap: '6px', alignItems: 'center', marginBottom: '3px' }}>
                <span style={{
                  fontSize: '0.55rem', fontWeight: 700, textTransform: 'uppercase',
                  padding: '1px 5px', borderRadius: '3px', letterSpacing: '0.03em',
                  background: SOURCE_COLORS[item.source] + '22',
                  color: SOURCE_COLORS[item.source],
                  border: `1px solid ${SOURCE_COLORS[item.source]}44`,
                }}>
                  {SOURCE_LABELS[item.source]}
                </span>
                <span style={{ fontSize: '0.6rem', color: 'var(--text-secondary, #666)' }}>
                  {SECTION_LABELS[item.section]}
                </span>
              </div>
              <div style={{ fontSize: '0.8rem', color: 'var(--text, #ccc)', marginBottom: '4px', lineHeight: 1.4 }}>
                {item.text}
              </div>
              <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
                <button
                  onClick={() => loop.discardReview(item.id)}
                  style={{
                    padding: '2px 8px', borderRadius: '3px', fontSize: '0.65rem',
                    border: '1px solid var(--border, #444)', background: 'transparent',
                    color: 'var(--text-secondary, #888)', cursor: 'pointer',
                  }}
                >Discard</button>
                <button
                  onClick={() => { setAddressingItem(item); setAddressValue(''); }}
                  style={{
                    padding: '2px 8px', borderRadius: '3px', fontSize: '0.65rem',
                    border: 'none', background: 'var(--accent, #4a9eff)',
                    color: '#fff', cursor: 'pointer',
                  }}
                >Address</button>
              </div>
            </div>
          ))}
          {loop.pendingReviewCount === 0 && (
            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary, #666)', textAlign: 'center', padding: '0.5rem' }}>
              No pending items
            </div>
          )}
        </div>
      )}

      {/* Conversation area — scrollable main area */}
      <div style={{
        flex: 1, overflow: 'auto', padding: '0.5rem 0.75rem',
        display: 'flex', flexDirection: 'column', gap: '6px',
      }}>
        {loop.log.map((entry, i) => {
          // User answer bubble
          if (entry.userAnswer) {
            return (
              <div key={i} style={{
                display: 'flex', justifyContent: 'flex-end', marginTop: '2px',
              }}>
                <div style={{
                  maxWidth: '80%', padding: '0.5rem 0.75rem', borderRadius: '12px 12px 4px 12px',
                  background: 'var(--accent, #4a9eff)', color: '#fff',
                  fontSize: '0.85rem', lineHeight: 1.45, whiteSpace: 'pre-wrap',
                }}>
                  {entry.userAnswer}
                </div>
              </div>
            );
          }

          // Question bubble (assistant side)
          if (entry.phase === 'await_answer' && entry.question) {
            return (
              <div key={i} style={{
                display: 'flex', justifyContent: 'flex-start', marginTop: '6px',
              }}>
                <div style={{
                  maxWidth: '80%', padding: '0.5rem 0.75rem', borderRadius: '12px 12px 12px 4px',
                  background: 'var(--bg-highlight, #252525)', border: '1px solid var(--border, #333)',
                }}>
                  <div style={{
                    fontSize: '0.65rem', color: 'var(--accent, #4a9eff)', marginBottom: '4px',
                    fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.03em',
                  }}>
                    {SECTION_LABELS[entry.question.section]}
                  </div>
                  <div style={{ color: 'var(--text, #e0e0e0)', fontSize: '0.85rem', lineHeight: 1.45 }}>
                    {entry.question.text}
                  </div>
                </div>
              </div>
            );
          }

          // Status line (non-Q&A phases)
          return <LogLine key={i} entry={entry} />;
        })}

        {/* Answer input — question is already visible in the log above */}
        {loop.currentQuestion && loop.phase === 'await_answer' && (
          <div style={{
            display: 'flex', gap: '0.5rem', marginTop: '4px',
            padding: '0 0.5rem',
          }}>
            <textarea
              value={answerValue}
              onChange={e => setAnswerValue(e.target.value)}
              onKeyDown={e => handleKeyDown(e, handleAnswer)}
              placeholder="Your answer..."
              rows={2}
              style={{
                flex: 1, padding: '0.5rem', borderRadius: '6px', fontSize: '0.85rem',
                border: '1px solid var(--accent, #4a9eff)', background: 'var(--bg-input, #1a1a1a)',
                color: 'var(--text, #e0e0e0)', resize: 'vertical', fontFamily: 'inherit',
              }}
            />
            <button
              onClick={handleAnswer}
              disabled={!answerValue.trim()}
              style={{
                padding: '0.4rem 0.8rem', borderRadius: '4px', border: 'none', fontSize: '0.8rem',
                background: answerValue.trim() ? 'var(--accent, #4a9eff)' : '#333',
                color: '#fff', cursor: answerValue.trim() ? 'pointer' : 'default',
                alignSelf: 'flex-end',
              }}
            >Answer</button>
          </div>
        )}

        {/* Error */}
        {loop.error && (
          <div style={{
            padding: '0.5rem', borderRadius: '4px', marginTop: '4px',
            background: '#3a1515', border: '1px solid #f44336', color: '#ff6b6b',
            fontSize: '0.8rem',
          }}>
            {loop.error}
          </div>
        )}

        {/* Complete */}
        {loop.phase === 'complete' && !loop.isRunning && (
          <>
            <div style={{
              padding: '0.5rem', borderRadius: '4px', marginTop: '4px',
              background: '#153a15', border: '1px solid #4caf50', color: '#81c784',
              fontSize: '0.8rem',
            }}>
              Vision document generated. Check the Artifact tab{onContinue ? ', or continue refining below.' : '.'}
            </div>
            {onContinue && (
              <div style={{ display: 'flex', gap: '0.5rem', marginTop: '6px', padding: '0 0.5rem' }}>
                <textarea
                  value={continueValue}
                  onChange={e => setContinueValue(e.target.value)}
                  onKeyDown={e => handleKeyDown(e, () => {
                    if (continueValue.trim()) { onContinue(continueValue.trim()); setContinueValue(''); }
                  })}
                  placeholder="Continue refining the vision..."
                  rows={2}
                  style={{
                    flex: 1, padding: '0.5rem', borderRadius: '6px', fontSize: '0.85rem',
                    border: '1px solid var(--accent, #4a9eff)', background: 'var(--bg-input, #1a1a1a)',
                    color: 'var(--text, #e0e0e0)', resize: 'vertical', fontFamily: 'inherit',
                  }}
                />
                <button
                  onClick={() => { if (continueValue.trim()) { onContinue(continueValue.trim()); setContinueValue(''); } }}
                  disabled={!continueValue.trim()}
                  style={{
                    padding: '0.4rem 0.8rem', borderRadius: '4px', border: 'none', fontSize: '0.8rem',
                    background: continueValue.trim() ? 'var(--accent, #4a9eff)' : '#333',
                    color: '#fff', cursor: continueValue.trim() ? 'pointer' : 'default',
                    alignSelf: 'flex-end',
                  }}
                >Send</button>
              </div>
            )}
          </>
        )}

        <div ref={logEndRef} />
      </div>

      {/* Address modal */}
      {addressingItem && (
        <div style={{
          position: 'absolute', inset: 0, display: 'flex', alignItems: 'center',
          justifyContent: 'center', background: 'rgba(0,0,0,0.6)', zIndex: 50,
        }}>
          <div style={{
            width: '90%', maxWidth: '500px', borderRadius: '8px',
            background: 'var(--bg-surface, #1e1e1e)', border: '1px solid var(--border, #333)',
            padding: '1rem', display: 'flex', flexDirection: 'column', gap: '0.75rem',
          }}>
            {/* Concern header */}
            <div>
              <div style={{ display: 'flex', gap: '6px', alignItems: 'center', marginBottom: '4px' }}>
                <span style={{
                  fontSize: '0.6rem', fontWeight: 700, textTransform: 'uppercase',
                  padding: '1px 5px', borderRadius: '3px', letterSpacing: '0.03em',
                  background: SOURCE_COLORS[addressingItem.source] + '22',
                  color: SOURCE_COLORS[addressingItem.source],
                  border: `1px solid ${SOURCE_COLORS[addressingItem.source]}44`,
                }}>
                  {SOURCE_LABELS[addressingItem.source]}
                </span>
                <span style={{ fontSize: '0.65rem', color: 'var(--text-secondary, #666)' }}>
                  {SECTION_LABELS[addressingItem.section]}
                </span>
              </div>
              <div style={{
                fontSize: '0.85rem', color: 'var(--text, #e0e0e0)', lineHeight: 1.5,
                padding: '0.5rem', borderRadius: '4px',
                background: 'var(--bg-highlight, #252525)', border: '1px solid var(--border, #333)',
              }}>
                {addressingItem.text}
              </div>
            </div>

            {/* User input */}
            <textarea
              value={addressValue}
              onChange={e => setAddressValue(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && !e.shiftKey && addressValue.trim() && !addressSending) {
                  e.preventDefault();
                  handleAddressSend();
                }
              }}
              placeholder="Your thoughts on this concern..."
              rows={3}
              autoFocus
              disabled={addressSending}
              style={{
                width: '100%', padding: '0.5rem', borderRadius: '6px', fontSize: '0.85rem',
                border: '1px solid var(--accent, #4a9eff)', background: 'var(--bg-input, #1a1a1a)',
                color: 'var(--text, #e0e0e0)', resize: 'vertical', fontFamily: 'inherit',
                opacity: addressSending ? 0.6 : 1,
              }}
            />

            {/* Actions */}
            <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end' }}>
              <button
                onClick={() => { setAddressingItem(null); setAddressValue(''); }}
                disabled={addressSending}
                style={{
                  padding: '0.4rem 1rem', borderRadius: '4px', fontSize: '0.8rem',
                  border: '1px solid var(--border, #444)', background: 'transparent',
                  color: 'var(--text-secondary, #888)', cursor: addressSending ? 'default' : 'pointer',
                }}
              >Cancel</button>
              <button
                onClick={handleAddressSend}
                disabled={!addressValue.trim() || addressSending}
                style={{
                  padding: '0.4rem 1rem', borderRadius: '4px', border: 'none', fontSize: '0.8rem',
                  background: addressValue.trim() && !addressSending ? 'var(--accent, #4a9eff)' : '#333',
                  color: '#fff', cursor: addressValue.trim() && !addressSending ? 'pointer' : 'default',
                }}
              >{addressSending ? 'Updating...' : 'Send'}</button>
            </div>
          </div>
        </div>
      )}

      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.3; }
        }
      `}</style>
    </div>
  );
}

function LogLine({ entry }: { entry: LogEntry }) {
  const icon = PHASE_ICONS[entry.phase] || '>';
  const isQuestion = entry.phase === 'await_answer' && entry.question;

  return (
    <div style={{
      display: 'flex', gap: '0.5rem', alignItems: 'flex-start',
      fontSize: '0.8rem', lineHeight: 1.4,
      padding: '2px 0',
    }}>
      <span style={{
        width: '18px', height: '18px', borderRadius: '3px', flexShrink: 0,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: '0.6rem', fontWeight: 700, marginTop: '1px',
        background: isQuestion ? 'var(--accent, #4a9eff)' : 'var(--bg-input, #2a2a2a)',
        color: isQuestion ? '#fff' : 'var(--text-secondary, #888)',
      }}>
        {icon}
      </span>
      <span style={{ color: 'var(--text, #ccc)' }}>
        {isQuestion && entry.question ? (
          <span style={{ color: 'var(--accent, #4a9eff)' }}>{entry.question.text}</span>
        ) : (
          entry.message
        )}
      </span>
    </div>
  );
}
