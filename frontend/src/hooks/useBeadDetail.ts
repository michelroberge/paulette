import { useState, useCallback, useRef } from 'react';
import { getBeadDetail, updateBead, controlBead, sendBeadChat } from '../api/beads';
import type { BeadDetail, Message, StreamEvent } from '../types';

export function useBeadDetail(projectId: string) {
  const [detail, setDetail] = useState<BeadDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [chatStreaming, setChatStreaming] = useState(false);
  const [chatStreamingContent, setChatStreamingContent] = useState('');
  const abortRef = useRef<AbortController | null>(null);

  const loadDetail = useCallback(async (beadId: string) => {
    setLoading(true);
    try {
      const d = await getBeadDetail(projectId, beadId);
      setDetail(d);
    } catch (err) {
      console.error('Failed to load bead detail:', err);
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  const saveDetail = useCallback(async (beadId: string, updates: { description?: string; notes?: string }) => {
    await updateBead(projectId, beadId, updates);
    // Update local state
    setDetail(prev => prev ? { ...prev, ...updates } : prev);
  }, [projectId]);

  const sendChat = useCallback(async (beadId: string, message: string) => {
    if (chatStreaming) return;
    setChatStreaming(true);
    setChatStreamingContent('');

    // Optimistically add user message
    const userMsg: Message = { role: 'user', content: message, timestamp: new Date().toISOString() };
    setDetail(prev => prev ? { ...prev, chatMessages: [...prev.chatMessages, userMsg] } : prev);

    const controller = new AbortController();
    abortRef.current = controller;

    let fullContent = '';

    try {
      await sendBeadChat(
        projectId,
        beadId,
        message,
        (event: StreamEvent) => {
          if (event.type === 'chunk') {
            fullContent += event.content;
            setChatStreamingContent(fullContent);
          } else if (event.type === 'done') {
            fullContent = event.content;
            const assistantMsg: Message = { role: 'assistant', content: fullContent, timestamp: new Date().toISOString() };
            setDetail(prev => prev ? { ...prev, chatMessages: [...prev.chatMessages, assistantMsg] } : prev);
            setChatStreamingContent('');
          }
        },
        controller.signal,
      );
    } catch (err) {
      if (err instanceof Error && err.name !== 'AbortError') {
        console.error('Bead chat error:', err);
      }
    } finally {
      setChatStreaming(false);
      abortRef.current = null;
    }
  }, [projectId, chatStreaming]);

  const stopChat = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setChatStreaming(false);
  }, []);

  const doControl = useCallback(async (beadId: string, action: 'pause' | 'restart') => {
    await controlBead(projectId, beadId, action);
    // Reload detail to reflect new status
    await loadDetail(beadId);
  }, [projectId, loadDetail]);

  return { detail, loading, chatStreaming, chatStreamingContent, loadDetail, saveDetail, sendChat, stopChat, control: doControl };
}
