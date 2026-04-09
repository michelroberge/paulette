import { useState, useCallback, useRef } from 'react';
import { startRefinementLoop, answerQuestion, getRefinementState, resetRefinementState, discardReviewItem, addressReviewItem } from '../api/refinement';
import type { StreamEvent } from '../types';
import type { RefinementEvent, RefinementQuestion, RefinementState, ReviewItem, LoopPhase, SectionName } from '../types/refinement';

export interface LogEntry {
  phase: LoopPhase;
  message: string;
  question?: RefinementQuestion;
  /** User's answer to a question — present only on 'user_answer' pseudo-entries. */
  userAnswer?: string;
  timestamp: number;
}

export interface UseRefinementLoopResult {
  phase: LoopPhase;
  iteration: number;
  maxIter: number;
  confidence: Partial<Record<SectionName, number>>;
  currentQuestion: RefinementQuestion | null;
  message: string;
  isRunning: boolean;
  error: string | null;
  artifactUpdated: number;
  knownFacts: string[];
  state: RefinementState | null;
  log: LogEntry[];
  reviewItems: ReviewItem[];
  pendingReviewCount: number;
  start: (message: string) => void;
  answer: (text: string) => Promise<void>;
  reset: () => Promise<void>;
  loadState: () => Promise<void>;
  discardReview: (itemId: string) => Promise<void>;
  addressReview: (itemId: string, response: string) => Promise<void>;
}

