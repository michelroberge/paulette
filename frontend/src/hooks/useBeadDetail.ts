import { useState, useCallback, useRef } from 'react';
import { getBeadDetail, updateBead, controlBead, sendBeadChat, sendInstruction, applyInstructionPlan } from '../api/beads';
import type { BeadDetail, BeadGraph, InstructionPlan, Message, StreamEvent } from '../types';

export function useBeadDetail(projectId: string) {
  const [detail, setDetail] = useState<BeadDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [chatStreaming, setChatStreaming] = useState(false);
  const [chatStreamingContent, setChatStreamingContent] = useState('');

  // Instruction mode
  const [instructStreaming, setInstructStreaming] = useState(false);
  const [instructStreamingContent, setInstructStreamingContent] = useState('');
  const [instructionProposal, setInstructionProposal] = useState<InstructionPlan | null>(null);
  const [applyingProposal, setApplyingProposal] = useState(false);

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
    setDetail(prev => prev ? { ...prev, ...updates } : prev);
  }, [projectId]);

  const sendChat = useCallback(async (beadId: string, message: string) => {
    if (chatStreaming) return;
    setChatStreaming(true);
    setChatStreamingContent('');

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
    setInstructStreaming(false);
  }, []);

  const sendInstruct = useCallback(async (message: string) => {
    if (instructStreaming) return;
    setInstructStreaming(true);
    setInstructStreamingContent('');
    setInstructionProposal(null);

    const controller = new AbortController();
    abortRef.current = controller;

    let fullContent = '';

    try {
      await sendInstruction(
        projectId,
        message,
        (event: StreamEvent) => {
          if (event.type === 'chunk') {
            fullContent += event.content;
            setInstructStreamingContent(fullContent);
          } else if (event.type === 'artifact') {
            try {
              const plan: InstructionPlan = JSON.parse(event.content);
              setInstructionProposal(plan);
            } catch {
              console.warn('Failed to parse instruction plan artifact');
            }
          } else if (event.type === 'done') {
            setInstructStreamingContent('');
          }
        },
        controller.signal,
      );
    } catch (err) {
      if (err instanceof Error && err.name !== 'AbortError') {
        console.error('Instruct error:', err);
      }
    } finally {
      setInstructStreaming(false);
      abortRef.current = null;
    }
  }, [projectId, instructStreaming]);

  const applyProposal = useCallback(async (plan: InstructionPlan): Promise<BeadGraph | null> => {
    setApplyingProposal(true);
    try {
      const result = await applyInstructionPlan(projectId, plan);
      setInstructionProposal(null);
      return result.graph;
    } catch (err) {
      console.error('Failed to apply instruction plan:', err);
      return null;
    } finally {
      setApplyingProposal(false);
    }
  }, [projectId]);

  const dismissProposal = useCallback(() => {
    setInstructionProposal(null);
    setInstructStreamingContent('');
  }, []);

  const doControl = useCallback(async (beadId: string, action: 'pause' | 'restart' | 'close') => {
    await controlBead(projectId, beadId, action);
    await loadDetail(beadId);
  }, [projectId, loadDetail]);

  return {
    detail,
    loading,
    chatStreaming,
    chatStreamingContent,
    instructStreaming,
    instructStreamingContent,
    instructionProposal,
    applyingProposal,
    loadDetail,
    saveDetail,
    sendChat,
    stopChat,
    sendInstruct,
    applyProposal,
    dismissProposal,
    control: doControl,
  };
}
