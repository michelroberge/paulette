import { apiFetch, apiStreamUrl } from './client';
import type { StreamEvent } from '../types';
import type { Message } from '../types';

export interface RefineRequest {
  selection: string;
  instruction: string;
  conversationHistory?: Message[];
}

export async function refineArtifact(
  projectId: string,
  stage: string,
  req: RefineRequest,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/stages/${stage}/artifact/refine`);
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
    signal,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }

  const reader = res.body?.getReader();
  if (!reader) return;

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
    if (err instanceof Error && err.name === 'AbortError') return;
    throw err;
  } finally {
    reader.cancel();
  }
}

export function manualEditArtifact(
  projectId: string,
  stage: string,
  oldText: string,
  newText: string,
): Promise<{ status: string; content: string }> {
  return apiFetch(`/projects/${projectId}/stages/${stage}/artifact/manual-edit`, {
    method: 'POST',
    body: JSON.stringify({ oldText, newText }),
  });
}

export function applyRefine(
  projectId: string,
  stage: string,
  oldText: string,
  newText: string,
): Promise<{ status: string }> {
  return apiFetch(`/projects/${projectId}/stages/${stage}/artifact/apply-refine`, {
    method: 'POST',
    body: JSON.stringify({ oldText, newText }),
  });
}
