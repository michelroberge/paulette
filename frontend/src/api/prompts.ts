import { apiFetch } from './client';

export interface PromptMeta {
  name: string;
  category: string;
  description: string;
  variables: string[];
}

export interface PromptDetail extends PromptMeta {
  content: string;
}

export function initPrompts(projectId: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/prompts/init`, { method: 'POST' });
}

export function listPrompts(projectId: string): Promise<PromptMeta[]> {
  return apiFetch<PromptMeta[]>(`/projects/${projectId}/prompts`);
}

export function getPrompt(projectId: string, name: string): Promise<PromptDetail> {
  return apiFetch<PromptDetail>(`/projects/${projectId}/prompts/${encodeURIComponent(name)}`);
}

export function updatePrompt(projectId: string, name: string, content: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/prompts/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  });
}

export function resetPrompt(projectId: string, name: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/prompts/reset/${encodeURIComponent(name)}`, {
    method: 'POST',
  });
}
