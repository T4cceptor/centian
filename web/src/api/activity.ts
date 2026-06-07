// Activity ("Intervention Skyline") data client.
//
// Backed by GET /api/{projectSlug}/activity, which aggregates interventions,
// request-volume buckets, and headline stats from the event store.

import { projectApiPath, requestJSON } from "./task-runs";

export type ActivityRange = "live" | "5m" | "1h" | "6h" | "1d" | "1w";

export const activityRanges: ActivityRange[] = ["live", "5m", "1h", "6h", "1d", "1w"];

// Button labels per range. "live" is a rolling, auto-refreshing 90s window.
export const rangeLabels: Record<ActivityRange, string> = {
  live: "Live",
  "5m": "5m",
  "1h": "1h",
  "6h": "6h",
  "1d": "1d",
  "1w": "1w",
};

// Window length per range (ms), mirroring the server's range resolution so the
// client can derive a window without waiting for a summary response.
export const rangeDurationsMs: Record<ActivityRange, number> = {
  live: 90 * 1000,
  "5m": 5 * 60 * 1000,
  "1h": 60 * 60 * 1000,
  "6h": 6 * 60 * 60 * 1000,
  "1d": 24 * 60 * 60 * 1000,
  "1w": 7 * 24 * 60 * 60 * 1000,
};

// LIVE_REFRESH_MS is how often Live mode re-polls the activity window.
export const LIVE_REFRESH_MS = 5 * 1000;

export function isActivityRange(value: string | null | undefined): value is ActivityRange {
  return value != null && (activityRanges as string[]).includes(value);
}

export type InterventionCategory = "security" | "policy" | "risk" | "quality" | "compliance";

export const interventionCategories: InterventionCategory[] = [
  "security",
  "policy",
  "risk",
  "quality",
  "compliance",
];

export const categoryLabels: Record<InterventionCategory, string> = {
  security: "Security",
  policy: "Policy",
  risk: "Risk",
  quality: "Quality",
  compliance: "Compliance",
};

export type Intervention = {
  id: string;
  category: InterventionCategory;
  timestampUnixMilli: number;
  severity: number; // 0..1 → marker height
  title: string;
  summary: string;
  ruleId: string;
  ruleExplanation: string;
  toolName: string;
  gateway: string;
  serverName: string;
  label?: string; // short chart label, e.g. "Redacted"
};

export type VolumePoint = {
  timeUnixMilli: number;
  volume: number;
};

export type ActivityStats = {
  interventions: number;
  actionsBlocked: number;
  redacted: number;
  contextCharsIn: number;
  contextCharsOut: number;
  requestsInspected: number;
};

export type ActivitySummary = {
  rangeStartUnixMilli: number;
  rangeEndUnixMilli: number;
  stats: ActivityStats;
  categoryCounts: Record<InterventionCategory, number>;
  volume: VolumePoint[];
  interventions: Intervention[];
};

// An activity window is either a quick relative preset or an explicit custom
// range with absolute start/end timestamps (unix milli).
export type ActivityQuery =
  | { kind: "preset"; range: ActivityRange }
  | { kind: "custom"; startUnixMilli: number; endUnixMilli: number };

function buildActivityQuery(query: ActivityQuery, principal?: string): string {
  const params = new URLSearchParams();
  if (query.kind === "custom") {
    params.set("start", String(query.startUnixMilli));
    params.set("end", String(query.endUnixMilli));
  } else {
    params.set("range", query.range);
  }
  if (principal) {
    params.set("principal", principal);
  }
  return `?${params.toString()}`;
}

export async function fetchActivitySummary(
  projectSlug: string | undefined,
  query: ActivityQuery,
  principal?: string,
  signal?: AbortSignal,
): Promise<ActivitySummary> {
  const summary = await requestJSON<ActivitySummary>(
    `${projectApiPath(projectSlug, "/activity")}${buildActivityQuery(query, principal)}`,
    signal,
  );
  return {
    rangeStartUnixMilli: summary?.rangeStartUnixMilli ?? 0,
    rangeEndUnixMilli: summary?.rangeEndUnixMilli ?? 0,
    stats: summary?.stats ?? {
      interventions: 0,
      actionsBlocked: 0,
      redacted: 0,
      contextCharsIn: 0,
      contextCharsOut: 0,
      requestsInspected: 0,
    },
    categoryCounts: summary?.categoryCounts ?? {
      security: 0,
      policy: 0,
      risk: 0,
      quality: 0,
      compliance: 0,
    },
    volume: Array.isArray(summary?.volume) ? summary.volume : [],
    interventions: Array.isArray(summary?.interventions) ? summary.interventions : [],
  };
}
