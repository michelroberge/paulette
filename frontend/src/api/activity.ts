import { apiFetch, apiStreamUrl } from './client';
import type { StreamEvent, BuildStreamEvent } from '../types';

export interface ActiveRun {
  id: string;
  projectId: string;
  stage: string;
  operation: string; // "chat", "mock", "beads-generate", "beads-execute"
  startedAt: string;
  eventCount: number;
  done: boolean;
}

export function getActiveRuns(projectId: string): Promise<ActiveRun[]> {
  return apiFetch<ActiveRun[]>(`/projects/${projectId}/activity`);
}

export function getProjectsActivity(): Promise<Record<string, number>> {
  return apiFetch<Record<string, number>>('/projects/activity');
}

export function sendBtw(projectId: string, runId: string, message: string): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/activity/${runId}/btw`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  });
}

export async function reconnectToRun(
  projectId: string,
  runId: string,
  onEvent: (event: StreamEvent | BuildStreamEvent) => void,
  signal?: AbortSignal,
  fromIndex = 0,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/activity/${runId}/stream?from=${fromIndex}`);

  const res = await fetch(url, { signal });

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
