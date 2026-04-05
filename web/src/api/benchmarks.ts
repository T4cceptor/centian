import { loadStoredApiAuth, unauthorizedAuthHeaderHint } from "./api-auth";
import { ApiError } from "./task-runs";

export type BenchmarkRunFilters = {
  suiteId?: string;
  templateId?: string;
  sessionId?: string;
  caseId?: string;
  agent?: string;
  templateVariant?: string;
};

export type BenchmarkSuiteSummary = {
  suiteId: string;
  suiteName?: string;
  templateId?: string;
  templateName?: string;
  latestGeneratedAt: string;
  sessionCount: number;
  runCount: number;
  agents?: string[];
  caseIds?: string[];
  caseNames?: string[];
  templateVariants?: string[];
};

export type BenchmarkRunSummary = {
  scorecardId: string;
  sessionId: string;
  sessionPath: string;
  suiteId: string;
  suiteName?: string;
  templateId: string;
  templateName?: string;
  caseId: string;
  caseName?: string;
  agent: string;
  templateVariant: string;
  attempt: number;
  rawStatus: string;
  latestTaskRunId?: string;
  linkedTaskRunIds?: string[];
  completedSuccessfully: boolean;
  finalVerificationPassed: boolean;
  firstPassSuccess: boolean;
  invariantViolation: boolean;
  restartOccurred: boolean;
  failOccurred: boolean;
  timeoutOccurred: boolean;
  wallClockSeconds: number;
  totalToolCalls: number;
  totalTaskToolCalls: number;
  totalDownstreamToolCalls: number;
  inputTokens?: number;
  outputTokens?: number;
  failedTaskToolCalls: number;
  failedDownstreamToolCalls: number;
  editedFilesCount: number;
  errorActionabilityScore?: number;
};

export type AggregateSummary = {
  key: string;
  sessionPath?: string;
  caseId?: string;
  agent?: string;
  templateVariant?: string;
  runCount: number;
  scoredRunCount: number;
  successRate: number;
  firstPassSuccessRate: number;
  finalVerificationPassRate: number;
  invariantViolationRate: number;
  restartFailTimeoutRate: number;
  medianWallClockSeconds: number;
  medianTotalToolCalls: number;
  medianInputTokens: number;
  medianOutputTokens: number;
  medianFailedTaskToolCalls: number;
  medianFailedDownstreamToolCalls: number;
  medianEditedFilesCount: number;
  manualActionabilityCount: number;
  averageManualActionabilityScore?: number;
};

export type BenchmarkSessionDetail = {
  sessionId: string;
  suiteId: string;
  suiteName?: string;
  templateId?: string;
  templateName?: string;
  sessionPath: string;
  generatedAt: string;
  runCount: number;
  scoredRunCount: number;
  failedToScoreCount: number;
  agents?: string[];
  caseIds?: string[];
  caseNames?: string[];
  templateVariants?: string[];
  aggregates: {
    byCase: AggregateSummary[];
    byAgent: AggregateSummary[];
    byTemplateVariant: AggregateSummary[];
    byCaseAgentVariant: AggregateSummary[];
  };
  runs?: BenchmarkRunSummary[];
};

export type BenchmarkRunDetail = {
  scorecardId: string;
  sessionId: string;
  sessionPath: string;
  suiteName?: string;
  templateName?: string;
  caseName?: string;
  scorecard: {
    suiteId: string;
    suiteName?: string;
    caseId: string;
    caseName?: string;
    templateId: string;
    templateName?: string;
    templateVariant: string;
    agent: string;
    attempt: number;
    rawStatus: string;
    latestTaskRunId?: string;
    linkedTaskRunIds?: string[];
    outcome: {
      completedSuccessfully: boolean;
      finalVerificationPassed: boolean;
      firstPassSuccess: boolean;
      restartOccurred: boolean;
      failOccurred: boolean;
      timeoutOccurred: boolean;
      invariantViolation: boolean;
    };
    process: {
      failedTaskToolCalls: number;
      failedDownstreamToolCalls: number;
      totalTaskToolCalls: number;
      totalDownstreamToolCalls: number;
      totalStepRetries: number;
      replanningCount: number;
      recoveryTimeSeconds?: number;
      recoveryToolCalls?: number;
    };
    efficiency: {
      wallClockSeconds: number;
      totalToolCalls: number;
      inputTokens?: number;
      outputTokens?: number;
      editedFilesCount: number;
      editedFiles?: string[];
      observedCommandCalls: number;
    };
    manual: {
      errorActionabilityScore?: number;
      errorActionabilityNotes?: string;
    };
    warnings?: string[];
    errors?: string[];
    agentMetadata?: unknown;
    generatedAt: string;
  };
};

