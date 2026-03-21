import { useEffect, useRef, useState } from 'react';
import type { Message } from '../../types';

interface Props {
  messages: Message[];
  streaming: boolean;
  streamingContent: string;
  onSend: (message: string) => void;
}

export function ChatPanel({ messages, streaming, streamingContent, onSend }: Props) {
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
            <div className="message-content">{msg.content}</div>
          </div>
        ))}
        {streaming && (
          <div className="message assistant streaming">
            <div className="message-role">Agent</div>
            <div className="message-content">
              {streamingContent || <span className="typing-indicator"><span /><span /><span /></span>}
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
        <button type="submit" disabled={streaming || !input.trim()}>
          {streaming ? '...' : 'Send'}
        </button>
      </form>
    </div>
  );
}
