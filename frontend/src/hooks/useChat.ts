import { useState, useCallback, useRef, useEffect } from 'react';
import { getChatHistory, sendMessage, resumeChat } from '../api/chat';
import { getActiveRuns, reconnectToRun } from '../api/activity';
import type { Message, StageName, StreamEvent } from '../types';
import type { ConnectionError } from '../types/provider';
import { parseConnectionError } from '../components/chat/ConnectionErrorBanner';

export function useChat(projectId: string | null, stage: StageName | null, reloadTrigger?: number, onTokens?: (n: number) => void) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [streamingContent, setStreamingContent] = useState('');
  const [artifactUpdated, setArtifactUpdated] = useState(0);
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const [nextTurn, setNextTurn] = useState<'agent' | 'user'>('agent');
  /** Populated when the SSE stream emits a structured connection-failure error. */
  const [connectionError, setConnectionError] = useState<ConnectionError | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const onTokensRef = useRef(onTokens);
  onTokensRef.current = onTokens;

  const handleEvent = useCallback((event: StreamEvent, fullContentRef: { current: string }) => {
    switch (event.type) {
      case 'chunk':
        fullContentRef.current += event.content;
        setStreamingContent(fullContentRef.current);
        break;
      case 'artifact':
        setArtifactUpdated(prev => prev + 1);
        break;
      case 'tokens': {
        const n = parseInt(event.content, 10);
        if (n > 0) onTokensRef.current?.(n);
        break;
      }
      case 'done':
        setMessages(prev => [
          ...prev,
          {
            role: 'assistant',
            content: event.content || fullContentRef.current,
            timestamp: new Date().toISOString(),
          },
        ]);
        setStreamingContent('');
        setStreaming(false);
        break;
      case 'error': {
        const connErr = parseConnectionError(event.content);
        if (connErr) {
          // Structured connection error — surface the ConnectionErrorBanner instead of
          // adding a generic error message to the chat history.  The banner persists
          // until the user sends a new message (which clears it in `send`/`resume`).
          setConnectionError(connErr);
        } else {
          // Generic / non-connection error — fall back to the existing inline message.
          setMessages(prev => [
            ...prev,
            {
              role: 'assistant',
              content: event.content || 'An error occurred. Please try again.',
              timestamp: new Date().toISOString(),
              isError: true,
            },
          ]);
        }
        setStreamingContent('');
        setStreaming(false);
        break;
      }
    }
  }, []);

  const loadHistory = useCallback(async () => {
    if (!projectId || !stage) return;
    setHistoryLoaded(false);
    setMessages([]);
    const history = await getChatHistory(projectId, stage);
    setMessages(history.messages);
    setNextTurn(history.nextTurn ?? 'agent');
    setHistoryLoaded(true);
    return history.messages;
  }, [projectId, stage, reloadTrigger]); // eslint-disable-line react-hooks/exhaustive-deps

  // Check for active runs and reconnect on mount
  useEffect(() => {
    if (!projectId || !stage || stage === 'complete') return;

    let cancelled = false;

    (async () => {
      try {
        const runs = await getActiveRuns(projectId);
        if (cancelled) return;

        const activeRun = runs.find(r => r.stage === stage && r.operation === 'chat');
        if (!activeRun) return;

        // There's an active chat agent — reconnect to its stream
        setStreaming(true);
        setStreamingContent('');

        const controller = new AbortController();
        abortRef.current = controller;
        const fullContentRef = { current: '' };

        await reconnectToRun(
          projectId,
          activeRun.id,
          (event) => handleEvent(event as StreamEvent, fullContentRef),
          controller.signal,
        );
      } catch {
        // Failed to check/reconnect — not critical
      }
    })();

    return () => { cancelled = true; };
  }, [projectId, stage]); // eslint-disable-line react-hooks/exhaustive-deps

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setStreamingContent('');
    setStreaming(false);
  }, []);

  const send = useCallback(async (message: string) => {
    if (!projectId || !stage || streaming) return;

    // Dismiss any pending connection error banner so the user gets a fresh start.
    setConnectionError(null);

    // Add user message immediately
    const userMsg: Message = {
      role: 'user',
      content: message,
      timestamp: new Date().toISOString(),
    };
    setMessages(prev => [...prev, userMsg]);
    setStreaming(true);
    setStreamingContent('');

    const controller = new AbortController();
    abortRef.current = controller;
    const fullContentRef = { current: '' };

    await sendMessage(projectId, stage, message, (event: StreamEvent) => {
      handleEvent(event, fullContentRef);
    }, controller.signal);
  }, [projectId, stage, streaming, handleEvent]);

  // Seed a local-only assistant greeting (not persisted to chat history, not sent to AI).
  // Used for the vision kickoff so the user sees a friendly welcome and can type their first
  // message, rather than having the agent auto-reply to a synthetic user turn.
  const seed = useCallback((content: string) => {
    setMessages(prev => (prev.length === 0
      ? [{ role: 'assistant', content, timestamp: new Date().toISOString() }]
      : prev));
    setNextTurn('user');
  }, []);

  // Resume re-invokes the agent for an unanswered user message (e.g. after server restart).
  const resume = useCallback(async () => {
    if (!projectId || !stage || streaming) return;
    // Clear any connection error banner before attempting a resume.
    setConnectionError(null);
    setStreaming(true);
    setStreamingContent('');

    const controller = new AbortController();
    abortRef.current = controller;
    const fullContentRef = { current: '' };

    await resumeChat(projectId, stage, (event: StreamEvent) => {
      handleEvent(event, fullContentRef);
    }, controller.signal);
  }, [projectId, stage, streaming, handleEvent]);

  return { messages, streaming, streamingContent, artifactUpdated, historyLoaded, nextTurn, connectionError, loadHistory, send, seed, resume, stop };
}
