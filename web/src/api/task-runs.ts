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

export type TaskRunDetailMetadata = {
  runId: string;
  benchmarkLinks?: TaskRunBenchmarkLink[];
};

export type ProcessorAnnotationFinding = {
  rule?: string;
  path?: string;
};

export type ProcessorAnnotation = {
  processor?: string;
  action?: string;
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

async function requestJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
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

export async function fetchTaskRuns(filters: TaskRunFilters = {}, signal?: AbortSignal): Promise<TaskRunSummary[]> {
  const runs = await requestJSON<TaskRunSummary[]>(`/api/task-runs${buildQuery(filters)}`, signal);
  return Array.isArray(runs) ? runs : [];
}

export async function fetchTaskRunDetail(runID: string, signal?: AbortSignal): Promise<TaskRunDetailMetadata> {
  return requestJSON<TaskRunDetailMetadata>(`/api/task-runs/${encodeURIComponent(runID)}`, signal);
}

export async function fetchTaskRunEvents(runID: string, signal?: AbortSignal): Promise<TaskRunEvent[]> {
  const events = await requestJSON<TaskRunEvent[]>(`/api/task-runs/${encodeURIComponent(runID)}/events`, signal);
  return Array.isArray(events) ? events : [];
}
