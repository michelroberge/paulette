import { useState, useCallback, useRef, useEffect } from 'react';
import { getMock, generateMock } from '../api/mock';
import { getActiveRuns, reconnectToRun } from '../api/activity';
import type { StreamEvent } from '../types';

export function useMock(projectId: string | null) {
  const [html, setHtml] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [tokenCount, setTokenCount] = useState(0);
  const [streamingText, setStreamingText] = useState('');
  const [loaded, setLoaded] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    if (!projectId) return;
    const res = await getMock(projectId);
    if (res.exists) setHtml(res.html);
    setLoaded(true);
  }, [projectId]);

  // Check for active mock generation and reconnect on mount
  useEffect(() => {
    if (!projectId) return;

    let cancelled = false;

    (async () => {
      try {
        const runs = await getActiveRuns(projectId);
        if (cancelled) return;

        const activeRun = runs.find(r => r.operation === 'mock');
        if (!activeRun) return;

        setGenerating(true);
        setTokenCount(0);
        setStreamingText('');

        const controller = new AbortController();
        abortRef.current = controller;
        let charCount = 0;

        await reconnectToRun(
          projectId,
          activeRun.id,
          (event) => {
            const ev = event as StreamEvent;
            switch (ev.type) {
              case 'chunk':
                charCount += ev.content.length;
                setTokenCount(Math.round(charCount / 4));
                setStreamingText(t => t + ev.content);
                break;
              case 'tokens':
                setTokenCount(parseInt(ev.content, 10));
                break;
              case 'done':
                setHtml(ev.content);
                break;
            }
          },
          controller.signal,
        );
      } catch {
        // not critical
      } finally {
        if (!cancelled) setGenerating(false);
      }
    })();

    return () => { cancelled = true; };
  }, [projectId]); // eslint-disable-line react-hooks/exhaustive-deps

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setGenerating(false);
  }, []);

  const generate = useCallback(async (refinement = '') => {
    if (!projectId || generating) return;
    setGenerating(true);
    setTokenCount(0);
    setStreamingText('');
    setHtml(null);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      let charCount = 0;
      await generateMock(projectId, refinement, (event: StreamEvent) => {
        switch (event.type) {
          case 'chunk':
            charCount += event.content.length;
            setTokenCount(Math.round(charCount / 4));
            setStreamingText(t => t + event.content);
            break;
          case 'tokens':
            setTokenCount(parseInt(event.content, 10));
            break;
          case 'done':
            setHtml(event.content);
            break;
        }
      }, controller.signal);
    } finally {
      setGenerating(false);
    }
  }, [projectId, generating]);

  return { html, generating, tokenCount, streamingText, loaded, load, generate, stop };
}
