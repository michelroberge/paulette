import { apiFetch } from './client';
import type { Project, PipelineState, VersionBump } from '../types';

interface EnhanceResponse {
  project: Project;
  pipeline: PipelineState;
}

export function startEnhancement(
  projectId: string,
  vision: string,
  versionBump: VersionBump,
): Promise<EnhanceResponse> {
  return apiFetch(`/projects/${projectId}/pipeline/enhance`, {
    method: 'POST',
    body: JSON.stringify({ vision, versionBump }),
  });
}
