import { useState, useCallback, useRef } from 'react';
import { getMock, generateMock } from '../api/mock';
import type { StreamEvent } from '../types';

export function useMock(projectId: string | null) {
  const [html, setHtml] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [tokenCount, setTokenCount] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    if (!projectId) return;
    const res = await getMock(projectId);
    if (res.exists) setHtml(res.html);
    setLoaded(true);
  }, [projectId]);

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setGenerating(false);
  }, []);

  const generate = useCallback(async (refinement = '') => {
    if (!projectId || generating) return;
    setGenerating(true);
    setTokenCount(0);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      let charCount = 0;
      await generateMock(projectId, refinement, (event: StreamEvent) => {
        switch (event.type) {
          case 'chunk':
            charCount += event.content.length;
            setTokenCount(Math.round(charCount / 4));
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

  return { html, generating, tokenCount, loaded, load, generate, stop };
}
