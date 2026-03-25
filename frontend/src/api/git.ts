import { apiFetch } from './client';
import type { CommitEntry, GitStatus, Project, PipelineState } from '../types';

export function getGitLog(projectId: string, limit = 10, offset = 0, branch?: string): Promise<CommitEntry[]> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (branch) params.set('branch', branch);
  return apiFetch<CommitEntry[]>(`/projects/${projectId}/git/log?${params}`);
}

export interface BranchList { branches: string[]; current: string; }
export function getBranches(projectId: string): Promise<BranchList> {
  return apiFetch<BranchList>(`/projects/${projectId}/git/branches`);
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

export function renameBranch(projectId: string, name: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/branch/rename`, {
    method: 'POST',
    body: JSON.stringify({ name }),
  });
}

export function setIdentity(projectId: string, name: string, email: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/identity`, {
    method: 'POST',
    body: JSON.stringify({ name, email }),
  });
}

export function commitAll(projectId: string, message: string): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/commit`, {
    method: 'POST',
    body: JSON.stringify({ message }),
  });
}

export function push(projectId: string, localBranch?: string, remoteBranch?: string, force?: boolean): Promise<void> {
  return apiFetch(`/projects/${projectId}/git/push`, {
    method: 'POST',
    body: JSON.stringify({ localBranch: localBranch ?? '', remoteBranch: remoteBranch ?? '', force: !!force }),
  });
}

export function getSSHKey(projectId: string): Promise<{ publicKey: string }> {
  return apiFetch(`/projects/${projectId}/git/ssh-key`);
}

export interface WorkingDiffEntry {
  path: string;
  original: string;
  modified: string;
  language: string;
}

export function getWorkingDiff(projectId: string): Promise<WorkingDiffEntry[]> {
  return apiFetch<WorkingDiffEntry[]>(`/projects/${projectId}/git/diff`);
}

export function pull(projectId: string): Promise<{ project: Project; pipeline: PipelineState }> {
  return apiFetch(`/projects/${projectId}/git/pull`, { method: 'POST' });
}
