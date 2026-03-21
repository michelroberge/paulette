import { useState, useCallback } from 'react';
import { getMock, generateMock } from '../api/mock';
import type { StreamEvent } from '../types';

export function useMock(projectId: string | null) {
  const [html, setHtml] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [streamingText, setStreamingText] = useState('');
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    if (!projectId) return;
    const res = await getMock(projectId);
    if (res.exists) setHtml(res.html);
    setLoaded(true);
  }, [projectId]);

  const generate = useCallback(async (refinement = '') => {
    if (!projectId || generating) return;
    setGenerating(true);
    setStreamingText('');

    try {
      let fullText = '';
      await generateMock(projectId, refinement, (event: StreamEvent) => {
        switch (event.type) {
          case 'chunk':
            fullText += event.content;
            setStreamingText(fullText);
            break;
          case 'done':
            setHtml(event.content);
            setStreamingText('');
            break;
          case 'error':
            setStreamingText('');
            break;
        }
      });
    } finally {
      setGenerating(false);
    }
  }, [projectId, generating]);

  return { html, generating, streamingText, loaded, load, generate };
}
