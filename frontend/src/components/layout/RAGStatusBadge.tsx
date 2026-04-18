import { useState, useEffect } from 'react';
import { getRagStatus, type RAGStatus } from '../../api/rag';

/**
 * Small badge showing RAG pipeline connection status.
 * Polls every 30s. Only renders when RAG is enabled.
 */
export function RAGStatusBadge() {
  const [status, setStatus] = useState<RAGStatus | null>(null);

  useEffect(() => {
    let cancelled = false;

    const check = async () => {
      try {
        const s = await getRagStatus();
        if (!cancelled) setStatus(s);
      } catch {
        if (!cancelled) setStatus(null);
      }
    };

    check();
    const interval = setInterval(check, 30_000);
    return () => { cancelled = true; clearInterval(interval); };
  }, []);

  if (!status?.enabled) return null;

  return (
    <span
      className="rag-status-badge"
      title={status.healthy ? `RAG connected: ${status.baseUrl}` : 'RAG disconnected'}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '4px',
        fontSize: '0.75rem',
        padding: '2px 8px',
        borderRadius: '9999px',
        background: status.healthy ? 'rgba(34, 197, 94, 0.15)' : 'rgba(239, 68, 68, 0.15)',
        color: status.healthy ? '#22c55e' : '#ef4444',
        marginLeft: '8px',
      }}
    >
      <span style={{
        width: '6px',
        height: '6px',
        borderRadius: '50%',
        background: status.healthy ? '#22c55e' : '#ef4444',
      }} />
      RAG
    </span>
  );
}
