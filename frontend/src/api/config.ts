import { apiFetch } from './client';

export interface AppConfig {
  reposPath: string;
  version: string;
  author: string;
}

export function getConfig(): Promise<AppConfig> {
  return apiFetch<AppConfig>('/config');
}
