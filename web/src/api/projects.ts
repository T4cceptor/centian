import { requestJSON } from "./task-runs";

export type ProjectSummary = {
  slug: string;
  name: string;
  description?: string;
  isDefault: boolean;
  uiEnabled: boolean;
  eventStorageEnabled: boolean;
  taskVerificationEnabled: boolean;
};

export type ProjectListResponse = {
  projects?: ProjectSummary[];
};

export async function fetchProjects(signal?: AbortSignal): Promise<ProjectSummary[]> {
  const response = await requestJSON<ProjectListResponse>("/api/projects", signal);
  return Array.isArray(response?.projects) ? response.projects : [];
}
