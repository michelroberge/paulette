import { apiFetch } from './client';
import type { Project } from '../types';

export function listProjects(): Promise<Project[]> {
  return apiFetch<Project[]>('/projects');
}

export function getProject(id: string): Promise<Project> {
  return apiFetch<Project>(`/projects/${id}`);
}

export function createProject(data: { name: string; author: string; hostDir: string }): Promise<Project> {
  return apiFetch<Project>('/projects', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export function deleteProject(id: string): Promise<void> {
  return apiFetch<void>(`/projects/${id}`, { method: 'DELETE' });
}
