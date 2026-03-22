import { apiFetch, apiStreamUrl } from './client';
import type { Project } from '../types';

export interface ImportRequest {
  repoUrl?: string;
  hostDir: string;
  name?: string;
  author?: string;
  version?: string;
}

export function importProject(data: ImportRequest): Promise<Project> {
  return apiFetch<Project>('/projects/import', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export function importWatchUrl(projectId: string): string {
  return apiStreamUrl(`/projects/${projectId}/import/watch`);
}
