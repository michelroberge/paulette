import { useState, useCallback, useRef, useEffect } from 'react';
import { getBeadGraph, generateBeads, executeBeads, watchBeads } from '../api/beads';
import { getActiveRuns, reconnectToRun } from '../api/activity';
import type { Bead, BeadGraph, BuildStreamEvent } from '../types';

export type BuildPhase = 'plan' | 'generating' | 'graph' | 'executing' | 'done';

export function useBeads(projectId: string | null) {
  const [graph, setGraph] = useState<BeadGraph>({ generatedAt: '', projectId: '', beads: [] });
  const [phase, setPhase] = useState<BuildPhase>('plan');
  const [loading, setLoading] = useState(false);
  const [executionLog, setExecutionLog] = useState<string[]>([]);
  const [streamingText, setStreamingText] = useState('');
  const [planLimitReached, setPlanLimitReached] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const loadGraph = useCallback(async () => {
    if (!projectId) return;
    setLoading(true);
    try {
      const g = await getBeadGraph(projectId);
      setGraph(g);
      if (g.beads.length > 0) {
        const allDone = g.beads.every(b => b.status === 'closed');
        setPhase(allDone ? 'done' : 'graph');
      }
    } catch {
      // graph doesn't exist yet
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setPhase(prev => prev === 'generating' || prev === 'executing' ? 'graph' : prev);
  }, []);

  const upsertBead = (beads: Bead[], incoming: Bead): Bead[] => {
    const idx = beads.findIndex(b => b.id === incoming.id);
    if (idx >= 0) {
      const next = [...beads];
      next[idx] = incoming;
      return next;
    }
    return [...beads, incoming];
  };

  const handleEvent = useCallback((event: BuildStreamEvent, mode: 'generate' | 'execute') => {
    switch (event.type) {
      case 'bead_created': {
        try {
          const bead: Bead = JSON.parse(event.content);
          setGraph(g => ({ ...g, beads: upsertBead(g.beads, bead) }));
        } catch { /* ignore */ }
        break;
      }
      case 'bead_update': {
        try {
          const bead: Bead = JSON.parse(event.content);
          setGraph(g => ({ ...g, beads: upsertBead(g.beads, bead) }));
        } catch { /* ignore */ }
        break;
      }
      case 'chunk':
        setStreamingText(t => t + event.content);
        break;
      case 'log':
        setExecutionLog(l => [...l, event.content]);
        break;
      case 'done':
        if (mode === 'generate') {
          setPhase('graph');
        } else {
          setPhase('done');
        }
        break;
      case 'plan_limit':
        setPlanLimitReached(true);
        setPhase(prev => prev === 'generating' || prev === 'executing' ? 'graph' : prev);
        break;
      case 'error':
        setExecutionLog(l => [...l, `ERROR: ${event.content}`]);
        break;
    }
  }, []);

  // Subscribe to the bead watch SSE stream when graph is visible and no active operation
  useEffect(() => {
    if (!projectId) return;
    if (phase !== 'graph' && phase !== 'done') return;

    const controller = new AbortController();

    watchBeads(
      projectId,
      (event) => {
        if (event.type === 'bead_update') {
          try {
            const bead: Bead = JSON.parse(event.content);
            setGraph(g => ({ ...g, beads: upsertBead(g.beads, bead) }));
            // If all beads are now closed, transition to done
            setGraph(g => {
              const allDone = g.beads.length > 0 && g.beads.every(b => b.status === 'closed');
              if (allDone) setPhase('done');
              return g;
            });
          } catch { /* ignore */ }
        }
      },
      controller.signal,
    ).catch(() => { /* connection closed or aborted */ });

    return () => controller.abort();
  }, [projectId, phase]);

  // Check for active bead runs and reconnect on mount
  useEffect(() => {
    if (!projectId) return;

    let cancelled = false;

    (async () => {
      try {
        const runs = await getActiveRuns(projectId);
        if (cancelled) return;

        const genRun = runs.find(r => r.operation === 'beads-generate');
        const execRun = runs.find(r => r.operation === 'beads-execute');

        if (genRun) {
          setPhase('generating');
          setGraph({ generatedAt: '', projectId: projectId, beads: [] });
          setExecutionLog([]);

          const controller = new AbortController();
          abortRef.current = controller;

          await reconnectToRun(
            projectId,
            genRun.id,
            (event) => handleEvent(event as BuildStreamEvent, 'generate'),
            controller.signal,
          );

          if (!cancelled) {
            // Reload from disk to pick up dependency links
            await loadGraph();
          }
        } else if (execRun) {
          setPhase('executing');
          setExecutionLog([]);

          // Load current graph state first
          await loadGraph();

          const controller = new AbortController();
          abortRef.current = controller;

          await reconnectToRun(
            projectId,
            execRun.id,
            (event) => handleEvent(event as BuildStreamEvent, 'execute'),
            controller.signal,
          );
        }
      } catch {
        // not critical
      } finally {
        if (!cancelled) abortRef.current = null;
      }
    })();

    return () => { cancelled = true; };
  }, [projectId]); // eslint-disable-line react-hooks/exhaustive-deps

  const generate = useCallback(async () => {
    if (!projectId || phase === 'generating') return;
    setPhase('generating');
    setGraph({ generatedAt: '', projectId, beads: [] });
    setExecutionLog([]);
    setStreamingText('');
    setPlanLimitReached(false);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await generateBeads(
        projectId,
        (event: BuildStreamEvent) => handleEvent(event, 'generate'),
        controller.signal,
      );
      // Reload from disk to pick up dependency links set after streaming
      await loadGraph();
    } catch (err) {
      if (err instanceof Error && err.name !== 'AbortError') {
        setExecutionLog(l => [...l, `ERROR: ${err.message}`]);
        setPhase('plan');
      }
    } finally {
      abortRef.current = null;
    }
  }, [projectId, phase, handleEvent, loadGraph]);

  const execute = useCallback(async (maxParallel: number) => {
    if (!projectId || phase === 'executing') return;
    setPhase('executing');
    setExecutionLog([]);
    setStreamingText('');
    setPlanLimitReached(false);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await executeBeads(
        projectId,
        maxParallel,
        (event: BuildStreamEvent) => handleEvent(event, 'execute'),
        controller.signal,
      );
    } catch (err) {
      if (err instanceof Error && err.name !== 'AbortError') {
        setExecutionLog(l => [...l, `ERROR: ${err.message}`]);
        setPhase('graph');
      }
    } finally {
      abortRef.current = null;
    }
  }, [projectId, phase, handleEvent]);

  const clearLog = useCallback(() => setExecutionLog([]), []);
  const clearPlanLimit = useCallback(() => setPlanLimitReached(false), []);

  return { graph, phase, loading, executionLog, streamingText, planLimitReached, loadGraph, generate, execute, stop, clearLog, clearPlanLimit };
}
