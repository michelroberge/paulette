import { apiStreamUrl, apiFetch } from './client';
import type { StreamEvent } from '../types';
import type { RefinementState, ReviewItem } from '../types/refinement';

async function streamSse(
  url: string,
  body: BodyInit,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
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
              onEvent(JSON.parse(data) as StreamEvent);
            } catch {
              // skip malformed JSON
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

export function startRefinementLoop(
  projectId: string,
  message: string,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/stages/vision/loop`);
  return streamSse(url, JSON.stringify({ message }), onEvent, signal);
}

export function answerQuestion(
  projectId: string,
  answer: string,
): Promise<{ status: string }> {
  return apiFetch(`/projects/${projectId}/stages/vision/loop/answer`, {
    method: 'POST',
    body: JSON.stringify({ answer }),
  });
}

export function getRefinementState(
  projectId: string,
): Promise<RefinementState> {
  return apiFetch<RefinementState>(`/projects/${projectId}/stages/vision/loop/state`);
}

export function resetRefinementState(
  projectId: string,
): Promise<{ status: string }> {
  return apiFetch(`/projects/${projectId}/stages/vision/loop/state`, {
    method: 'DELETE',
  });
}

export function getReviewItems(
  projectId: string,
): Promise<ReviewItem[]> {
  return apiFetch<ReviewItem[]>(`/projects/${projectId}/stages/vision/loop/review`);
}

export function discardReviewItem(
  projectId: string,
  itemId: string,
): Promise<{ status: string }> {
  return apiFetch(`/projects/${projectId}/stages/vision/loop/review/${itemId}/discard`, {
    method: 'POST',
  });
}

export function addressReviewItem(
  projectId: string,
  itemId: string,
  response: string,
): Promise<{ status: string; summary: string }> {
  return apiFetch(`/projects/${projectId}/stages/vision/loop/review/${itemId}/address`, {
    method: 'POST',
    body: JSON.stringify({ response }),
  });
}
