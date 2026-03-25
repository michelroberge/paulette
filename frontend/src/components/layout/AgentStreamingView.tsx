import { useRef, useEffect } from 'react';
import { BuildingAnimation } from '../ux/BuildingAnimation';

const operationLabels: Record<string, string> = {
  chat: 'Thinking…',
  'beads-generate': 'Planning tasks…',
  'beads-execute': 'Executing task…',
  'beads-review': 'Reviewing…',
  summary: 'Writing summary…',
};

interface Props {
  streamingText: string;
  operation: string;
  btwInput?: string;
  btwSending?: boolean;
  onBtwChange?: (v: string) => void;
  onBtwSubmit?: (e: React.SyntheticEvent) => void;
  btwPendingCount?: number;
}

export function AgentStreamingView({ streamingText, operation, btwInput = '', btwSending = false, onBtwChange, onBtwSubmit, btwPendingCount = 0 }: Readonly<Props>) {
  const streamRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (streamRef.current) {
      streamRef.current.scrollTop = streamRef.current.scrollHeight;
    }
  }, [streamingText]);

  const label = operationLabels[operation] ?? 'Working…';

  return (
    <div className="mock-split-layout">
      <div className="mock-main-column">
        <BuildingAnimation />
      </div>
      <div className="mock-activity-panel">
        {onBtwSubmit && (
          <form className="btw-form btw-form-main" onSubmit={onBtwSubmit}>
            <input
              className="btw-input"
              type="text"
              placeholder="btw…"
              value={btwInput}
              onChange={e => onBtwChange?.(e.target.value)}
              disabled={btwSending}
            />
            <button type="submit" className="btw-send-btn" disabled={!btwInput.trim() || btwSending}>↵</button>
            {btwPendingCount > 0 && (
              <span className="btw-pending-count" title={`${btwPendingCount} message${btwPendingCount > 1 ? 's' : ''} queued`}>{btwPendingCount}</span>
            )}
          </form>
        )}
        <div className="mock-activity-header">
          <span className="mock-stream-dot" />
          <span>{label}</span>
        </div>
        <div className="mock-activity-content" ref={streamRef}>
          {streamingText || 'Starting…'}
        </div>
      </div>
    </div>
  );
}
