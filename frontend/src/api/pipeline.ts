import { apiFetch } from './client';
import type { PipelineState } from '../types';

export function getPipeline(projectId: string): Promise<PipelineState> {
  return apiFetch<PipelineState>(`/projects/${projectId}/pipeline`);
}

export function approveStage(projectId: string): Promise<{ previousStage: string; currentStage: string }> {
  return apiFetch(`/projects/${projectId}/pipeline/approve`, { method: 'POST' });
}

export function resetStage(projectId: string, stage: string): Promise<PipelineState> {
  return apiFetch<PipelineState>(`/projects/${projectId}/stages/${stage}/reset`, { method: 'POST' });
}
