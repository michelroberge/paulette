import { apiFetch, apiStreamUrl } from './client';
import type { Skill, SkillSuggestions, StreamEvent } from '../types';

export function getSkillSuggestions(projectId: string): Promise<SkillSuggestions> {
  return apiFetch<SkillSuggestions>(`/projects/${projectId}/stages/build/skills/suggestions`);
}

export function getObservedSkills(projectId: string): Promise<SkillSuggestions> {
  return apiFetch<SkillSuggestions>(`/projects/${projectId}/stages/build/skills/observed`);
}

export function approveSkills(projectId: string, indices: number[]): Promise<{ count: number; created: Skill[] }> {
  return apiFetch(`/projects/${projectId}/stages/build/skills/approve`, {
    method: 'POST',
    body: JSON.stringify({ indices }),
  });
}

export function listSkills(): Promise<Skill[]> {
  return apiFetch<Skill[]>('/skills');
}

export function getSkill(skillId: string): Promise<{ skill: Skill; promptTemplate: string }> {
  return apiFetch(`/skills/${skillId}`);
}

export async function analyzeSkills(
  projectId: string,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = apiStreamUrl(`/projects/${projectId}/stages/build/skills/analyze`);
  const res = await fetch(url, { method: 'POST', signal });
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
