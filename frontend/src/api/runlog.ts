import { apiFetch } from './client';

export interface ArtifactRef {
  name: string;
  path: string;
}

export interface RunLogEntry {
  id: string;
  projectId: string;
  projectName: string;
  stage: string;
  operation: string;
  attempt: number;
  status: 'running' | 'success' | 'failed';
  startedAt: string;
  endedAt?: string;
  durationMs?: number;
  artifacts?: ArtifactRef[];
  error?: string;
  errorLogPath?: string;
  tokensTotal?: number;
  notes?: string;
}

export function getProjectRunLog(projectId: string): Promise<RunLogEntry[]> {
  return apiFetch<RunLogEntry[]>(`/projects/${projectId}/run-log`);
}

export function getAllRunLog(): Promise<RunLogEntry[]> {
  return apiFetch<RunLogEntry[]>(`/run-log`);
}
