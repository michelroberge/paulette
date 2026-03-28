import { apiFetch } from './client';
import type { VersionListEntry, VersionSnapshot, VersionBeadDetail } from '../types';

export function listVersions(projectId: string): Promise<VersionListEntry[]> {
  return apiFetch<VersionListEntry[]>(`/projects/${projectId}/versions`);
}

export function getVersionSnapshot(projectId: string, version: string): Promise<VersionSnapshot> {
  return apiFetch<VersionSnapshot>(`/projects/${projectId}/versions/${version}`);
}

export function getVersionBeadExecution(projectId: string, version: string, beadId: string): Promise<VersionBeadDetail> {
  return apiFetch<VersionBeadDetail>(`/projects/${projectId}/versions/${version}/beads/${beadId}`);
}

export function getVersionMockUrl(projectId: string, version: string): string {
  return `/api/projects/${projectId}/versions/${version}/mock`;
}
