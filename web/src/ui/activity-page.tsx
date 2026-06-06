import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";

import {
  type ActivityRange,
  type ActivitySummary,
  type Intervention,
  type InterventionCategory,
  categoryLabels,
  fetchActivitySummary,
  interventionCategories,
} from "../api/activity";
import { ApiError, normalizeProjectSlug } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
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

// Picks the marker the detail panel should show first: the most severe intervention.
function highestSeverityId(interventions: Intervention[]): string | undefined {
  return interventions.reduce<Intervention | undefined>((best, item) => {
    return !best || item.severity > best.severity ? item : best;
  }, undefined)?.id;
}

export function ActivityPage() {
  const { projectSlug: rawProjectSlug } = useParams();
  const projectSlug = normalizeProjectSlug(rawProjectSlug);
  const [summary, setSummary] = useState<ActivitySummary>();
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [range, setRange] = useState<ActivityRange>("6h");
  const [selectedId, setSelectedId] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");

    void fetchActivitySummary(projectSlug, range, controller.signal)
      .then((result) => {
        setSummary(result);
        setSelectedId((current) => {
          if (current && result.interventions.some((item) => item.id === current)) {
            return current;
          }
          return highestSeverityId(result.interventions);
        });
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

  const selected = useMemo(
    () => summary?.interventions.find((item) => item.id === selectedId),
    [summary, selectedId],
  );

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
        <span className="activity__legend-caption">marker height = severity</span>
      </div>

      <div className="activity__chart">
        <span className="activity__chart-axis">↑ INTERVENTIONS</span>
        <InterventionSkyline summary={summary} selectedId={selectedId} onSelect={setSelectedId} />
        <span className="activity__chart-axis activity__chart-axis--bottom">↓ request volume</span>
      </div>

      {selected ? <InterventionDetail intervention={selected} /> : null}

      <div className="activity__strip">
        <span className="activity__strip-label">FULL WINDOW</span>
        <div className="activity__strip-markers">
          {summary.interventions.map((item) => (
            <button
              key={item.id}
              type="button"
              title={item.title}
              aria-label={item.title}
              onClick={() => setSelectedId(item.id)}
              className={
                item.id === selectedId
                  ? "activity__strip-marker activity__strip-marker--selected"
                  : "activity__strip-marker"
              }
              style={{ ["--marker-color" as string]: `var(--c-${item.category})`, ["--marker-height" as string]: `${Math.round(item.severity * 100)}%` }}
            >
              <span className="activity__strip-stem" />
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

function InterventionDetail({ intervention }: { intervention: Intervention }) {
  return (
    <div className={`activity__detail activity__detail--${intervention.category}`}>
      <div className="activity__detail-head">
        <span className={`activity__pill activity__pill--${intervention.category}`}>
          <CategoryIcon category={intervention.category} />
          {categoryLabels[intervention.category]}
        </span>
        <span className="activity__detail-time">{formatClockTime(intervention.timestampUnixMilli)}</span>
      </div>
      <div className="activity__detail-title">{intervention.title}</div>
      <div className="activity__detail-summary">{intervention.summary}</div>
      <div className="activity__detail-rule">
        <div className="activity__detail-rule-id">{intervention.ruleId}</div>
        <div className="activity__detail-rule-text">{intervention.ruleExplanation}</div>
      </div>
      <div className="activity__detail-footer">
        <span className="activity__detail-tool">{intervention.toolName}</span>
        <span className="activity__detail-sep">·</span>
        <span className="activity__detail-target">
          {intervention.gateway}/{intervention.serverName}
        </span>
      </div>
    </div>
  );
}

// Inline category icons matching the design mockup (no icon-library dependency).
function CategoryIcon({ category }: { category: InterventionCategory }) {
  const common = {
    width: 12,
    height: 12,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.7,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": true,
  };
  switch (category) {
    case "security":
      return (
        <svg {...common}>
          <path d="M12 2.5 19.5 5.3V11c0 5-3.4 8.3-7.5 10.2C7.9 19.3 4.5 16 4.5 11V5.3L12 2.5Z" />
        </svg>
      );
    case "policy":
      return (
        <svg {...common}>
          <path d="M6 3h8l4 4v14H6z" />
          <path d="M14 3v4h4" />
          <path d="M9 12h6M9 16h6" />
        </svg>
      );
    case "risk":
      return (
        <svg {...common}>
          <path d="M12 3 22 20H2L12 3Z" />
          <path d="M12 10v4.5M12 17.4h.01" />
        </svg>
      );
    case "quality":
      return (
        <svg {...common}>
          <circle cx="10.5" cy="10.5" r="6.5" />
          <path d="M15.5 15.5 21 21" />
        </svg>
      );
    case "compliance":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="9" />
          <path d="M8 12.3l2.7 2.7L16 9.5" />
        </svg>
      );
    default:
      return null;
  }
}
