import { apiFetch } from './client';
import type { CommitEntry, GitStatus, Project, PipelineState } from '../types';

export function getGitLog(projectId: string, limit = 50): Promise<CommitEntry[]> {
  return apiFetch<CommitEntry[]>(`/projects/${projectId}/git/log?limit=${limit}`);
}

export function getGitStatus(projectId: string): Promise<GitStatus> {
  return apiFetch<GitStatus>(`/projects/${projectId}/git/status`);
}

export function resetToCommit(projectId: string, ref: string): Promise<{ project: Project; pipeline: PipelineState }> {
  return apiFetch(`/projects/${projectId}/git/reset`, {
    method: 'POST',
    body: JSON.stringify({ ref }),
  });
}

export function discardChanges(projectId: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/discard`, { method: 'POST' });
}

export function setRemote(projectId: string, url: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/remote`, {
    method: 'PUT',
    body: JSON.stringify({ url }),
  });
}

export function removeRemote(projectId: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/remote`, { method: 'DELETE' });
}

export function push(projectId: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/push`, { method: 'POST' });
}

export function pull(projectId: string): Promise<{ project: Project; pipeline: PipelineState }> {
  return apiFetch(`/projects/${projectId}/git/pull`, { method: 'POST' });
}
