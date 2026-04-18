import { useState } from 'react';
import type { RAGSource } from '../../types';

interface Props {
  sources: RAGSource[];
}

/**
 * Collapsible panel showing RAG knowledge sources used for the current response.
 */
export function RAGSourcesPanel({ sources }: Readonly<Props>) {
  const [expanded, setExpanded] = useState(false);

  if (sources.length === 0) return null;

  return (
    <div style={{
      margin: '8px 0',
      border: '1px solid rgba(139, 92, 246, 0.3)',
      borderRadius: '6px',
      background: 'rgba(139, 92, 246, 0.05)',
      fontSize: '0.8rem',
    }}>
      <button
        onClick={() => setExpanded(!expanded)}
        style={{
          width: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 10px',
          background: 'none',
          border: 'none',
          color: '#a78bfa',
          cursor: 'pointer',
          fontFamily: 'inherit',
          fontSize: 'inherit',
        }}
      >
        <span>{sources.length} knowledge source{sources.length !== 1 ? 's' : ''} used</span>
        <span>{expanded ? '\u25B2' : '\u25BC'}</span>
      </button>
      {expanded && (
        <div style={{ padding: '0 10px 8px' }}>
          {sources.map((s) => (
            <div
              key={s.chunk_id}
              style={{
                padding: '4px 0',
                borderTop: '1px solid rgba(139, 92, 246, 0.15)',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: '8px' }}>
                <span style={{ color: '#c4b5fd', fontWeight: 500 }}>
                  {s.file_path || s.chunk_id}
                </span>
                <span style={{ color: '#7c3aed', whiteSpace: 'nowrap' }}>
                  {s.collection} &middot; {(s.score * 100).toFixed(0)}%
                </span>
              </div>
              {s.preview && (
                <div style={{
                  color: '#9ca3af',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  marginTop: '2px',
                }}>
                  {s.preview}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