export type BenchmarkComparison = {
  suiteId: string;
  suiteName?: string;
  templateId?: string;
  templateName?: string;
  filters: BenchmarkRunFilters;
  sessionCount: number;
  runCount: number;
  sessions: Array<{
    sessionPath: string;
    generatedAt: string;
    runCount: number;
    scoredRunCount: number;
    failedToScoreCount: number;
  }>;
  runs: BenchmarkRunSummary[];
  aggregates: {
    bySession: AggregateSummary[];
    byCase: AggregateSummary[];
    byAgent: AggregateSummary[];
    byTemplateVariant: AggregateSummary[];
    byCaseAgentVariant: AggregateSummary[];
  };
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

function buildQuery(filters: BenchmarkRunFilters = {}): string {
  const params = new URLSearchParams();
  if (filters.suiteId) params.set("suite", filters.suiteId);
  if (filters.templateId) params.set("template", filters.templateId);
  if (filters.sessionId) params.set("sessionID", filters.sessionId);
  if (filters.caseId) params.set("case", filters.caseId);
  if (filters.agent) params.set("agent", filters.agent);
  if (filters.templateVariant) params.set("templateVariant", filters.templateVariant);
  const query = params.toString();
  return query ? `?${query}` : "";
}

export async function fetchBenchmarkSuites(filters: BenchmarkRunFilters = {}, signal?: AbortSignal): Promise<BenchmarkSuiteSummary[]> {
  const items = await requestJSON<BenchmarkSuiteSummary[]>(`/api/benchmarks/suites${buildQuery(filters)}`, signal);
  return Array.isArray(items) ? items : [];
}

export async function fetchBenchmarkSessions(
  suiteId: string,
  filters: BenchmarkRunFilters = {},
  signal?: AbortSignal,
): Promise<BenchmarkSessionDetail[]> {
  const items = await requestJSON<BenchmarkSessionDetail[]>(
    `/api/benchmarks/suites/${encodeURIComponent(suiteId)}/sessions${buildQuery(filters)}`,
    signal,
  );
  return Array.isArray(items) ? items : [];
}

export async function fetchBenchmarkSession(
  suiteId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<BenchmarkSessionDetail> {
  return requestJSON<BenchmarkSessionDetail>(
    `/api/benchmarks/suites/${encodeURIComponent(suiteId)}/sessions/${encodeURIComponent(sessionId)}`,
    signal,
  );
}

export async function fetchBenchmarkRuns(
  suiteId: string,
  filters: BenchmarkRunFilters = {},
  signal?: AbortSignal,
): Promise<BenchmarkRunSummary[]> {
  const items = await requestJSON<BenchmarkRunSummary[]>(
    `/api/benchmarks/suites/${encodeURIComponent(suiteId)}/runs${buildQuery(filters)}`,
    signal,
  );
  return Array.isArray(items) ? items : [];
}

export async function fetchBenchmarkRun(
  suiteId: string,
  scorecardId: string,
  signal?: AbortSignal,
): Promise<BenchmarkRunDetail> {
  return requestJSON<BenchmarkRunDetail>(
    `/api/benchmarks/suites/${encodeURIComponent(suiteId)}/runs/${encodeURIComponent(scorecardId)}`,
    signal,
  );
}

export async function fetchBenchmarkComparison(
  suiteId: string,
  filters: BenchmarkRunFilters = {},
  signal?: AbortSignal,
): Promise<BenchmarkComparison> {
  return requestJSON<BenchmarkComparison>(
    `/api/benchmarks/suites/${encodeURIComponent(suiteId)}/comparison${buildQuery(filters)}`,
    signal,
  );
}
