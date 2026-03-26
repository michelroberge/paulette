import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Message } from '../../types';
import { ConnectionErrorBanner } from './ConnectionErrorBanner';
import type { ConnectionError } from '../../types/provider';

const ARTIFACT_RE = /<!--\s*ARTIFACT:START\s*-->[\s\S]*?<!--\s*ARTIFACT:END\s*-->/g;
function stripArtifact(text: string) {
  return text.replace(ARTIFACT_RE, '').trim();
}

interface Props {
  messages: Message[];
  streaming: boolean;
  streamingContent: string;
  onSend: (message: string) => void;
  onStop?: () => void;
  /**
   * When set, replaces the streaming area with a `ConnectionErrorBanner` describing
   * the failing provider connection (SCR-012 / JRN-v0.2.0-008).
   * Cleared automatically by `useChat` when the user sends a new message.
   */
  connectionError?: ConnectionError | null;
  /**
   * Opens the Project Stage Settings slide-over so the user can reassign this
   * stage to a working connection without navigating away.
   * When omitted, the "Change stage connection" CTA is not rendered in the banner.
   */
  onOpenProjectSettings?: () => void;
}

export function ChatPanel({ messages, streaming, streamingContent, onSend, onStop, connectionError, onOpenProjectSettings }: Props) {
  const [input, setInput] = useState('');
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamingContent]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || streaming) return;
    onSend(input.trim());
    setInput('');
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  return (
    <div className="chat-panel">
      <div className="messages">
        {messages.map((msg, i) => (
          <div key={i} className={`message ${msg.role}${msg.isError ? ' error' : ''}`}>
            <div className="message-role">{msg.role === 'user' ? 'You' : 'Agent'}</div>
            <div className="message-content">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
            </div>
          </div>
        ))}
        {/* Connection error banner replaces the streaming area (SCR-012 / JRN-v0.2.0-008).
            The banner persists until the user sends a new message (which clears it in useChat). */}
        {connectionError ? (
          <ConnectionErrorBanner
            connectionName={connectionError.connectionName}
            connectionId={connectionError.connectionId}
            reason={connectionError.reason}
            onOpenProjectSettings={onOpenProjectSettings}
          />
        ) : streaming ? (
          <div className="message assistant streaming">
            <div className="message-role">
              Agent
              <span className="thinking-label">
                <span className="thinking-dot" /><span className="thinking-dot" /><span className="thinking-dot" />
              </span>
            </div>
            <div className="message-content">
              {stripArtifact(streamingContent) || '\u00A0'}
            </div>
          </div>
        ) : null}
        <div ref={messagesEndRef} />
      </div>

      <form className="chat-input" onSubmit={handleSubmit}>
        <textarea
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Type your message..."
          disabled={streaming}
          rows={2}
        />
        {streaming ? (
          <button type="button" className="stop-button" onClick={onStop}>
            Stop
          </button>
        ) : (
          <button type="submit" disabled={!input.trim()}>
            Send
          </button>
        )}
      </form>
    </div>
  );
}
