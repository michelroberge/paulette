import { apiFetch } from './client';
import type { PromptMeta, PromptDetail } from './prompts';

export function initConnectionPrompts(connectionId: string): Promise<void> {
  return apiFetch(`/connections/${connectionId}/prompts/init`, { method: 'POST' });
}

export function listConnectionPrompts(connectionId: string): Promise<PromptMeta[]> {
  return apiFetch<PromptMeta[]>(`/connections/${connectionId}/prompts`);
}

export function getConnectionPrompt(connectionId: string, name: string): Promise<PromptDetail> {
  return apiFetch<PromptDetail>(`/connections/${connectionId}/prompts/${encodeURIComponent(name)}`);
}

export function updateConnectionPrompt(connectionId: string, name: string, content: string): Promise<void> {
  return apiFetch(`/connections/${connectionId}/prompts/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  });
}

export function resetConnectionPrompt(connectionId: string, name: string): Promise<void> {
  return apiFetch(`/connections/${connectionId}/prompts/reset/${encodeURIComponent(name)}`, {
    method: 'POST',
  });
}
