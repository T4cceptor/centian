import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import { type BenchmarkSessionDetail, fetchBenchmarkSession } from "../api/benchmarks";
import { ApiError } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatBenchmarkRate } from "./benchmark-format";
import { BenchmarkRunTable } from "./benchmark-run-table";

type LoadState = "loading" | "ready" | "error" | "unauthorized";

export function BenchmarkSessionDetailPage() {
  const { suiteID = "", sessionID = "" } = useParams();
  const [session, setSession] = useState<BenchmarkSessionDetail | null>(null);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");

    void fetchBenchmarkSession(suiteID, sessionID, controller.signal)
      .then((item) => {
        setSession(item);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          setAuthHeaderName(error.authHeaderName);
          setErrorMessage("Enter a Centian API key to inspect benchmark session details.");
          setLoadState("unauthorized");
          return;
        }
        setErrorMessage("Unable to load benchmark session details right now.");
        setLoadState("error");
      });

    return () => controller.abort();
  }, [suiteID, sessionID, reloadToken]);

  if (loadState === "loading") {
    return (
      <div className="state-card" data-testid="benchmark-session-loading">
        <p className="state-card__eyebrow">Syncing</p>
        <h2>Loading benchmark session…</h2>
        <p>Reading the persisted session summary and linked scorecards.</p>
      </div>
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Access Required"
        title="Benchmark session is protected"
        body={errorMessage}
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((value) => value + 1)}
      />
    );
  }

  if (loadState === "error" || session == null) {
    return (
      <div className="state-card state-card--error" role="alert">
        <p className="state-card__eyebrow">Link Loss</p>
        <h2>Benchmark session unavailable</h2>
        <p>{errorMessage || "The requested benchmark session could not be loaded."}</p>
      </div>
    );
  }

  const byCase = session.aggregates.byCase;

  return (
    <div className="benchmark-page">
      <div className="benchmark-toolbar">
        <div>
          <p className="state-card__eyebrow">Session Detail</p>
          <h2>{session.suiteName || session.suiteId}</h2>
          <p className="benchmark-toolbar__meta">{session.sessionPath.split("/").slice(-1)[0]}</p>
        </div>
        <p className="benchmark-toolbar__meta">{session.runCount} runs preserved</p>
      </div>

      <div className="benchmark-summary-grid">
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">Scored</p>
          <h3>{session.scoredRunCount}</h3>
          <p>{session.failedToScoreCount} failed to score.</p>
        </article>
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">Agents</p>
          <h3>{session.agents?.join(", ") || "—"}</h3>
          <p>{session.templateVariants?.join(", ") || "No template variants recorded."}</p>
        </article>
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">By Case</p>
          <ul className="benchmark-metric-list">
            {byCase.slice(0, 3).map((item) => (
              <li key={item.key}>
                <strong>{session.runs?.find((run) => run.caseId === item.caseId)?.caseName || item.caseId}</strong>
                <span>{formatBenchmarkRate(item.successRate)} success</span>
              </li>
            ))}
          </ul>
        </article>
      </div>

      <section className="benchmark-section">
        <div className="benchmark-section__header">
          <h3>Session Runs</h3>
          <p>{session.runs?.length ?? 0} linked scorecards</p>
        </div>
        <BenchmarkRunTable suiteId={suiteID} runs={session.runs ?? []} />
      </section>
    </div>
  );
}
