/**
 * API client for stage configuration — global defaults and per-project overrides.
 *
 * Global defaults are stored in ~/.paulette/config.json and apply to all new projects.
 * Per-project overrides are stored in <hostDir>/.paulette/stage_config.json and take
 * precedence over global defaults for a specific project.
 *
 * Journeys: JRN-v0.2.0-006 (Set Global Stage Defaults), JRN-v0.2.0-007 (Per-Project Override)
 * Architecture: ARCH-v0.2.0-030
 */

import type { StageName } from '../types';
import type {
  GlobalStageConfig,
  ProjectStageConfig,
  StageAssignment,
  OperationKey,
  BuildPoolConfig,
  BuildPoolStatus,
} from '../types/provider';
import { apiFetch } from './client';

/**
 * Retrieve the global stage defaults from ~/.paulette/config.json.
 * Absent stage entries fall back to the built-in Claude CLI default at runtime.
 */
export function getGlobalDefaults(): Promise<GlobalStageConfig> {
  return apiFetch<GlobalStageConfig>('/config/stages');
}

/**
 * Set the global default connection and model for a single pipeline stage.
 * The change is written immediately to ~/.paulette/config.json.
 *
 * @param stage  The pipeline stage to configure (e.g. "vision", "build").
 * @param assignment  The connection ID and model to use for this stage.
 */
export function setGlobalStageDefault(
  stage: StageName,
  assignment: StageAssignment,
): Promise<void> {
  return apiFetch<void>(`/config/stages/${stage}`, {
    method: 'PUT',
    body: JSON.stringify(assignment),
  });
}

/**
 * Retrieve the per-project stage overrides for a specific project.
 * Stages not present in `overrides`, or present with a `null` value, inherit the global default.
 *
 * @param projectId  The project UUID.
 */
export function getProjectOverrides(projectId: string): Promise<ProjectStageConfig> {
  return apiFetch<ProjectStageConfig>(`/projects/${projectId}/config/stages`);
}

/**
 * Set or clear the per-project stage override for a single pipeline stage.
 *
 * Pass a `StageAssignment` to override the stage for this project only.
 * Pass `null` to clear the override and revert the stage to the global default (inherit).
 *
 * @param projectId   The project UUID.
 * @param stage       The pipeline stage to configure or reset.
 * @param assignment  The connection + model to use, or `null` to inherit from global defaults.
 */
export function setProjectStageOverride(
  projectId: string,
  stage: StageName,
  assignment: StageAssignment | null,
): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/config/stages/${stage}`, {
    method: 'PUT',
    body: JSON.stringify(assignment),
  });
}

/**
 * Reset all per-project stage overrides for a project, reverting every stage to the
 * global default. Equivalent to toggling every "Inherit" toggle back to ON in the
 * Project Stage Settings panel.
 *
 * @param projectId  The project UUID.
 */
export function resetProjectOverrides(projectId: string): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/config/stages/reset`, {
    method: 'POST',
  });
}

// ---------------------------------------------------------------------------
// Operation-level defaults & overrides
// ---------------------------------------------------------------------------

export function setGlobalOperationDefault(
  operation: OperationKey,
  assignment: StageAssignment,
): Promise<void> {
  return apiFetch<void>(`/config/operations/${operation}`, {
    method: 'PUT',
    body: JSON.stringify(assignment),
  });
}

export function deleteGlobalOperationDefault(operation: OperationKey): Promise<void> {
  return apiFetch<void>(`/config/operations/${operation}`, {
    method: 'DELETE',
  });
}

export function setProjectOperationOverride(
  projectId: string,
  operation: OperationKey,
  assignment: StageAssignment | null,
): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/config/operations/${operation}`, {
    method: 'PUT',
    body: JSON.stringify(assignment),
  });
}

export function deleteProjectOperationOverride(
  projectId: string,
  operation: OperationKey,
): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/config/operations/${operation}`, {
    method: 'DELETE',
  });
}

// ---------------------------------------------------------------------------
// Build pool
// ---------------------------------------------------------------------------

export function getBuildPool(): Promise<BuildPoolConfig | null> {
  return apiFetch<BuildPoolConfig | null>('/config/build-pool');
}

export function setBuildPool(config: BuildPoolConfig): Promise<void> {
  return apiFetch<void>('/config/build-pool', {
    method: 'PUT',
    body: JSON.stringify(config),
  });
}

export function deleteBuildPool(): Promise<void> {
  return apiFetch<void>('/config/build-pool', {
    method: 'DELETE',
  });
}

export function getBuildPoolStatus(): Promise<BuildPoolStatus> {
  return apiFetch<BuildPoolStatus>('/config/build-pool/status');
}
