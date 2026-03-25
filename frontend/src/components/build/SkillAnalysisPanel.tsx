import { useState, useEffect, useRef } from 'react';
import type { SkillSuggestion } from '../../types';
import { analyzeSkills, getSkillSuggestions, getObservedSkills, approveSkills } from '../../api/skills';
import { BuildingAnimation } from '../ux/BuildingAnimation';

interface Props {
  projectId: string;
}

export function SkillAnalysisPanel({ projectId }: Props) {
  const [analyzing, setAnalyzing] = useState(false);
  const [streamText, setStreamText] = useState('');
  const [preSuggestions, setPreSuggestions] = useState<SkillSuggestion[]>([]);
  const [observedSuggestions, setObservedSuggestions] = useState<SkillSuggestion[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [approving, setApproving] = useState(false);
  const [approvedCount, setApprovedCount] = useState(0);
  const streamRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    getSkillSuggestions(projectId)
      .then(r => setPreSuggestions(r.suggestions || []))
      .catch(() => {});
    getObservedSkills(projectId)
      .then(r => setObservedSuggestions(r.suggestions || []))
      .catch(() => {});
  }, [projectId]);

  const allSuggestions = [...preSuggestions, ...observedSuggestions];

  const handleAnalyze = () => {
    setAnalyzing(true);
    setStreamText('');
    const controller = new AbortController();
    abortRef.current = controller;

    analyzeSkills(projectId, (event) => {
      if (event.type === 'chunk') {
        setStreamText(prev => prev + event.content);
        if (streamRef.current) {
          streamRef.current.scrollTop = streamRef.current.scrollHeight;
        }
      } else if (event.type === 'done') {
        setAnalyzing(false);
        getSkillSuggestions(projectId)
          .then(r => setPreSuggestions(r.suggestions || []))
          .catch(() => {});
      } else if (event.type === 'error') {
        setAnalyzing(false);
      }
    }, controller.signal).catch(() => setAnalyzing(false));
  };

  const toggleSelected = (idx: number) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  };

  const handleApprove = async () => {
    if (selected.size === 0) return;
    setApproving(true);
    try {
      const result = await approveSkills(projectId, Array.from(selected));
      setApprovedCount(prev => prev + result.count);
      setSelected(new Set());
      getSkillSuggestions(projectId)
        .then(r => setPreSuggestions(r.suggestions || []))
        .catch(() => {});
      getObservedSkills(projectId)
        .then(r => setObservedSuggestions(r.suggestions || []))
        .catch(() => {});
    } catch (err) {
      console.error('Failed to approve skills:', err);
    } finally {
      setApproving(false);
    }
  };

  // While analyzing: show split layout with animation + streaming
  if (analyzing) {
    return (
      <div className="mock-split-layout">
        <div className="mock-main-column">
          <BuildingAnimation />
        </div>
        <div className="mock-activity-panel">
          <div className="mock-activity-header">
            <span className="mock-stream-dot" />
            <span>Analyzing for reusable skills…</span>
          </div>
          <div className="mock-activity-content" ref={streamRef}>
            {streamText || 'Starting…'}
          </div>
        </div>
      </div>
    );
  }

  // After analysis or on first load: show suggestions + analyze button
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'auto', padding: '1rem', gap: '0.75rem' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
        <h3 style={{ margin: 0, fontSize: '0.9rem' }}>Skill Library</h3>
        <button
          className="generate-mock-button"
          onClick={handleAnalyze}
          style={{ marginLeft: 'auto' }}
        >
          Analyze Build Plan
        </button>
        {approvedCount > 0 && (
          <span style={{ fontSize: '0.8rem', color: '#94a3b8' }}>
            {approvedCount} skill{approvedCount !== 1 ? 's' : ''} created
          </span>
        )}
      </div>

      {allSuggestions.length > 0 && (
        <>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
            {allSuggestions.map((sg, idx) => (
              <label
                key={idx}
                style={{
                  display: 'flex',
                  gap: '0.5rem',
                  padding: '0.5rem',
                  background: sg.approved ? '#1e3a2f' : '#1e293b',
                  borderRadius: '0.375rem',
                  border: '1px solid #334155',
                  cursor: sg.approved ? 'default' : 'pointer',
                  opacity: sg.approved ? 0.6 : 1,
                }}
              >
                {!sg.approved && (
                  <input
                    type="checkbox"
                    checked={selected.has(idx)}
                    onChange={() => toggleSelected(idx)}
                  />
                )}
                {sg.approved && <span style={{ color: '#4ade80' }}>✓</span>}
                <div style={{ flex: 1 }}>
                  <div style={{ fontWeight: 600, fontSize: '0.85rem' }}>
                    {sg.name}
                    <span style={{ fontWeight: 400, color: '#94a3b8', marginLeft: '0.5rem', fontSize: '0.75rem' }}>
                      {sg.category}
                    </span>
                    {sg.sourceBeads && sg.sourceBeads.length > 0 && (
                      <span style={{ fontWeight: 400, color: '#818cf8', marginLeft: '0.5rem', fontSize: '0.75rem' }}>
                        (observed)
                      </span>
                    )}
                  </div>
                  <div style={{ fontSize: '0.8rem', color: '#94a3b8', marginTop: '0.25rem' }}>
                    {sg.description}
                  </div>
                  {sg.tags && sg.tags.length > 0 && (
                    <div style={{ marginTop: '0.25rem', display: 'flex', gap: '0.25rem', flexWrap: 'wrap' }}>
                      {sg.tags.map(t => (
                        <span key={t} style={{ fontSize: '0.7rem', padding: '0.1rem 0.4rem', background: '#334155', borderRadius: '0.25rem', color: '#cbd5e1' }}>
                          {t}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </label>
            ))}
          </div>

          {selected.size > 0 && (
            <button
              className="approve-button"
              onClick={handleApprove}
              disabled={approving}
              style={{ alignSelf: 'flex-start' }}
            >
              {approving ? 'Creating...' : `Approve ${selected.size} Skill${selected.size !== 1 ? 's' : ''}`}
            </button>
          )}
        </>
      )}

      {allSuggestions.length === 0 && (
        <p style={{ color: '#64748b', fontSize: '0.85rem', margin: 0 }}>
          Click "Analyze Build Plan" to identify reusable patterns that can become skills.
          Skills are saved to your paulette library for reuse across projects.
        </p>
      )}
    </div>
  );
}
