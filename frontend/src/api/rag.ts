import { apiFetch } from './client';

export interface RAGStatus {
  enabled: boolean;
  healthy: boolean;
  baseUrl: string;
}

export function getRagStatus(): Promise<RAGStatus> {
  return apiFetch<RAGStatus>('/rag/status');
}
