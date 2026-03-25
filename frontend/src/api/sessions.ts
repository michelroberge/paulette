import { apiFetch } from './client';
import type { StageName, SessionSummary } from '../types';

export function getSessions(projectId: string, stage?: StageName): Promise<SessionSummary> {
  const query = stage ? `?stage=${stage}` : '';
  return apiFetch<SessionSummary>(`/projects/${projectId}/sessions${query}`);
}
