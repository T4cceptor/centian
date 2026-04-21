import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { ApiError, fetchTaskRuns, type TaskRunSummary } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import {
  formatDuration,
  formatTaskRunId,
  formatTemplateLabel,
  formatTimestamp,
  formatTimestampCompact,
  humanizePhase,
} from "./format";
import { getTaskRunUIStatus } from "./task-run-status";

// Tracks the high-level fetch state for the list view.
type LoadState = "loading" | "ready" | "error" | "unauthorized";

// Prefers a final "Completed" label once a run has fully succeeded.
function getTaskRunDisplayPhase(run: TaskRunSummary): string {
  const uiStatus = getTaskRunUIStatus(run.status, run.endedAt);
  if (uiStatus === "success") {
    return "Completed";
  }
  if (uiStatus === "timed_out") {
    return "Timed Out";
  }

  return humanizePhase(run.currentPhase);
}

// Shows the live task run index, including loading, empty, and error states.
export function TaskRunListPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [runs, setRuns] = useState<TaskRunSummary[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [now, setNow] = useState(() => Date.now());
  const [reloadToken, setReloadToken] = useState(0);
  const benchmarkSuite = searchParams.get("benchmarkSuite")?.trim() ?? "";
  const activeFilters = benchmarkSuite ? [{ key: "benchmarkSuite", label: `Benchmark suite: ${benchmarkSuite}` }] : [];
  const listSearch = searchParams.toString();
  const detailSuffix = listSearch ? `?${listSearch}` : "";

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");

    // Abort in-flight fetches when the page unmounts so stale responses do not win.
    void fetchTaskRuns({ benchmarkSuite: benchmarkSuite || undefined }, controller.signal)
      .then((result) => {
        setRuns(result);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          setAuthHeaderName(error.authHeaderName);
          setErrorMessage("Enter a Centian API key to read persisted task runs.");
          setLoadState("unauthorized");
          return;
        }
        setErrorMessage("Unable to load task runs right now.");
        setLoadState("error");
      });

    return () => controller.abort();
  }, [benchmarkSuite, reloadToken]);

  useEffect(() => {
    if (loadState !== "ready") {
      return;
    }

    let inFlight = false;
    let controller: AbortController | null = null;

    const poll = () => {
      if (inFlight) {
        return;
      }
      if (document.visibilityState === "hidden") {
        return;
      }

      inFlight = true;
      controller = new AbortController();

      void fetchTaskRuns({ benchmarkSuite: benchmarkSuite || undefined }, controller.signal)
        .then((result) => {
          setRuns(result);
        })
        .catch((error: unknown) => {
          if ((error as Error)?.name === "AbortError") {
            return;
          }
          if (error instanceof ApiError && error.status === 401) {
            setAuthHeaderName(error.authHeaderName);
            setErrorMessage("Enter a Centian API key to read persisted task runs.");
            setLoadState("unauthorized");
          }
        })
        .finally(() => {
          inFlight = false;
          controller = null;
        });
    };

    const timer = window.setInterval(poll, 2000);
    return () => {
      window.clearInterval(timer);
      controller?.abort();
    };
  }, [benchmarkSuite, loadState]);

  useEffect(() => {
    const activeRunExists = runs.some((run) => getTaskRunUIStatus(run.status, run.endedAt) === "active");
    if (!activeRunExists) {
      return;
    }

    // Refresh the clock only while at least one run is active so durations keep ticking.
    const timer = window.setInterval(() => {
      setNow(Date.now());
    }, 1000);

    return () => window.clearInterval(timer);
  }, [runs]);

  if (loadState === "loading") {
    return (
      <div className="state-card" data-testid="task-run-loading">
        <p className="state-card__eyebrow">Syncing</p>
        <h2>Loading task runs…</h2>
        <p>Pulling the latest workflow activity from the Centian API.</p>
      </div>
    );
  }

  if (loadState === "error") {
    return (
      <div className="state-card state-card--error" role="alert">
        <p className="state-card__eyebrow">Link Loss</p>
        <h2>Task run feed unavailable</h2>
        <p>{errorMessage}</p>
      </div>
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Access Required"
        title="Task run feed is protected"
        body={errorMessage}
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((value) => value + 1)}
      />
    );
  }

  if (runs.length === 0) {
    return (
      <div className="state-card">
        <p className="state-card__eyebrow">Quiet Channel</p>
        <h2>{activeFilters.length > 0 ? "No matching task runs" : "No task runs yet"}</h2>
        <p>
          {activeFilters.length > 0
            ? "No persisted task runs match the active benchmark filters."
            : "Registered task workflows will appear here once the event store has data."}
        </p>
      </div>
    );
  }

  return (
    <div className="task-run-list">
      <div className="task-run-list__toolbar">
        <div>
          <p className="state-card__eyebrow">Live Index</p>
          <h2>Observed task executions</h2>
          {activeFilters.length > 0 ? (
            <div className="task-run-filter-bar" aria-label="Active task run filters">
              {activeFilters.map((filter) => (
                <span key={filter.key} className="task-run-filter-chip">
                  {filter.label}
                </span>
              ))}
              <button
                type="button"
                className="task-run-filter-clear"
                onClick={() => setSearchParams(new URLSearchParams())}
              >
                Clear
              </button>
            </div>
          ) : null}
        </div>
        <p className="task-run-list__count">{runs.length} tracked runs</p>
      </div>

      <div className="task-run-table" role="table" aria-label="Task runs">
        <div className="task-run-table__header" role="row">
          <span>Run</span>
          <span>Template</span>
          <span>Agent</span>
          <span>Status</span>
          <span>Phase</span>
          <span>Started</span>
          <span>Duration</span>
          <span>Events</span>
        </div>

        <div className="task-run-table__body" role="rowgroup">
          {runs.map((run) => {
            const uiStatus = getTaskRunUIStatus(run.status, run.endedAt);
            const templateLabel = formatTemplateLabel(run.templateId, run.templateName);
            const phaseLabel = getTaskRunDisplayPhase(run);
            const startedLabel = formatTimestamp(run.startedAt);
            const startedParts = formatTimestampCompact(run.startedAt);
            return (
              <Link
                key={run.runId}
                aria-label={`Open task run ${run.runId}`}
                className="task-run-row"
                to={`/tasks/${run.runId}${detailSuffix}`}
              >
                <span className="task-run-row__run" title={run.runId}>
                  <strong>{formatTaskRunId(run.runId)}</strong>
                </span>
                <span className="task-run-row__template" title={run.templateId}>
                  {templateLabel}
                </span>
                <span
                  className="task-run-row__agent"
                  title={[run.clientName, run.clientVersion].filter(Boolean).join(" ")}
                >
                  {run.clientName ?? "—"}
                </span>
                <span className="task-run-row__status">
                  <span className={`status-badge status-badge--${uiStatus}`}>{uiStatus}</span>
                </span>
                <span className="task-run-row__phase" title={phaseLabel}>
                  {phaseLabel}
                </span>
                <span className="task-run-row__started" title={startedLabel}>
                  <span>{startedParts.date}</span>
                  <span>{startedParts.time}</span>
                </span>
                <span>{formatDuration(run.startedAt, run.endedAt, now)}</span>
                <span>{run.eventCount}</span>
              </Link>
            );
          })}
        </div>
      </div>
    </div>
  );
}
