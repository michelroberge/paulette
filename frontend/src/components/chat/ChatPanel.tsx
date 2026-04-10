import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Message, RAGSource } from '../../types';
import { ConnectionErrorBanner } from './ConnectionErrorBanner';
import { RAGSourcesPanel } from './RAGSourcesPanel';
import type { ConnectionError } from '../../types/provider';

const ARTIFACT_RE = /<!--\s*ARTIFACT:START\s*-->[\s\S]*?<!--\s*ARTIFACT:END\s*-->/g;
function stripArtifact(text: string) {
  return text.replace(ARTIFACT_RE, '').trim();
}

const THINKING_PHRASES = [
  'Thinking...', 'Inferring...', 'Contemplating...', 'Pondering...', 'Ruminating...',
  'Hypothesizing...', 'Deliberating...', 'Extrapolating...', 'Synthesizing...', 'Cogitating...',
  'Deducing...', 'Reasoning...', 'Envisioning...', 'Speculating...', 'Calculating...',
  'Mulling it over...', 'Connecting dots...', 'Brewing ideas...', 'Processing...', 'Manifesting...',
  'Brewing coffe...', 'Taking a nap...', 'Enjoying the sun...'
];

function useThinkingPhrase(active: boolean) {
  const [index, setIndex] = useState(0);
  useEffect(() => {
    if (!active) return;
    setIndex(Math.floor(Math.random() * THINKING_PHRASES.length));
    const id = setInterval(() => {
      setIndex(i => (i + 1) % THINKING_PHRASES.length);
    }, 2500);
    return () => clearInterval(id);
  }, [active]);
  return THINKING_PHRASES[index];
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
  /**
   * Retries the last unanswered user message without requiring re-typing.
   * Wired to `resume()` from `useChat`. Shown in the error banner so the user
   * can fix the connection inline (via "Change stage connection") then immediately
   * retry without navigating away or losing their original message.
   * When omitted, the Retry button is not rendered in the banner.
   */
  onRetry?: () => void;
  /** RAG knowledge sources used for the current/last response. */
  ragSources?: RAGSource[];
  /**
   * When provided, shows a "Generate [label]" button above the chat input.
   * Clicking it sends a directive message asking the LLM to produce the artifact.
   * Used for UX/Architecture stages to escape the chat loop.
   */
  onGenerateArtifact?: () => void;
  /** Label for the generate button (e.g. "UX Design", "Architecture"). */
  generateArtifactLabel?: string;
}

export function ChatPanel({ messages, streaming, streamingContent, onSend, onStop, connectionError, onOpenProjectSettings, onRetry, ragSources, onGenerateArtifact, generateArtifactLabel }: Props) {
  const [input, setInput] = useState('');
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const thinkingPhrase = useThinkingPhrase(streaming);

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
        {ragSources && ragSources.length > 0 && (
          <RAGSourcesPanel sources={ragSources} />
        )}
        {/* Connection error banner replaces the streaming area (SCR-012 / JRN-v0.2.0-008).
            The banner persists until the user sends a new message (which clears it in useChat). */}
        {connectionError ? (
          <ConnectionErrorBanner
            connectionName={connectionError.connectionName}
            connectionId={connectionError.connectionId}
            reason={connectionError.reason}
            onOpenProjectSettings={onOpenProjectSettings}
            onRetry={onRetry}
          />
        ) : streaming ? (
          <div className="message assistant streaming">
            <div className="message-role">
              Agent
              <span className="thinking-label">{thinkingPhrase}</span>
            </div>
            <div className="message-content">
              {stripArtifact(streamingContent) || '\u00A0'}
            </div>
          </div>
        ) : null}
        <div ref={messagesEndRef} />
      </div>

      <form className="chat-input" onSubmit={handleSubmit}>
        {onGenerateArtifact && messages.length > 1 && !streaming && (
          <button
            type="button"
            onClick={onGenerateArtifact}
            style={{
              width: '100%', padding: '0.4rem', marginBottom: '0.4rem',
              borderRadius: '4px', fontSize: '0.8rem',
              border: '1px solid #4caf50', background: 'transparent',
              color: '#4caf50', cursor: 'pointer',
            }}
          >Generate {generateArtifactLabel || 'Artifact'}</button>
        )}
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
