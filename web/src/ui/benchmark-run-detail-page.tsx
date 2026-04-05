import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import { type BenchmarkRunDetail, fetchBenchmarkRun } from "../api/benchmarks";
import { ApiError } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatBenchmarkRate, formatBenchmarkSeconds } from "./benchmark-format";

type LoadState = "loading" | "ready" | "error" | "unauthorized";

export function BenchmarkRunDetailPage() {
  const { suiteID = "", scorecardID = "" } = useParams();
  const [detail, setDetail] = useState<BenchmarkRunDetail | null>(null);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");

    void fetchBenchmarkRun(suiteID, scorecardID, controller.signal)
      .then((item) => {
        setDetail(item);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          setAuthHeaderName(error.authHeaderName);
          setErrorMessage("Enter a Centian API key to inspect benchmark run details.");
          setLoadState("unauthorized");
          return;
        }
        setErrorMessage("Unable to load benchmark run details right now.");
        setLoadState("error");
      });

    return () => controller.abort();
  }, [suiteID, scorecardID, reloadToken]);

  if (loadState === "loading") {
    return (
      <div className="state-card" data-testid="benchmark-run-loading">
        <p className="state-card__eyebrow">Syncing</p>
        <h2>Loading benchmark run…</h2>
        <p>Building the live benchmark scorecard.</p>
      </div>
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Access Required"
        title="Benchmark run is protected"
        body={errorMessage}
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((value) => value + 1)}
      />
    );
  }

  if (loadState === "error" || detail == null) {
    return (
      <div className="state-card state-card--error" role="alert">
        <p className="state-card__eyebrow">Link Loss</p>
        <h2>Benchmark run unavailable</h2>
        <p>{errorMessage || "The requested benchmark run could not be loaded."}</p>
      </div>
    );
  }

  const { scorecard } = detail;

  return (
    <div className="benchmark-page">
      <div className="benchmark-toolbar">
        <div>
          <p className="state-card__eyebrow">Run Detail</p>
          <h2>{detail.caseName || scorecard.caseId}</h2>
          <p className="benchmark-toolbar__meta">{detail.templateName || scorecard.templateId}</p>
        </div>
        <p className="benchmark-toolbar__meta">
          {scorecard.agent} · {scorecard.templateVariant} · attempt {scorecard.attempt}
        </p>
      </div>

      <div className="benchmark-summary-grid">
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">Outcome</p>
          <h3>{formatBenchmarkRate(scorecard.outcome.completedSuccessfully ? 1 : 0)} success</h3>
          <p>{formatBenchmarkRate(scorecard.outcome.firstPassSuccess ? 1 : 0)} first pass.</p>
        </article>
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">Efficiency</p>
          <h3>{formatBenchmarkSeconds(scorecard.efficiency.wallClockSeconds)}</h3>
          <p>{scorecard.efficiency.totalToolCalls} tool calls.</p>
        </article>
        <article className="benchmark-summary-card">
          <p className="state-card__eyebrow">Tokens</p>
          <h3>{(scorecard.efficiency.inputTokens ?? 0) + (scorecard.efficiency.outputTokens ?? 0)}</h3>
          <p>
            in {scorecard.efficiency.inputTokens ?? 0} / out {scorecard.efficiency.outputTokens ?? 0}
          </p>
        </article>
      </div>

      <section className="benchmark-section benchmark-detail-grid">
        <article className="benchmark-detail-card">
          <h3>Outcome</h3>
          <dl className="benchmark-detail-list">
            <div><dt>Completed</dt><dd>{String(scorecard.outcome.completedSuccessfully)}</dd></div>
            <div><dt>Final Verification</dt><dd>{String(scorecard.outcome.finalVerificationPassed)}</dd></div>
            <div><dt>First Pass</dt><dd>{String(scorecard.outcome.firstPassSuccess)}</dd></div>
            <div><dt>Invariant Violation</dt><dd>{String(scorecard.outcome.invariantViolation)}</dd></div>
          </dl>
        </article>
        <article className="benchmark-detail-card">
          <h3>Process</h3>
          <dl className="benchmark-detail-list">
            <div><dt>Failed Task Calls</dt><dd>{scorecard.process.failedTaskToolCalls}</dd></div>
            <div><dt>Failed Downstream Calls</dt><dd>{scorecard.process.failedDownstreamToolCalls}</dd></div>
            <div><dt>Restart Count</dt><dd>{scorecard.process.restartCount}</dd></div>
            <div><dt>Fail Count</dt><dd>{scorecard.process.failCount}</dd></div>
            <div><dt>Timeout Count</dt><dd>{scorecard.process.timeoutCount}</dd></div>
          </dl>
        </article>
        <article className="benchmark-detail-card">
          <h3>Files</h3>
          <dl className="benchmark-detail-list">
            <div><dt>Edited Files</dt><dd>{scorecard.efficiency.editedFilesCount}</dd></div>
            <div><dt>Latest Task Run</dt><dd>{scorecard.latestTaskRunId || "—"}</dd></div>
          </dl>
        </article>
      </section>
    </div>
  );
}
