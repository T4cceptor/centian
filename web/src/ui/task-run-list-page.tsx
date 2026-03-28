import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { fetchTaskRuns, type TaskRunSummary } from "../api/task-runs";
import { formatDuration, formatTaskRunId, formatTimestamp, humanizePhase } from "./format";
import { getTaskRunUIStatus } from "./task-run-status";

// Tracks the high-level fetch state for the list view.
type LoadState = "loading" | "ready" | "error";

// Prefers a final "Completed" label once a run has fully succeeded.
function getTaskRunDisplayPhase(run: TaskRunSummary): string {
  const uiStatus = getTaskRunUIStatus(run.status, run.endedAt);
  if (uiStatus === "success") {
    return "Completed";
  }

  return humanizePhase(run.currentPhase);
}

// Shows the live task run index, including loading, empty, and error states.
export function TaskRunListPage() {
  const [runs, setRuns] = useState<TaskRunSummary[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");

    // Abort in-flight fetches when the page unmounts so stale responses do not win.
    void fetchTaskRuns(controller.signal)
      .then((result) => {
        setRuns(result);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        setErrorMessage("Unable to load task runs right now.");
        setLoadState("error");
      });

    return () => controller.abort();
  }, []);

  useEffect(() => {
    const activeRunExists = runs.some((run) => run.endedAt == null);
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

  if (runs.length === 0) {
    return (
      <div className="state-card">
        <p className="state-card__eyebrow">Quiet Channel</p>
        <h2>No task runs yet</h2>
        <p>Registered task workflows will appear here once the event store has data.</p>
      </div>
    );
  }

  return (
    <div className="task-run-list">
      <div className="task-run-list__toolbar">
        <div>
          <p className="state-card__eyebrow">Live Index</p>
          <h2>Observed task executions</h2>
        </div>
        <p className="task-run-list__count">{runs.length} tracked runs</p>
      </div>

      <div className="task-run-table" role="table" aria-label="Task runs">
        <div className="task-run-table__header" role="row">
          <span>Run</span>
          <span>Template</span>
          <span>Status</span>
          <span>Phase</span>
          <span>Started</span>
          <span>Duration</span>
          <span>Events</span>
        </div>

        <div className="task-run-table__body" role="rowgroup">
          {runs.map((run) => {
            const uiStatus = getTaskRunUIStatus(run.status, run.endedAt);
            return (
              <Link
                key={run.runId}
                aria-label={`Open task run ${run.runId}`}
                className="task-run-row"
                to={`/tasks/${run.runId}`}
              >
                <span className="task-run-row__run" title={run.runId}>
                  <strong>{formatTaskRunId(run.runId)}</strong>
                </span>
                <span className="task-run-row__template" title={run.templateId}>
                  {run.templateId}
                </span>
                <span className="task-run-row__status">
                  <span className={`status-badge status-badge--${uiStatus}`}>{uiStatus}</span>
                </span>
                <span>{getTaskRunDisplayPhase(run)}</span>
                <span>{formatTimestamp(run.startedAt)}</span>
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
