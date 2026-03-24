import { useState, useEffect, useRef } from 'react';
import { reconnectToRun } from '../api/activity';
import type { ActiveRun } from '../api/activity';
import type { StreamEvent } from '../types';

export interface AgentStreamState {
  active: boolean;
  streamingText: string;
  operation: string;
  stage: string;
}

export function useAgentStream(projectId: string | null, activeRuns: ActiveRun[]): AgentStreamState {
  const [active, setActive] = useState(false);
  const [streamingText, setStreamingText] = useState('');
  const [operation, setOperation] = useState('');
  const [stage, setStage] = useState('');
  const abortRef = useRef<AbortController | null>(null);
  const activeRunIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!projectId) return;

    // Find first non-mock active run
    const run = activeRuns.find(r => r.operation !== 'mock');

    if (!run) {
      // No active non-mock run — if we were connected, keep text visible until cleared naturally
      if (activeRunIdRef.current) {
        activeRunIdRef.current = null;
        abortRef.current?.abort();
        abortRef.current = null;
        setActive(false);
      }
      return;
    }

    // Already connected to this run
    if (activeRunIdRef.current === run.id) return;

    // Cancel previous connection
    abortRef.current?.abort();

    activeRunIdRef.current = run.id;
    setActive(true);
    setOperation(run.operation);
    setStage(run.stage);
    setStreamingText('');

    const controller = new AbortController();
    abortRef.current = controller;

    // fromIndex=0 replays all buffered events — handles browser close/reconnect
    reconnectToRun(
      projectId,
      run.id,
      (event) => {
        const ev = event as StreamEvent;
        switch (ev.type) {
          case 'chunk':
            setStreamingText(t => t + ev.content);
            break;
          case 'done':
            setActive(false);
            activeRunIdRef.current = null;
            break;
        }
      },
      controller.signal,
      0,
    ).catch(() => {});

    return () => {
      controller.abort();
    };
  }, [projectId, activeRuns]); // eslint-disable-line react-hooks/exhaustive-deps

  return { active, streamingText, operation, stage };
}
