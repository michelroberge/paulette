import { apiFetch, apiStreamUrl } from './client';
import type { BeadGraph, BeadDetail, BuildStreamEvent, StreamEvent, BeadFilesResponse, BeadDiffResponse, InstructionPlan } from '../types';

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

export function getBeadDetail(projectId: string, beadId: string): Promise<BeadDetail> {
  return apiFetch<BeadDetail>(`/projects/${projectId}/stages/build/beads/${beadId}`);
}

export function updateBead(projectId: string, beadId: string, updates: { description?: string; notes?: string }): Promise<void> {
  return apiFetch(`/projects/${projectId}/stages/build/beads/${beadId}`, {
    method: 'PATCH',
    body: JSON.stringify(updates),
  });
}

export function controlBead(projectId: string, beadId: string, action: 'pause' | 'restart'): Promise<void> {
  return apiFetch(`/projects/${projectId}/stages/build/beads/${beadId}/control`, {
    method: 'POST',
    body: JSON.stringify({ action }),
  });
}

export function sendBeadChat(
  projectId: string,
  beadId: string,
  message: string,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamBeads(
    apiStreamUrl(`/projects/${projectId}/stages/build/beads/${beadId}/chat`),
    { message },
    onEvent as (event: BuildStreamEvent) => void,
    signal,
  );
}

export function getBeadFiles(projectId: string, beadId: string): Promise<BeadFilesResponse> {
  return apiFetch<BeadFilesResponse>(`/projects/${projectId}/stages/build/beads/${beadId}/files`);
}

export function getBeadDiff(projectId: string, beadId: string): Promise<BeadDiffResponse> {
  return apiFetch<BeadDiffResponse>(`/projects/${projectId}/stages/build/beads/${beadId}/diff`);
}

export function sendInstruction(
  projectId: string,
  message: string,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  return streamBeads(
    apiStreamUrl(`/projects/${projectId}/stages/build/beads/instruct`),
    { message },
    onEvent as (event: BuildStreamEvent) => void,
    signal,
  );
}

export function applyInstructionPlan(
  projectId: string,
  plan: InstructionPlan,
): Promise<{ created: { requestedTitle: string; id: string }[]; graph: import('../types').BeadGraph }> {
  return apiFetch(`/projects/${projectId}/stages/build/beads/instruct/apply`, {
    method: 'POST',
    body: JSON.stringify({ plan }),
  });
}
