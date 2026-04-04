import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
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

  const byVariant = comparison?.aggregates.byTemplateVariant ?? [];
  const byAgent = comparison?.aggregates.byAgent ?? [];
  const agentOptions = Array.from(new Set(allRuns.map((run) => run.agent))).sort();
  const caseOptions = Array.from(new Set(allRuns.map((run) => run.caseId))).sort();
  const variantOptions = Array.from(new Set(allRuns.map((run) => run.templateVariant))).sort();
  const templateTitle = comparison?.templateName ?? runs[0]?.templateName ?? comparison?.templateId ?? suiteID;
  const suiteTitle = comparison?.suiteName ?? sessions[0]?.suiteName ?? suiteID;
  const variantErrorRows = Array.from(
    runs.reduce((map, run) => {
      const current = map.get(run.templateVariant) ?? {
        templateVariant: run.templateVariant,
        centianErrors: 0,
        mcpErrors: 0,
      };
      current.centianErrors += run.failedTaskToolCalls;
      current.mcpErrors += run.failedDownstreamToolCalls;
      map.set(run.templateVariant, current);
      return map;
    }, new Map<string, { templateVariant: string; centianErrors: number; mcpErrors: number }>()),
  )
    .map(([, row]) => row)
    .sort((left, right) => left.templateVariant.localeCompare(right.templateVariant));

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

      <div className="benchmark-summary-grid">
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">Errors By Variant</p>
          {variantErrorRows.length === 0 ? (
            <p className="benchmark-empty">No error data for the current filters.</p>
          ) : (
            <ul className="benchmark-metric-list benchmark-metric-list--stacked">
              {variantErrorRows.map((item) => (
                <li key={item.templateVariant}>
                  <strong>{item.templateVariant}</strong>
                  <span className="benchmark-error-split">
                    <span className="benchmark-error-split__centian">{item.centianErrors}</span>
                    <span>/</span>
                    <span className="benchmark-error-split__mcp">{item.mcpErrors}</span>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </article>
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">By Variant</p>
          <ul className="benchmark-metric-list">
            {byVariant.slice(0, 3).map((item) => (
              <li key={item.key}>
                <strong>{item.templateVariant}</strong>
                <span>{formatBenchmarkRate(item.successRate)} success</span>
              </li>
            ))}
          </ul>
        </article>
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">By Agent</p>
          <ul className="benchmark-metric-list">
            {byAgent.slice(0, 3).map((item) => (
              <li key={item.key}>
                <strong>{item.agent}</strong>
                <span>{formatBenchmarkSeconds(item.medianWallClockSeconds)} median</span>
              </li>
            ))}
          </ul>
        </article>
      </div>

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
          <p>{runs.length} scorecards</p>
        </div>
        <BenchmarkRunTable suiteId={suiteID} runs={runs} />
      </section>
    </div>
  );
}
