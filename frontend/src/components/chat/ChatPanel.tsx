import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Message } from '../../types';

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
}

export function ChatPanel({ messages, streaming, streamingContent, onSend, onStop }: Props) {
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
        {streaming && (
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
        )}
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
