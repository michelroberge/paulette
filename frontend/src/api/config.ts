import { apiFetch } from './client';

export interface AppConfig {
  reposPath: string;
}

export function getConfig(): Promise<AppConfig> {
  return apiFetch<AppConfig>('/config');
}
