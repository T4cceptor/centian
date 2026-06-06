import { useEffect, useRef, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import {
  type ActivityQuery,
  type ActivitySummary,
  LIVE_REFRESH_MS,
  activityRanges,
  categoryLabels,
  fetchActivitySummary,
  interventionCategories,
  isActivityRange,
  rangeDurationsMs,
  rangeLabels,
} from "../api/activity";
import { type PrincipalRef, fetchPrincipals } from "../api/principals";
import { ApiError, normalizeProjectSlug } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { CategoryIcon } from "./category-icon";
import { formatClockTime, formatTimestampCompact, toDatetimeLocalValue } from "./format";
import { InterventionSkyline } from "./intervention-skyline";

type LoadState = "loading" | "ready" | "error" | "unauthorized";

const RANGES = activityRanges;

const DEFAULT_QUERY: ActivityQuery = { kind: "preset", range: "6h" };

// A stable string key so the fetch effect re-runs only when the window changes.
function queryKey(query: ActivityQuery): string {
  return query.kind === "custom" ? `c:${query.startUnixMilli}:${query.endUnixMilli}` : `p:${query.range}`;
}

// isLiveQuery reports whether a query is the auto-refreshing Live preset.
function isLiveQuery(query: ActivityQuery): boolean {
  return query.kind === "preset" && query.range === "live";
}

// windowForQuery derives an absolute [start, end] (unix milli) for a query so the
// principals dropdown can be scoped without waiting on the activity response.
function windowForQuery(query: ActivityQuery): { start: number; end: number } {
  if (query.kind === "custom") {
    return { start: query.startUnixMilli, end: query.endUnixMilli };
  }
  const end = Date.now();
  return { start: end - rangeDurationsMs[query.range], end };
}

// Renders the window header compactly, expanding to full dates when it spans days.
function formatWindowLabel(startUnixMilli: number, endUnixMilli: number): string {
  const start = formatTimestampCompact(startUnixMilli);
  const end = formatTimestampCompact(endUnixMilli);
  const startClock = formatClockTime(startUnixMilli);
  const endClock = formatClockTime(endUnixMilli);
  if (start.date === end.date) {
    return `${start.date} · ${startClock}–${endClock}`;
  }
  return `${start.date} ${startClock} → ${end.date} ${endClock}`;
}

const STAT_FIELDS: { key: keyof ActivitySummary["stats"]; label: string }[] = [
  { key: "interventions", label: "Interventions" },
  { key: "threatsNeutralized", label: "Threats neutralized" },
  { key: "piiRedacted", label: "PII / secrets redacted" },
  { key: "riskyActionsHeld", label: "Risky actions held" },
  { key: "requestsInspected", label: "Requests inspected" },
];

export function ActivityPage() {
  const { projectSlug: rawProjectSlug } = useParams();
  const projectSlug = normalizeProjectSlug(rawProjectSlug);
  const [searchParams] = useSearchParams();
  const [summary, setSummary] = useState<ActivitySummary>();
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  // Seed the timeframe from a ?range= query param (e.g. `centian demo` opens Live).
  const [query, setQuery] = useState<ActivityQuery>(() => {
    const requested = searchParams.get("range");
    return isActivityRange(requested) ? { kind: "preset", range: requested } : DEFAULT_QUERY;
  });
  const [customStart, setCustomStart] = useState<string>("");
  const [customEnd, setCustomEnd] = useState<string>("");
  const [customError, setCustomError] = useState<string>("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [principal, setPrincipal] = useState<string>("");
  const [principals, setPrincipals] = useState<PrincipalRef[]>([]);
  const [pinnedId, setPinnedId] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const timeframeRef = useRef<HTMLDivElement>(null);
  // Stack of prior queries so double-clicking the chart can zoom back out.
  const historyRef = useRef<ActivityQuery[]>([]);

  const activeKey = queryKey(query);

  // Replace the active query, remembering the previous one for zoom-out.
  const pushQuery = (next: ActivityQuery) => {
    historyRef.current.push(query);
    setQuery(next);
  };

  // Set a query without remembering history (preset/custom switches start fresh).
  const resetQuery = (next: ActivityQuery) => {
    historyRef.current = [];
    setQuery(next);
  };

  const handleZoom = (startUnixMilli: number, endUnixMilli: number) => {
    pushQuery({ kind: "custom", startUnixMilli, endUnixMilli });
  };

  const handleZoomOut = () => {
    const previous = historyRef.current.pop();
    setQuery(previous ?? DEFAULT_QUERY);
  };

  // Close the timeframe popover on outside click or Escape.
  useEffect(() => {
    if (!pickerOpen) {
      return;
    }
    const onPointerDown = (event: MouseEvent) => {
      if (timeframeRef.current && !timeframeRef.current.contains(event.target as Node)) {
        setPickerOpen(false);
      }
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setPickerOpen(false);
      }
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [pickerOpen]);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");
    setPinnedId(null);

    void fetchActivitySummary(projectSlug, query, principal || undefined, controller.signal)
      .then((result) => {
        setSummary(result);
        setLoadState("ready");
        // Keep the custom pickers in sync with the resolved window while on a
        // preset, so opening the custom range starts from what's on screen.
        if (query.kind === "preset") {
          setCustomStart(toDatetimeLocalValue(result.rangeStartUnixMilli));
          setCustomEnd(toDatetimeLocalValue(result.rangeEndUnixMilli));
        }
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          setAuthHeaderName(error.authHeaderName);
          setErrorMessage("Enter a Centian API key to read activity.");
          setLoadState("unauthorized");
          return;
        }
        setErrorMessage("Unable to load activity right now.");
        setLoadState("error");
      });

    return () => controller.abort();
    // activeKey captures the meaningful contents of `query`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectSlug, activeKey, principal, reloadToken]);

  // Populate the principal dropdown with principals seen in the selected window.
  // Keyed on the timeframe (activeKey), not the sliding summary, so Live polling
  // does not churn the list or clear the current selection.
  useEffect(() => {
    const controller = new AbortController();
    const principalWindow = windowForQuery(query);
    void fetchPrincipals(projectSlug, principalWindow, controller.signal)
      .then((result) => {
        setPrincipals(result);
        // Drop the selected principal if it is absent from the new timeframe.
        setPrincipal((current) => (current && !result.some((p) => p.id === current) ? "" : current));
      })
      .catch(() => {
        /* principals are a best-effort affordance; ignore failures */
      });
    return () => controller.abort();
    // activeKey captures the meaningful contents of `query`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectSlug, activeKey]);

  // Live mode: silently re-poll the rolling window every few seconds. The interval
  // is torn down when a different timeframe is selected and re-armed when Live is
  // chosen again. Polling pauses while the tab is hidden.
  useEffect(() => {
    if (!isLiveQuery(query)) {
      return;
    }
    let controller: AbortController | null = null;
    const tick = () => {
      if (document.visibilityState === "hidden") {
        return;
      }
      controller?.abort();
      controller = new AbortController();
      void fetchActivitySummary(projectSlug, query, principal || undefined, controller.signal)
        .then((result) => setSummary(result))
        .catch(() => {
          /* transient errors are ignored; the next tick retries */
        });
    };
    const interval = window.setInterval(tick, LIVE_REFRESH_MS);
    return () => {
      window.clearInterval(interval);
      controller?.abort();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectSlug, activeKey, principal]);

  const applyCustomRange = () => {
    const startMs = customStart ? new Date(customStart).getTime() : NaN;
    const endMs = customEnd ? new Date(customEnd).getTime() : NaN;
    if (Number.isNaN(startMs) || Number.isNaN(endMs)) {
      setCustomError("Pick both a start and an end.");
      return;
    }
    if (endMs <= startMs) {
      setCustomError("End must be after start.");
      return;
    }
    setCustomError("");
    setPickerOpen(false);
    resetQuery({ kind: "custom", startUnixMilli: startMs, endUnixMilli: endMs });
  };

  if (loadState === "loading") {
    return (
      <div className="state-card">
        <p className="state-card__eyebrow">Centian / Activity</p>
        <h2>Loading activity…</h2>
        <p>Gathering interventions for this window.</p>
      </div>
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Centian / Activity"
        title="Authentication required"
        body={errorMessage}
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((token) => token + 1)}
      />
    );
  }

  if (loadState === "error" || !summary) {
    return (
      <div className="state-card">
        <p className="state-card__eyebrow">Centian / Activity</p>
        <h2>Something went wrong</h2>
        <p>{errorMessage || "Unable to load activity right now."}</p>
        <button className="action-button" type="button" onClick={() => setReloadToken((token) => token + 1)}>
          Retry
        </button>
      </div>
    );
  }

  const windowLabel = formatWindowLabel(summary.rangeStartUnixMilli, summary.rangeEndUnixMilli);
  const hasInterventions = summary.interventions.length > 0;

  return (
    <div className="activity">
      <div className="activity__toolbar">
        <span className="activity__window">{windowLabel}</span>
        <div className="activity__timeframe" ref={timeframeRef}>
          <div className="activity__range" role="group" aria-label="Timeframe">
            {RANGES.map((option) => (
              <button
                key={option}
                type="button"
                className={
                  query.kind === "preset" && query.range === option
                    ? `activity__range-btn activity__range-btn--active${option === "live" ? " activity__range-btn--live" : ""}`
                    : "activity__range-btn"
                }
                onClick={() => {
                  setPickerOpen(false);
                  resetQuery({ kind: "preset", range: option });
                }}
              >
                {option === "live" && query.kind === "preset" && query.range === "live" ? (
                  <span className="activity__live-dot" aria-hidden="true" />
                ) : null}
                {rangeLabels[option]}
              </button>
            ))}
            <button
              type="button"
              className={
                "activity__range-btn activity__range-btn--custom" +
                (query.kind === "custom" ? " activity__range-btn--active" : "") +
                (pickerOpen ? " activity__range-btn--open" : "")
              }
              aria-haspopup="dialog"
              aria-expanded={pickerOpen}
              onClick={() => setPickerOpen((open) => !open)}
            >
              Custom
              <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" aria-hidden="true">
                <path d="M5 9l7 7 7-7" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </button>
          </div>
          {pickerOpen ? (
            <div className="activity__picker" role="dialog" aria-label="Custom timeframe">
              <label className="activity__custom-field">
                <span>From</span>
                <input
                  type="datetime-local"
                  className="activity__datetime"
                  value={customStart}
                  max={customEnd || undefined}
                  onChange={(event) => setCustomStart(event.target.value)}
                />
              </label>
              <label className="activity__custom-field">
                <span>To</span>
                <input
                  type="datetime-local"
                  className="activity__datetime"
                  value={customEnd}
                  min={customStart || undefined}
                  onChange={(event) => setCustomEnd(event.target.value)}
                />
              </label>
              {customError ? <p className="activity__custom-error">{customError}</p> : null}
              <div className="activity__picker-actions">
                <button type="button" className="activity__picker-cancel" onClick={() => setPickerOpen(false)}>
                  Cancel
                </button>
                <button type="button" className="activity__apply" onClick={applyCustomRange}>
                  Apply
                </button>
              </div>
            </div>
          ) : null}
        </div>
      </div>

      <div className="activity__stats">
        {STAT_FIELDS.map((stat) => (
          <div className="activity__stat" key={stat.key}>
            <div className="activity__stat-value">{summary.stats[stat.key].toLocaleString()}</div>
            <div className="activity__stat-label">{stat.label}</div>
          </div>
        ))}
      </div>

      <div className="activity__legend">
        {interventionCategories.map((category) => (
          <span className={`activity__pill activity__pill--${category}`} key={category}>
            <CategoryIcon category={category} />
            {categoryLabels[category]} {summary.categoryCounts[category]}
          </span>
        ))}
        <label className="activity__principal">
          <span className="activity__principal-label">Principal</span>
          <select value={principal} onChange={(event) => setPrincipal(event.target.value)}>
            <option value="">All principals</option>
            {principals.map((option) => (
              <option key={option.id} value={option.id}>
                {option.displayName}
              </option>
            ))}
          </select>
        </label>
      </div>

      <p className="activity__hint">marker height = severity · hover to inspect, click to pin · drag to zoom, double-click to zoom out</p>

      <div className="activity__chart">
        <span className="activity__chart-axis">↑ INTERVENTIONS</span>
        <InterventionSkyline
          summary={summary}
          pinnedId={pinnedId ?? undefined}
          onPin={setPinnedId}
          onZoom={handleZoom}
          onZoomOut={handleZoomOut}
        />
        <span className="activity__chart-axis activity__chart-axis--bottom">↓ request volume</span>
        {hasInterventions ? null : (
          <p className="activity__empty">No interventions in this window. Centian stayed on the baseline.</p>
        )}
      </div>

      {hasInterventions ? (
        <div className="activity__strip">
          <span className="activity__strip-label">FULL WINDOW</span>
          <div className="activity__strip-markers">
            {summary.interventions.map((item) => (
              <button
                key={item.id}
                type="button"
                title={item.title}
                aria-label={item.title}
                onClick={() => setPinnedId((current) => (current === item.id ? null : item.id))}
                className={
                  item.id === pinnedId
                    ? "activity__strip-marker activity__strip-marker--selected"
                    : "activity__strip-marker"
                }
                style={{
                  ["--marker-color" as string]: `var(--c-${item.category})`,
                  ["--marker-height" as string]: `${Math.round(item.severity * 100)}%`,
                }}
              >
                <span className="activity__strip-stem" />
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