export function useRefinementLoop(
  projectId: string | null,
  onTokens?: (n: number) => void,
): UseRefinementLoopResult {
  const [phase, setPhase] = useState<LoopPhase>('init');
  const [iteration, setIteration] = useState(0);
  const [maxIter, setMaxIter] = useState(10);
  const [confidence, setConfidence] = useState<Partial<Record<SectionName, number>>>({});
  const [currentQuestion, setCurrentQuestion] = useState<RefinementQuestion | null>(null);
  const [message, setMessage] = useState('');
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [artifactUpdated, setArtifactUpdated] = useState(0);
  const [knownFacts, setKnownFacts] = useState<string[]>([]);
  const [state, setState] = useState<RefinementState | null>(null);
  const [log, setLog] = useState<LogEntry[]>([]);
  const [reviewItems, setReviewItems] = useState<ReviewItem[]>([]);
  const abortRef = useRef<AbortController | null>(null);
  const onTokensRef = useRef(onTokens);
  onTokensRef.current = onTokens;

  const handleEvent = useCallback((event: StreamEvent) => {
    switch (event.type) {
      case 'refinement': {
        try {
          const data: RefinementEvent = JSON.parse(event.content);
          setPhase(data.phase);
          setIteration(data.iteration);
          setMaxIter(data.maxIter);
          if (data.confidence) setConfidence(data.confidence);
          if (data.question) setCurrentQuestion(data.question);
          else setCurrentQuestion(null);
          // Collect review items from SSE
          if (data.reviewItem) {
            setReviewItems(prev => {
              if (prev.some(r => r.id === data.reviewItem!.id)) return prev;
              return [...prev, data.reviewItem!];
            });
          }
          // Always log questions, even when message is absent
          if (data.message || data.question) {
            const msg = data.message || data.question?.text || '';
            setMessage(msg);
            setLog(prev => [...prev, {
              phase: data.phase,
              message: msg,
              question: data.question,
              timestamp: Date.now(),
            }]);
          }
        } catch { /* ignore parse errors */ }
        break;
      }
      case 'artifact':
        setArtifactUpdated(prev => prev + 1);
        break;
      case 'tokens': {
        const n = parseInt(event.content, 10);
        if (n > 0) onTokensRef.current?.(n);
        break;
      }
      case 'done':
        setIsRunning(false);
        setPhase('complete');
        break;
      case 'error':
        setError(event.content);
        setIsRunning(false);
        break;
    }
  }, []);

  const start = useCallback((msg: string) => {
    if (!projectId || isRunning) return;

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setIsRunning(true);
    setError(null);
    setPhase('init');
    setMessage('Starting refinement loop...');
    setConfidence({});
    setCurrentQuestion(null);
    setKnownFacts([]);
    setReviewItems([]);
    setLog([{
      phase: 'init' as LoopPhase,
      message: msg,
      userAnswer: msg,
      timestamp: Date.now(),
    }]);

    startRefinementLoop(projectId, msg, handleEvent, controller.signal).catch(err => {
      if (err instanceof Error && err.name !== 'AbortError') {
        setError(err.message);
      }
      setIsRunning(false);
    });
  }, [projectId, isRunning, handleEvent]);

  const answerFn = useCallback(async (text: string) => {
    if (!projectId) return;
    try {
      // Persist the answer in the log before sending
      setLog(prev => [...prev, {
        phase: 'await_answer' as LoopPhase,
        message: text,
        userAnswer: text,
        timestamp: Date.now(),
      }]);
      await answerQuestion(projectId, text);
      setCurrentQuestion(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to send answer');
    }
  }, [projectId]);

  const resetFn = useCallback(async () => {
    if (!projectId) return;
    abortRef.current?.abort();
    try {
      await resetRefinementState(projectId);
      setPhase('init');
      setIteration(0);
      setConfidence({});
      setCurrentQuestion(null);
      setMessage('');
      setError(null);
      setKnownFacts([]);
      setReviewItems([]);
      setLog([]);
      setState(null);
      setIsRunning(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset');
    }
  }, [projectId]);

  const loadState = useCallback(async () => {
    if (!projectId) return;
    try {
      const s = await getRefinementState(projectId);
      setState(s);
      setPhase(s.phase);
      setIteration(s.iteration);
      setMaxIter(s.max_iterations);
      setConfidence(s.confidence);
      setKnownFacts(s.known_facts || []);
      setReviewItems(s.review_items || []);
      if (s.error) setError(s.error);

      // Reconstruct conversation log from persisted state (only if not already populated by a live stream)
      setLog(prev => {
        if (prev.length > 0) return prev; // live stream already populating
        const restored: LogEntry[] = [];
        if (s.idea_raw) {
          restored.push({ phase: 'init', message: s.idea_raw, userAnswer: s.idea_raw, timestamp: 0 });
        }
        for (const q of s.open_questions || []) {
          restored.push({ phase: 'await_answer', message: q.text, question: q, timestamp: 0 });
          if (q.answered && q.answer) {
            restored.push({ phase: 'await_answer', message: q.answer, userAnswer: q.answer, timestamp: 0 });
          }
        }
        return restored;
      });
    } catch {
      // No state yet — that's fine
    }
  }, [projectId]);

  const discardReviewFn = useCallback(async (itemId: string) => {
    if (!projectId) return;
    // Optimistic UI update
    setReviewItems(prev => prev.map(r => r.id === itemId ? { ...r, status: 'discarded' as const } : r));
    try {
      await discardReviewItem(projectId, itemId);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to discard review item');
    }
  }, [projectId]);

  const addressReviewFn = useCallback(async (itemId: string, response: string) => {
    if (!projectId) return;
    const item = reviewItems.find(r => r.id === itemId);
    // Optimistic UI update
    setReviewItems(prev => prev.map(r => r.id === itemId ? { ...r, status: 'addressed' as const } : r));
    // Log the concern + user response in the guided chat
    if (item) {
      setLog(prev => [
        ...prev,
        { phase: 'critique' as LoopPhase, message: `[${item.source}] ${item.text}`, timestamp: Date.now() },
        { phase: 'await_answer' as LoopPhase, message: response, userAnswer: response, timestamp: Date.now() },
      ]);
    }
    try {
      const result = await addressReviewItem(projectId, itemId, response);
      // Log the AI update summary and signal artifact change
      setLog(prev => [...prev, {
        phase: 'complete' as LoopPhase,
        message: result.summary,
        timestamp: Date.now(),
      }]);
      setArtifactUpdated(prev => prev + 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to address review item');
    }
  }, [projectId, reviewItems]);

  const pendingItems = reviewItems.filter(r => r.status === 'pending');

  return {
    phase, iteration, maxIter, confidence, currentQuestion, message,
    isRunning, error, artifactUpdated, knownFacts, state, log,
    reviewItems, pendingReviewCount: pendingItems.length,
    start, answer: answerFn, reset: resetFn, loadState,
    discardReview: discardReviewFn, addressReview: addressReviewFn,
  };
}
