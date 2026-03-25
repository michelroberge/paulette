import { apiFetch } from './client';
import type { FrameworkConfig } from '../types';

export function getFramework(projectId: string): Promise<FrameworkConfig> {
  return apiFetch<FrameworkConfig>(`/projects/${projectId}/stages/ux/framework`);
}

export function setFramework(projectId: string, config: FrameworkConfig): Promise<FrameworkConfig> {
  return apiFetch<FrameworkConfig>(`/projects/${projectId}/stages/ux/framework`, {
    method: 'PUT',
    body: JSON.stringify(config),
  });
}
