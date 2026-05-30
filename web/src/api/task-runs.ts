import { loadStoredApiAuth, unauthorizedAuthHeaderHint } from "./api-auth";

export type TaskRunSummary = {
  runId: string;
  templateId: string;
  templateName?: string;
  principalId?: string;
  sessionId?: string;
  clientName?: string;
  clientVersion?: string;
  startedAt: number;
  endedAt?: number;
  status: string;
  currentPhase: string;
  currentNodeKind?: string;
  taskEventCount: number;
  actionEventCount: number;
  eventCount: number;
};

export type TaskRunFilters = {
  benchmarkSuite?: string;
};

export const defaultProjectSlug = "default";

export type TaskRunBenchmarkLink = {
  benchmarkRunId: string;
  sessionId: string;
  sessionPath?: string;
  suiteId: string;
  suiteName?: string;
  caseId: string;
  caseName?: string;
  agent: string;
  selectedModel?: string;
  templateVariant: string;
  attempt: number;
  startedAtUnixMilli: number;
};

export type TaskRunSnapshot = {
  runId: string;
  templateId: string;
  templateName: string;
  taskDescription?: string;
  status: string;
  phase: string;
  workflowReady: boolean;
  lastFailureMessage?: string;
  explicitFailReason?: string;
  lastActivityAtUnixMilli?: number;
  expiresAtUnixMilli?: number;
  onboarding?: Record<string, unknown>;
  planning?: Record<string, unknown>;
  selectedTemplate: TaskRunTemplateSnapshot;
  runnableTemplate?: TaskRunTemplateSnapshot;
  steps?: TaskRunStepStateSnapshot[];
};

export type TaskRunTemplateSnapshot = {
  version: string;
  task: TaskRunTemplateTaskSnapshot;
  parameters?: TaskRunTemplateParameter[];
  workflow?: TaskRunWorkflowSnapshot;
  compiledWorkflow?: TaskRunCompiledWorkflowSnapshot;
};

export type TaskRunTemplateTaskSnapshot = {
  id: string;
  name: string;
  description: string;
  instructions?: string;
};

export type TaskRunTemplateParameter = {
  name: string;
  description?: string;
};

export type TaskRunWorkflowSnapshot = {
  onboarding?: TaskRunLifecycleNodeSpec;
  planning?: TaskRunPlanningNodeSpec;
  scaffolding?: TaskRunExecutionNodeSpec[];
  execution?: TaskRunExecutionNodeSpec[];
};

export type TaskRunLifecycleNodeSpec = {
  instructions?: string;
  toolsAllowed?: string[];
  checkpoint?: TaskRunCheckpointHint;
};

export type TaskRunPlanningNodeSpec = TaskRunLifecycleNodeSpec & {
  editableFields?: string[];
  requiredInputs?: string[];
  next?: string;
};

export type TaskRunExecutionNodeSpec = TaskRunLifecycleNodeSpec & {
  id: string;
  kind?: string;
  name?: string;
  description?: string;
  checks?: TaskRunCheck[];
  invariants?: TaskRunInvariant[];
  next?: string;
  subSteps?: TaskRunExecutionNodeSpec[];
};

export type TaskRunCheckpointHint = {
  enabled?: boolean;
};

export type TaskRunCheck = {
  id: string;
  description?: string;
  command: string;
  pre_conditions?: TaskRunCondition[];
  post_conditions?: TaskRunCondition[];
};

export type TaskRunInvariant = {
  id: string;
  description?: string;
  command: string;
};

export type TaskRunCondition = {
  type: string;
  value?: unknown;
  values?: unknown[];
  path?: string;
};

export type TaskRunCompiledWorkflowSnapshot = {
  nodes?: Record<string, TaskRunWorkflowNodeSnapshot>;
  onboardingPath?: string;
  planningPath?: string;
  firstExecutablePath?: string;
  workflowSteps?: TaskRunCompiledStepSnapshot[];
};

export type TaskRunWorkflowNodeSnapshot = {
  path: string;
  kind: string;
  parentPath?: string;
  nextPath?: string;
  stepNumber?: number;
  stepId?: string;
  name?: string;
  description?: string;
  instructions?: string;
  allowedTools?: string[];
  checkpoint?: TaskRunCheckpointHint;
  editableFields?: string[];
  requiredPlanningInputs?: string[];
};

