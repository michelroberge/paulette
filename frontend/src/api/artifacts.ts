import { apiFetch } from './client';
import type { StageName } from '../types';

interface ArtifactResponse {
  content: string;
  exists: boolean;
}

export function getArtifact(projectId: string, stage: StageName): Promise<ArtifactResponse> {
  return apiFetch<ArtifactResponse>(`/projects/${projectId}/stages/${stage}/artifact`);
}
