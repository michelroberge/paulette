import { apiFetch, apiStreamUrl } from './client';
import type { BeadGraph, BuildStreamEvent } from '../types';

export function getBeadGraph(projectId: string): Promise<BeadGraph> {
  return apiFetch<BeadGraph>(`/projects/${projectId}/stages/build/beads`);
}

async function streamBeads(
  url: string,
  body: object,
  onEvent: (event: BuildStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }

  const reader = res.body?.getReader();
  if (!reader) throw new Error('No response body');

  const decoder = new TextDecoder();
  let buffer = '';

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6).trim();
          if (data) {
            try {
              onEvent(JSON.parse(data));
            } catch {
              // skip malformed
            }
          }
        }
      }
    }
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') return;
    throw err;
  } finally {
    reader.cancel();
  }
}

/** Subscribe to the long-lived bead watch SSE stream. Calls onEvent for status changes. */
export function watchBeads(
  projectId: string,
  onEvent: (event: BuildStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const url = apiStreamUrl(`/projects/${projectId}/stages/build/beads/watch`);
    fetch(url, { signal })
      .then(async (res) => {
        if (!res.ok) {
          reject(new Error(await res.text() || res.statusText));
          return;
        }
        const reader = res.body?.getReader();
        if (!reader) { resolve(); return; }
        const decoder = new TextDecoder();
        let buffer = '';
        try {
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';
            for (const line of lines) {
              if (line.startsWith('data: ')) {
                const data = line.slice(6).trim();
                if (data) {
                  try { onEvent(JSON.parse(data)); } catch { /* skip */ }
                }
              }
            }
          }
        } catch (err) {
          if (err instanceof Error && err.name === 'AbortError') { resolve(); return; }
          reject(err);
          return;
        } finally {
          reader.cancel();
        }
        resolve();
      })
      .catch((err) => {
        if (err instanceof Error && err.name === 'AbortError') resolve();
        else reject(err);
      });
  });
}

export function generateBeads(
  projectId: string,
  onEvent: (event: BuildStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamBeads(
    apiStreamUrl(`/projects/${projectId}/stages/build/beads/generate`),
    {},
    onEvent,
    signal,
  );
}

export function executeBeads(
  projectId: string,
  maxParallel: number,
  onEvent: (event: BuildStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamBeads(
    apiStreamUrl(`/projects/${projectId}/stages/build/beads/execute`),
    { maxParallel },
    onEvent,
    signal,
  );
}
