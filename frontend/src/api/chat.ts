import { apiStreamUrl, apiFetch } from './client';
import type { ChatHistory, StageName, StreamEvent } from '../types';

export function getChatHistory(projectId: string, stage: StageName): Promise<ChatHistory> {
  return apiFetch<ChatHistory>(`/projects/${projectId}/stages/${stage}/chat`);
}

export async function sendMessage(
  projectId: string,
  stage: StageName,
  message: string,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/stages/${stage}/chat`);

  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
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
              const event: StreamEvent = JSON.parse(data);
              onEvent(event);
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
