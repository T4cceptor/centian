import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import {
  type ActivityRange,
  type ActivitySummary,
  categoryLabels,
  fetchActivitySummary,
  interventionCategories,
} from "../api/activity";
import { ApiError, normalizeProjectSlug } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { CategoryIcon } from "./category-icon";
import { formatClockTime, formatTimestampCompact } from "./format";
import { InterventionSkyline } from "./intervention-skyline";

type LoadState = "loading" | "ready" | "error" | "unauthorized";

const RANGES: ActivityRange[] = ["1h", "6h", "1d", "1w"];

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
  const [summary, setSummary] = useState<ActivitySummary>();
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [range, setRange] = useState<ActivityRange>("6h");
  const [pinnedId, setPinnedId] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");
    setPinnedId(null);

    void fetchActivitySummary(projectSlug, range, controller.signal)
      .then((result) => {
        setSummary(result);
        setLoadState("ready");
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
  }, [projectSlug, range, reloadToken]);

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

  const window = formatTimestampCompact(summary.rangeStartUnixMilli);
  const hasInterventions = summary.interventions.length > 0;

  return (
    <div className="activity">
      <div className="activity__toolbar">
        <span className="activity__window">
          {window.date} · {formatClockTime(summary.rangeStartUnixMilli)}–{formatClockTime(summary.rangeEndUnixMilli)}
        </span>
        <div className="activity__range" role="group" aria-label="Time range">
          {RANGES.map((option) => (
            <button
              key={option}
              type="button"
              className={
                option === range ? "activity__range-btn activity__range-btn--active" : "activity__range-btn"
              }
              onClick={() => setRange(option)}
            >
              {option}
            </button>
          ))}
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
        <span className="activity__legend-caption">marker height = severity · hover a marker, click to pin</span>
      </div>

      <div className="activity__chart">
        <span className="activity__chart-axis">↑ INTERVENTIONS</span>
        <InterventionSkyline summary={summary} pinnedId={pinnedId ?? undefined} onPin={setPinnedId} />
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
