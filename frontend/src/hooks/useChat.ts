import { useState, useCallback, useRef, useEffect } from 'react';
import { getChatHistory, sendMessage } from '../api/chat';
import { getActiveRuns, reconnectToRun } from '../api/activity';
import type { Message, StageName, StreamEvent } from '../types';

export function useChat(projectId: string | null, stage: StageName | null, reloadTrigger?: number, onTokens?: (n: number) => void) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [streamingContent, setStreamingContent] = useState('');
  const [artifactUpdated, setArtifactUpdated] = useState(0);
  const [historyLoaded, setHistoryLoaded] = useState(false);
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
      case 'error':
        setMessages(prev => [
          ...prev,
          {
            role: 'assistant',
            content: event.content || 'An error occurred. Please try again.',
            timestamp: new Date().toISOString(),
            isError: true,
          },
        ]);
        setStreamingContent('');
        setStreaming(false);
        break;
    }
  }, []);

  const loadHistory = useCallback(async () => {
    if (!projectId || !stage) return;
    setHistoryLoaded(false);
    setMessages([]);
    const { messages } = await getChatHistory(projectId, stage);
    setMessages(messages);
    setHistoryLoaded(true);
    return messages;
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

  return { messages, streaming, streamingContent, artifactUpdated, historyLoaded, loadHistory, send, stop };
}
