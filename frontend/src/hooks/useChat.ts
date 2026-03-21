import { useState, useCallback } from 'react';
import { getChatHistory, sendMessage } from '../api/chat';
import type { Message, StageName, StreamEvent } from '../types';

export function useChat(projectId: string | null, stage: StageName | null) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [streamingContent, setStreamingContent] = useState('');
  const [artifactUpdated, setArtifactUpdated] = useState(0);
  const [historyLoaded, setHistoryLoaded] = useState(false);

  const loadHistory = useCallback(async () => {
    if (!projectId || !stage) return;
    setHistoryLoaded(false);
    setMessages([]);
    const { messages } = await getChatHistory(projectId, stage);
    setMessages(messages);
    setHistoryLoaded(true);
    return messages;
  }, [projectId, stage]);

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

    let fullContent = '';

    await sendMessage(projectId, stage, message, (event: StreamEvent) => {
      switch (event.type) {
        case 'chunk':
          fullContent += event.content;
          setStreamingContent(fullContent);
          break;
        case 'artifact':
          setArtifactUpdated(prev => prev + 1);
          break;
        case 'done':
          // Add completed assistant message
          setMessages(prev => [
            ...prev,
            {
              role: 'assistant',
              content: fullContent,
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
    });
  }, [projectId, stage, streaming]);

  return { messages, streaming, streamingContent, artifactUpdated, historyLoaded, loadHistory, send };
}
