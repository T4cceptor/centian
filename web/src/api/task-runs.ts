import { loadStoredApiAuth, unauthorizedAuthHeaderHint } from "./api-auth";

export type TaskRunSummary = {
  runId: string;
  templateId: string;
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

export type TaskRunEvent = {
  source: "task" | "action";
  id: string;
  createdAtUnixMilli: number;
  payloadJson?: unknown;
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

export async function fetchTaskRuns(signal?: AbortSignal): Promise<TaskRunSummary[]> {
  const runs = await requestJSON<TaskRunSummary[]>("/api/task-runs", signal);
  return Array.isArray(runs) ? runs : [];
}

export async function fetchTaskRunEvents(runID: string, signal?: AbortSignal): Promise<TaskRunEvent[]> {
  const events = await requestJSON<TaskRunEvent[]>(`/api/task-runs/${encodeURIComponent(runID)}/events`, signal);
  return Array.isArray(events) ? events : [];
}
