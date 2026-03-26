export type TaskRunSummary = {
  runId: string;
  templateId: string;
  principalId?: string;
  sessionId?: string;
  startedAt: number;
  endedAt?: number;
  status: string;
  currentPhase: string;
  currentNodeKind?: string;
  taskEventCount: number;
  actionEventCount: number;
  eventCount: number;
};

export async function fetchTaskRuns(signal?: AbortSignal): Promise<TaskRunSummary[]> {
  const response = await fetch("/api/task-runs", { signal });
  if (!response.ok) {
    throw new Error(`Failed to fetch task runs (${response.status})`);
  }

  const runs = (await response.json()) as TaskRunSummary[];
  return Array.isArray(runs) ? runs : [];
}
