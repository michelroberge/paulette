import { apiFetch, apiStreamUrl } from './client';
import type { PipelineState, StreamEvent } from '../types';

export function getPipeline(projectId: string): Promise<PipelineState> {
  return apiFetch<PipelineState>(`/projects/${projectId}/pipeline`);
}

export function approveStage(projectId: string): Promise<{ previousStage: string; currentStage: string }> {
  return apiFetch(`/projects/${projectId}/pipeline/approve`, { method: 'POST' });
}

export function resetStage(projectId: string, stage: string): Promise<PipelineState> {
  return apiFetch<PipelineState>(`/projects/${projectId}/stages/${stage}/reset`, { method: 'POST' });
}

export function regenerateSummary(projectId: string): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/pipeline/summary`, { method: 'POST' });
}

export function getSummary(projectId: string): Promise<{ content: string; exists: boolean }> {
  return apiFetch<{ content: string; exists: boolean }>(`/projects/${projectId}/pipeline/summary`);
}

export function approveSummary(projectId: string): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/pipeline/summary/approve`, { method: 'POST' });
}

export async function watchPipeline(
  projectId: string,
  onState: (state: PipelineState) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/pipeline/watch`);
  const res = await fetch(url, { signal });
  if (!res.ok) return;

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
            try { onState(JSON.parse(data) as PipelineState); } catch { /* skip */ }
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

export async function watchSummary(
  projectId: string,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/pipeline/summary/watch`);
  const res = await fetch(url, { signal });
  if (!res.ok) return;

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