export type TaskRunCompiledStepSnapshot = {
  id: string;
  path: string;
  parentPath?: string;
  nextPath?: string;
  name?: string;
  description?: string;
  instructions?: string;
  allowedTools?: string[];
  checkpoint?: TaskRunCheckpointHint;
  checks?: TaskRunCheck[];
  invariants?: TaskRunInvariant[];
};

export type TaskRunStepStateSnapshot = {
  id: string;
  path: string;
  status: string;
  invariantBaselines?: Record<string, string>;
};

export type TaskRunDetailMetadata = {
  runId: string;
  summary?: TaskRunSummary;
  snapshot?: TaskRunSnapshot;
  benchmarkLinks?: TaskRunBenchmarkLink[];
};

export type ProcessorAnnotationFinding = {
  rule?: string;
  path?: string;
};

export type ProcessorAnnotation = {
  type?: string;
  processor?: string;
  action?: string;
  category?: string;
  severity?: string;
  message?: string;
  findings?: ProcessorAnnotationFinding[];
  details?: Record<string, unknown>;
};

export type TaskRunEvent = {
  source: "task" | "action";
  id: string;
  createdAtUnixMilli: number;
  payloadJson?: unknown;
  annotations?: ProcessorAnnotation[];
  eventType?: string;
  outcome?: string;
  relatedActionRequestId?: string;
  phasePath?: string;
  nodeKind?: string;
  resultingPhasePath?: string;
  resultingNodeKind?: string;
  requestId?: string;
  direction?: string;
  messageType?: string;
  toolName?: string;
  originalToolName?: string;
  success?: boolean;
  isError?: boolean;
  transport?: string;
  gateway?: string;
  serverName?: string;
  endpoint?: string;
};

export class ApiError extends Error {
  status: number;
  authHeaderName?: string;

  constructor(status: number, message?: string, authHeaderName?: string) {
    super(message ?? `Request failed (${status})`);
    this.name = "ApiError";
    this.status = status;
    this.authHeaderName = authHeaderName;
  }
}

export function normalizeProjectSlug(projectSlug?: string): string {
  const trimmed = projectSlug?.trim();
  return trimmed || defaultProjectSlug;
}

export function projectApiPath(projectSlug: string | undefined, path: string): string {
  return `/api/${encodeURIComponent(normalizeProjectSlug(projectSlug))}${path}`;
}

export async function requestJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const headers = new Headers();
  const storedAuth = loadStoredApiAuth();
  if (storedAuth) {
    headers.set(storedAuth.headerName, storedAuth.apiKey);
  }

  const response = await fetch(url, { signal, headers });
  if (!response.ok) {
    const authHeaderName =
      response.headers && typeof response.headers.get === "function"
        ? (response.headers.get(unauthorizedAuthHeaderHint) ?? undefined)
        : undefined;
    throw new ApiError(response.status, undefined, authHeaderName);
  }
  return (await response.json()) as T;
}

function buildQuery(filters: TaskRunFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.benchmarkSuite) {
    params.set("benchmarkSuite", filters.benchmarkSuite);
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

export async function fetchTaskRuns(projectSlug: string | undefined, filters: TaskRunFilters = {}, signal?: AbortSignal): Promise<TaskRunSummary[]> {
  const runs = await requestJSON<TaskRunSummary[]>(`${projectApiPath(projectSlug, "/task-runs")}${buildQuery(filters)}`, signal);
  return Array.isArray(runs) ? runs : [];
}

export async function fetchTaskRunDetail(projectSlug: string | undefined, runID: string, signal?: AbortSignal): Promise<TaskRunDetailMetadata> {
  return requestJSON<TaskRunDetailMetadata>(projectApiPath(projectSlug, `/task-runs/${encodeURIComponent(runID)}`), signal);
}

export async function fetchTaskRunEvents(projectSlug: string | undefined, runID: string, signal?: AbortSignal): Promise<TaskRunEvent[]> {
  const events = await requestJSON<TaskRunEvent[]>(projectApiPath(projectSlug, `/task-runs/${encodeURIComponent(runID)}/events`), signal);
  return Array.isArray(events) ? events : [];
}
