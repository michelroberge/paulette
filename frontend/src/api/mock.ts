import { apiStreamUrl, apiFetch } from './client';
import type { StreamEvent } from '../types';

interface MockResponse {
  html: string;
  exists: boolean;
}

export function getMock(projectId: string): Promise<MockResponse> {
  return apiFetch<MockResponse>(`/projects/${projectId}/stages/ux/mock`);
}

export async function generateMock(
  projectId: string,
  refinement: string,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/stages/ux/mock`);

  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refinement }),
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
