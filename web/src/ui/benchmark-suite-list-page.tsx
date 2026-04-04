import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { type BenchmarkSuiteSummary, fetchBenchmarkSuites } from "../api/benchmarks";
import { ApiError } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatTemplateLabel, formatTimestamp } from "./format";

type LoadState = "loading" | "ready" | "error" | "unauthorized";

export function BenchmarkSuiteListPage() {
  const [items, setItems] = useState<BenchmarkSuiteSummary[]>([]);
  const [templateFilter, setTemplateFilter] = useState("");
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");

    void fetchBenchmarkSuites({}, controller.signal)
      .then((result) => {
        setItems(result);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          setAuthHeaderName(error.authHeaderName);
          setErrorMessage("Enter a Centian API key to read persisted benchmark results.");
          setLoadState("unauthorized");
          return;
        }
        setErrorMessage("Unable to load benchmark suites right now.");
        setLoadState("error");
      });

    return () => controller.abort();
  }, [reloadToken]);

  if (loadState === "loading") {
    return (
      <div className="state-card" data-testid="benchmark-suite-loading">
        <p className="state-card__eyebrow">Syncing</p>
        <h2>Loading benchmark suites…</h2>
        <p>Pulling persisted suite summaries from the Centian API.</p>
      </div>
    );
  }

  if (loadState === "error") {
    return (
      <div className="state-card state-card--error" role="alert">
        <p className="state-card__eyebrow">Link Loss</p>
        <h2>Benchmark feed unavailable</h2>
        <p>{errorMessage}</p>
      </div>
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Access Required"
        title="Benchmark feed is protected"
        body={errorMessage}
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((value) => value + 1)}
      />
    );
  }

  const templateOptions = Array.from(
    new Set(items.map((item) => item.templateId).filter((value): value is string => Boolean(value))),
  ).sort();
  const filteredItems = templateFilter ? items.filter((item) => item.templateId === templateFilter) : items;

  return (
    <div className="benchmark-page">
      <div className="benchmark-toolbar">
        <div>
          <p className="state-card__eyebrow">Benchmark Suites</p>
          <h2>Persisted benchmark history</h2>
        </div>
        <label className="benchmark-filter">
          <span>Template</span>
          <select
            value={templateFilter}
            onChange={(event) => setTemplateFilter(event.target.value)}
          >
            <option value="">All templates</option>
            {templateOptions.map((value) => (
              <option key={value} value={value}>
                {items.find((item) => item.templateId === value)?.templateName ?? formatTemplateLabel(value)}
              </option>
            ))}
          </select>
        </label>
      </div>

      {filteredItems.length === 0 ? (
        <div className="state-card">
          <p className="state-card__eyebrow">Quiet Channel</p>
          <h2>No benchmark suites yet</h2>
          <p>Run and score a benchmark session to make persisted results visible here.</p>
        </div>
      ) : (
        <div className="benchmark-suite-grid">
          {filteredItems.map((item) => (
            <Link key={item.suiteId} className="benchmark-suite-card" to={`/benchmarks/${item.suiteId}`}>
              <div>
                <p className="state-card__eyebrow">{item.templateName ?? formatTemplateLabel(item.templateId ?? item.suiteId)}</p>
                <h3>{item.suiteName || item.suiteId}</h3>
                <p className="benchmark-suite-card__meta">{item.suiteId}</p>
              </div>
              <dl className="benchmark-suite-card__stats">
                <div>
                  <dt>Sessions</dt>
                  <dd>{item.sessionCount}</dd>
                </div>
                <div>
                  <dt>Runs</dt>
                  <dd>{item.runCount}</dd>
                </div>
                <div>
                  <dt>Latest</dt>
                  <dd>{formatTimestamp(new Date(item.latestGeneratedAt).getTime())}</dd>
                </div>
              </dl>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
