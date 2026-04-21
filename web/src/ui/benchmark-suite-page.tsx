import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
  type AggregateSummary,
  type BenchmarkComparison,
  type BenchmarkRunSummary,
  type BenchmarkSessionDetail,
  fetchBenchmarkComparison,
  fetchBenchmarkRuns,
  fetchBenchmarkSessions,
} from "../api/benchmarks";
import { ApiError } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatBenchmarkRate, formatBenchmarkSeconds } from "./benchmark-format";
import { BenchmarkRunTable } from "./benchmark-run-table";

type LoadState = "loading" | "ready" | "error" | "unauthorized";
export function BenchmarkSuitePage() {
  const { suiteID = "" } = useParams();
  const [allRuns, setAllRuns] = useState<BenchmarkRunSummary[]>([]);
  const [sessions, setSessions] = useState<BenchmarkSessionDetail[]>([]);
  const [runs, setRuns] = useState<BenchmarkRunSummary[]>([]);
  const [comparison, setComparison] = useState<BenchmarkComparison | null>(null);
  const [agent, setAgent] = useState("");
  const [caseId, setCaseId] = useState("");
  const [templateVariant, setTemplateVariant] = useState("");
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");
    Promise.all([
      fetchBenchmarkSessions(suiteID, {}, controller.signal),
      fetchBenchmarkRuns(suiteID, {}, controller.signal),
    ])
      .then(([sessionItems, runItems]) => {
        setAllRuns(runItems);
        setSessions(sessionItems);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          setAuthHeaderName(error.authHeaderName);
          setErrorMessage("Enter a Centian API key to inspect benchmark results.");
          setLoadState("unauthorized");
          return;
        }
        setErrorMessage("Unable to load benchmark suite details right now.");
        setLoadState("error");
      });

    return () => controller.abort();
  }, [suiteID, reloadToken]);

  useEffect(() => {
    if (loadState === "unauthorized") {
      return;
    }
    const controller = new AbortController();
    const filters = {
      agent: agent || undefined,
      caseId: caseId || undefined,
      templateVariant: templateVariant || undefined,
    };

    Promise.all([
      fetchBenchmarkSessions(suiteID, filters, controller.signal),
      fetchBenchmarkRuns(suiteID, filters, controller.signal),
      fetchBenchmarkComparison(suiteID, filters, controller.signal),
    ])
      .then(([sessionItems, runItems, comparisonItem]) => {
        setSessions(sessionItems);
        setRuns(runItems);
        setComparison(comparisonItem);
      })
      .catch(() => {});

    return () => controller.abort();
  }, [suiteID, agent, caseId, templateVariant, loadState]);

  if (loadState === "loading") {
    return (
      <div className="state-card" data-testid="benchmark-suite-detail-loading">
        <p className="state-card__eyebrow">Syncing</p>
        <h2>Loading benchmark suite…</h2>
        <p>Reading persisted sessions, runs, and comparison aggregates.</p>
      </div>
    );
  }

  if (loadState === "error") {
    return (
      <div className="state-card state-card--error" role="alert">
        <p className="state-card__eyebrow">Link Loss</p>
        <h2>Benchmark suite unavailable</h2>
        <p>{errorMessage}</p>
      </div>
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Access Required"
        title="Benchmark suite is protected"
        body={errorMessage}
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((value) => value + 1)}
      />
    );
  }

  const agentOptions = Array.from(new Set(allRuns.map((run) => run.agent))).sort();
  const caseOptions = Array.from(new Set(allRuns.map((run) => run.caseId))).sort();
  const variantOptions = Array.from(new Set(allRuns.map((run) => run.templateVariant))).sort();
  const templateTitle = comparison?.templateName ?? runs[0]?.templateName ?? comparison?.templateId ?? suiteID;
  const suiteTitle = comparison?.suiteName ?? sessions[0]?.suiteName ?? suiteID;
  const variantRows = comparison?.aggregates.byTemplateVariant ?? [];
  const agentRows = comparison?.aggregates.byAgent ?? [];

  return (
    <div className="benchmark-page">
      <div className="benchmark-toolbar">
        <div className="benchmark-toolbar__heading">
          <p className="state-card__eyebrow">Suite Overview</p>
          <div className="benchmark-toolbar__title-row">
            <h2>{suiteTitle}</h2>
            <div className="benchmark-toolbar__badges">
              <span className="benchmark-badge">{comparison?.runCount ?? 0} Runs</span>
              <span className="benchmark-badge">{comparison?.sessionCount ?? 0} Sessions</span>
            </div>
          </div>
          <p className="benchmark-toolbar__meta">{templateTitle}</p>
        </div>
        <div className="benchmark-filters">
          <label className="benchmark-filter">
            <span>Agent</span>
            <select value={agent} onChange={(event) => setAgent(event.target.value)}>
              <option value="">All agents</option>
              {agentOptions.map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </select>
          </label>
          <label className="benchmark-filter">
            <span>Case</span>
            <select value={caseId} onChange={(event) => setCaseId(event.target.value)}>
              <option value="">All cases</option>
              {caseOptions.map((value) => (
                <option key={value} value={value}>
                  {allRuns.find((run) => run.caseId === value)?.caseName ?? value}
                </option>
              ))}
            </select>
          </label>
          <label className="benchmark-filter">
            <span>Variant</span>
            <select value={templateVariant} onChange={(event) => setTemplateVariant(event.target.value)}>
              <option value="">All variants</option>
              {variantOptions.map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </select>
          </label>
        </div>
      </div>

      <section className="benchmark-section">
        <div className="benchmark-section__header">
          <h3>By Variant</h3>
          <p>{variantRows.length} grouped rows</p>
        </div>
        <BenchmarkAnalysisTable rows={variantRows} firstColumnLabel="Variant" />
      </section>

      <section className="benchmark-section">
        <div className="benchmark-section__header">
          <h3>By Agent</h3>
          <p>{agentRows.length} grouped rows</p>
        </div>
        <BenchmarkAnalysisTable rows={agentRows} firstColumnLabel="Agent" />
      </section>

      <section className="benchmark-section">
        <div className="benchmark-section__header">
          <h3>Sessions</h3>
          <p>{sessions.length} session rows</p>
        </div>
        {sessions.length === 0 ? (
          <p className="benchmark-empty">No sessions match the current filters.</p>
        ) : (
          <div className="benchmark-table" role="table" aria-label="Benchmark sessions">
            <div className="benchmark-table__header benchmark-table__header--sessions" role="row">
              <span>Session</span>
              <span>Runs</span>
              <span>Scored</span>
              <span>Failures</span>
            </div>
            <div className="benchmark-table__body" role="rowgroup">
              {sessions.map((session) => (
                <Link
                  key={session.sessionId}
                  className="benchmark-row benchmark-row--sessions"
                  to={`/benchmarks/${suiteID}/sessions/${session.sessionId}`}
                >
                  <span>{session.sessionPath.split("/").slice(-1)[0]}</span>
                  <span>{session.runCount}</span>
                  <span>{session.scoredRunCount}</span>
                  <span>{session.failedToScoreCount}</span>
                </Link>
              ))}
            </div>
          </div>
        )}
      </section>

      <section className="benchmark-section">
        <div className="benchmark-section__header">
          <h3>Run History</h3>
          <div className="benchmark-section__actions">
            <p>{runs.length} scorecards</p>
            <Link className="benchmark-section__link" to={`/tasks?benchmarkSuite=${encodeURIComponent(suiteID)}`}>
              Show task runs
            </Link>
          </div>
        </div>
        <BenchmarkRunTable suiteId={suiteID} runs={runs} />
      </section>
    </div>
  );
}

function BenchmarkAnalysisTable({ rows, firstColumnLabel }: { rows: AggregateSummary[]; firstColumnLabel: string }) {
  if (rows.length === 0) {
    return <p className="benchmark-empty">No benchmark runs match the current filters.</p>;
  }

  return (
    <div className="benchmark-analysis-table" role="table" aria-label={`Benchmark ${firstColumnLabel.toLowerCase()} analysis`}>
      <div className="benchmark-analysis-table__header" role="row">
        <span>{firstColumnLabel}</span>
        <span>Scored</span>
        <span>Success Rate</span>
        <span>Errors (Centian/MCP)</span>
        <span>Median Time</span>
        <span>First Pass</span>
        <span>Total Actions (Centian/MCP)</span>
      </div>
      <div className="benchmark-analysis-table__body" role="rowgroup">
        {rows.map((row) => (
          <div key={row.key} className="benchmark-analysis-row" role="row">
            <span className="benchmark-analysis-row__label">{analysisRowLabel(row, firstColumnLabel)}</span>
            <span>{row.scoredRunCount}/{row.runCount}</span>
            <span className={`benchmark-analysis-row__success ${successRateClassName(row.successRate)}`}>
              {formatBenchmarkRate(row.successRate)}
            </span>
            <span className="benchmark-error-split">
              <span className="benchmark-error-split__centian">{row.medianFailedTaskToolCalls}</span>
              <span>/</span>
              <span className="benchmark-error-split__mcp">{row.medianFailedDownstreamToolCalls}</span>
            </span>
            <span>{formatBenchmarkSeconds(row.medianWallClockSeconds)}</span>
            <span>{formatBenchmarkRate(row.firstPassSuccessRate)}</span>
            <span className="benchmark-error-split">
              <span className="benchmark-error-split__centian">{row.totalTaskToolCalls}</span>
              <span>/</span>
              <span className="benchmark-error-split__mcp">{row.totalDownstreamToolCalls}</span>
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

function analysisRowLabel(row: AggregateSummary, firstColumnLabel: string): string {
  if (firstColumnLabel === "Variant") {
    return row.templateVariant || row.key;
  }
  if (firstColumnLabel === "Agent") {
    return row.agent || row.key;
  }
  return row.key;
}

function successRateClassName(value: number): string {
  if (value > 0.85) {
    return "benchmark-analysis-row__success--good";
  }
  if (value >= 0.6) {
    return "benchmark-analysis-row__success--warn";
  }
  if (value >= 0.4) {
    return "benchmark-analysis-row__success--risk";
  }
  return "benchmark-analysis-row__success--bad";
}
