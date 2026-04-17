import { apiFetch } from './client';

export interface ArtifactRef {
  name: string;
  path: string;
}

export interface StepLogRef {
  step: string;
  file: string;
  status: string;
  detail?: string;
  durationMs?: number;
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
  stepLogs?: StepLogRef[];
}

export function getProjectRunLog(projectId: string): Promise<RunLogEntry[]> {
  return apiFetch<RunLogEntry[]>(`/projects/${projectId}/run-log`);
}

export function getAllRunLog(): Promise<RunLogEntry[]> {
  return apiFetch<RunLogEntry[]>(`/run-log`);
}

export async function getTraceFile(projectId: string, runId: string, filename: string): Promise<string> {
  const res = await fetch(`/api/projects/${projectId}/run-log/${runId}/trace/${encodeURIComponent(filename)}`, {
    credentials: 'include',
  });
  if (!res.ok) throw new Error(await res.text() || res.statusText);
  return res.text();
}

export function deleteRun(projectId: string, runId: string): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/run-log/${runId}`, { method: 'DELETE' });
}

export function pruneRuns(projectId: string): Promise<{ deleted: number }> {
  return apiFetch<{ deleted: number }>(`/projects/${projectId}/run-log/prune`, { method: 'POST' });
}
