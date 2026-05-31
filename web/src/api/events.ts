import { loadStoredApiAuth, unauthorizedAuthHeaderHint } from "./api-auth";
import { ApiError, projectApiPath, type ProcessorAnnotation } from "./task-runs";

export type EventListFilters = {
  gateway?: string;
  server?: string;
  tool?: string;
  direction?: string;
  messageType?: string;
  success?: boolean;
  requestId?: string;
  sessionId?: string;
  cursor?: string;
  limit?: number;
};

export type EventListItem = {
  id: string;
  createdAtUnixMilli: number;
  requestId?: string;
  sessionId?: string;
  transport?: string;
  direction?: string;
  messageType?: string;
  gateway?: string;
  serverName?: string;
  endpoint?: string;
  toolName?: string;
  originalToolName?: string;
  success: boolean;
  isError: boolean;
  payloadJson?: unknown;
  annotations?: ProcessorAnnotation[];
  taskRunId?: string;
  invocationPhasePath?: string;
  invocationNodeKind?: string;
};

export type EventListPage = {
  items: EventListItem[];
  nextCursor?: string;
};

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

function buildQuery(filters: EventListFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.gateway) params.set("gateway", filters.gateway);
  if (filters.server) params.set("server", filters.server);
  if (filters.tool) params.set("tool", filters.tool);
  if (filters.direction) params.set("direction", filters.direction);
  if (filters.messageType) params.set("messageType", filters.messageType);
  if (typeof filters.success === "boolean") params.set("success", String(filters.success));
  if (filters.requestId) params.set("requestId", filters.requestId);
  if (filters.sessionId) params.set("sessionId", filters.sessionId);
  if (filters.cursor) params.set("cursor", filters.cursor);
  if (typeof filters.limit === "number" && Number.isFinite(filters.limit) && filters.limit > 0) {
    params.set("limit", String(filters.limit));
  }
  const query = params.toString();
  return query ? `?${query}` : "";
}

export async function fetchEvents(projectSlug: string | undefined, filters: EventListFilters = {}, signal?: AbortSignal): Promise<EventListPage> {
  const page = await requestJSON<EventListPage>(`${projectApiPath(projectSlug, "/events")}${buildQuery(filters)}`, signal);
  return {
    items: Array.isArray(page?.items) ? page.items : [],
    nextCursor: typeof page?.nextCursor === "string" && page.nextCursor ? page.nextCursor : undefined,
  };
}
